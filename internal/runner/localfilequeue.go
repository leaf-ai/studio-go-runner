// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This contains the implementation of a simple file directory-based task queue
// to be used to retrieve work within an StudioML Exchange

import (
	"bufio"
	"context"
	"crypto/rsa"
	"fmt"
	"os"
	"path"
	"regexp"
	"time"

	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"

	"github.com/leaf-ai/go-service/pkg/log"
	"github.com/leaf-ai/go-service/pkg/server"

	"github.com/leaf-ai/studio-go-runner/internal/task"
	"github.com/leaf-ai/studio-go-runner/pkg/wrapper"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
	"github.com/rs/xid"
)

const (
	FileLockName = "lock.lock"
)

type FileDirLock struct {
	dirPath string
	timeout time.Duration
}

func GetFileLock(filePath string, timeout time.Duration) kv.Error {
	var lockFile *os.File = nil
	var err error
	defer func() { if lockFile != nil { lockFile.Close() } } ()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		lockFile, err = os.OpenFile(filePath, os.O_CREATE | os.O_EXCL, 0)
		if err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
    return kv.NewError(fmt.Sprintf("Timeout trying to acquire %s", filePath)).With("stack", stack.Trace().TrimRuntime())
}

func UnlockFile(filePath string) (err kv.Error) {
	if errGo := os.Remove(filePath); errGo != nil {
	    return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

func (lock *FileDirLock) Check() (err kv.Error) {
	fileInfo, errGo := os.Stat(lock.dirPath)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if !fileInfo.IsDir() {
		return kv.NewError(fmt.Sprintf("Not a directory: %s", lock.dirPath)).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

func (lock *FileDirLock) Lock() (err kv.Error) {
	if err := lock.Check(); err != nil {
		return err
	}
	lockPath := path.Join(lock.dirPath, FileLockName)
	return GetFileLock(lockPath, lock.timeout)
}

func (lock *FileDirLock) UnLock() (err kv.Error) {
	if err := lock.Check(); err != nil {
		return err
	}
	lockPath := path.Join(lock.dirPath, FileLockName)
	return UnlockFile(lockPath)
}

// LocalQueue "project" is basically a local root directory
// containing queues sub-directories.
type LocalQueue struct {
	RootDir string          // full file path to root queues "server" directory
    timeout time.Duration   // timeout in seconds for lock/unlock operations
	wrapper wrapper.Wrapper // Decryption information for messages with encrypted payloads
	logger  *log.Logger
	lock    *FileDirLock
}

func NewLocalQueue(root string, w wrapper.Wrapper, logger *log.Logger) (fq *LocalQueue) {
	timeout := 10 * time.Second

	fqp := &LocalQueue{
		RootDir: root,
		timeout: timeout,
		wrapper: w,
		logger:  logger,
		lock: &FileDirLock{
			dirPath: root,
			timeout: timeout,
		},
	}
	return fqp
}

func (fq *LocalQueue) IsEncrypted() (encrypted bool) {
	return nil != fq.wrapper
}

func (fq *LocalQueue) ensureQueueExists(queueName string) (queuePath string, err kv.Error) {
    fq.lock.Lock()
    defer fq.lock.UnLock()

    queuePath = path.Join(fq.RootDir, queueName)
	queueStat, errGo := os.Stat(queuePath)
    if errGo != nil {
        if os.IsNotExist(errGo) {
		    errGo = os.Mkdir(queuePath, os.ModeDir | 0o775)
		    if errGo == nil {
		    	// We must query os.Stat() again here:
	            queueStat, errGo = os.Stat(queuePath)
			}
	    }
	}
	if errGo != nil {
		return queuePath, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", queuePath)
	}
	if !queueStat.IsDir() {
		// We already have regular file with the same name:
		return queuePath, kv.NewError("Regular file exists already").With("stack", stack.Trace().TrimRuntime()).With("path", queuePath)
	}
	return queuePath, nil
}

func (fq *LocalQueue) Publish(queueName string, contentType string, msg []byte) (err kv.Error) {
	queuePath := ""
	if queuePath, err = fq.ensureQueueExists(queueName); err != nil {
		return err
	}
	// Get a unique file name for our queue item:
	fileName := path.Join(queuePath, xid.New().String())

	queueLock := &FileDirLock{
		dirPath: queuePath,
		timeout: fq.timeout,
	}
	if err = queueLock.Lock(); err != nil {
		return err
	}
	defer queueLock.UnLock()

	itemFile, errGo := os.Create(fileName)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", fileName)
	}
	defer itemFile.Close()

	written, errGo := itemFile.Write(msg)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", fileName)
	}
	fq.logger.Debug("Wrote payload", "file", fileName, "length", written, "contentType", contentType)

	return nil
}

// Refresh will examine the local file queues "server" and extract a list of the queues
// that relate to StudioML work.
//
func (fq *LocalQueue) Refresh(ctx context.Context, matcher *regexp.Regexp, mismatcher *regexp.Regexp) (known map[string]interface{}, err kv.Error) {

	known = map[string]interface{}{}

	if err = fq.lock.Lock(); err != nil {
		return known, err
	}
	defer fq.lock.UnLock()

	rootFile, errGo := os.Open(fq.RootDir)
	if errGo != nil {
		return known, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", fq.RootDir)
	}
	defer rootFile.Close()

	listInfo, errGo := rootFile.Readdir(-1)
	if errGo != nil {
		return known, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", fq.RootDir)
	}

	for _, info := range listInfo {
		// We are looking for subdirectories in our root "server" directory:
		if !info.IsDir() {
			continue
		}
		dirName := info.Name()
		if matcher != nil {
			if !matcher.MatchString(dirName) {
				continue
			}
		}
		if mismatcher != nil {
			// We cannot allow an excluded queue
			if mismatcher.MatchString(dirName) {
				continue
			}
		}
		known[path.Join(fq.RootDir, dirName)] = info.ModTime()
	}
    return known, nil
}

func (fq *LocalQueue) GetKnown(ctx context.Context, matcher *regexp.Regexp, mismatcher *regexp.Regexp) (found map[string]task.QueueDesc, err kv.Error) {
	// We only know one "project", and that's us.
	found = make(map[string]task.QueueDesc, 1)
	queueDesc := task.QueueDesc{
		Proj: fq.RootDir,
		Mgt: "",
		Cred: "",
	}
	found[fq.RootDir] = queueDesc
	return found, nil
}

// Exists will check that file queue named "subscription"
// does exist as sub-directory under root "server" directory.
//
func (fq *LocalQueue) Exists(ctx context.Context, subscription string) (exists bool, err kv.Error) {
	if err := fq.lock.Lock(); err != nil {
		return false, err
	}
	defer fq.lock.UnLock()

	queuePath := path.Join(fq.RootDir, subscription)
	fileInfo, errGo := os.Stat(queuePath)
	if os.IsNotExist(errGo) {
		return false, nil
	}
	if errGo != nil {
		return false, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", queuePath)
	}
	if !fileInfo.IsDir() {
		return false, kv.NewError("Not a directory").With("stack", stack.Trace().TrimRuntime()).With("path", queuePath)
	}
	return true, nil
}

// GetShortQName GetShortQueueName is useful for storing queue specific information in collections etc
func (fq *LocalQueue) GetShortQName(qt *task.QueueTask) (shortName string, err kv.Error) {
	return qt.Subscription, nil
}

func getOldest(listInfo []os.FileInfo) (result int) {
	result = -1
	if len(listInfo) == 0 {
		return result
	}
	minTime := time.Now().Add(time.Hour)
	for inx, item := range listInfo {
		if item.Name() != FileLockName && !item.IsDir() && item.ModTime().Before(minTime) {
			result = inx
			minTime = item.ModTime()
		}
	}
	return result
}

func readBytes(filePath string) (data []byte, err kv.Error) {
	// Read the whole file into []byte
	itemFile, errGo := os.Open(filePath)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", filePath)
	}
	defer itemFile.Close()

	stats, errGo := itemFile.Stat()
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", filePath)
	}
	data = make([]byte, stats.Size())
	buffer := bufio.NewReader(itemFile)
	_, errGo = buffer.Read(data)
	if errGo != nil {
		return data, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", filePath)
	}
	return data, nil
}

func (fq *LocalQueue) getOldestItem(subscription string) (item os.FileInfo, err kv.Error) {
	queueDirPath := subscription

	rootFile, errGo := os.Open(queueDirPath)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", queueDirPath)
	}
	defer rootFile.Close()

	listInfo, errGo := rootFile.Readdir(-1)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", queueDirPath)
	}
	itemInx := getOldest(listInfo)
	if itemInx < 0 {
		fq.logger.Debug("No item was selected", "queue", queueDirPath)
		// Nothing is found in our "queue"
		return nil, nil
	}
	return listInfo[itemInx], nil
}

func (fq *LocalQueue) Get(subscription string) (Msg []byte, MsgID string, err kv.Error) {
	queueDirPath := subscription
	lock := &FileDirLock{  dirPath: queueDirPath,
		                   timeout: fq.timeout,
	                    }
    if err := lock.Lock(); err != nil {
		return nil, "", err
	}
	defer lock.UnLock()

    itemInfo, err := fq.getOldestItem(subscription)
    if err != nil {
    	return nil, "", err
	}
	if itemInfo == nil {
		fq.logger.Debug("No item was selected", "queue", queueDirPath)
		// Nothing is found in our "queue"
		return nil, "", nil
	}
	MsgID = path.Join(queueDirPath, itemInfo.Name())
	// Read the whole file into []byte
	Msg, err = readBytes(MsgID)
	if err != nil {
		return Msg, MsgID, err
	}

	if errGo := os.Remove(MsgID); errGo != nil {
		return Msg, MsgID, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", MsgID)
	}
	return Msg, MsgID, nil
}

// Work will connect to the FileQueue "server" identified in the receiver, fq, and will see if any work
// can be found on the queue identified by the go runner subscription and present work
// to the handler for processing
//
func (fq *LocalQueue) Work(ctx context.Context, qt *task.QueueTask) (msgProcessed bool, resource *server.Resource, err kv.Error) {

	fq.logger.Debug("Enter: WORK", "subscription", qt.Subscription)
	defer fq.logger.Debug("Exit: WORK", "subscription", qt.Subscription)

	msgBytes, filePath, err := fq.Get(qt.Subscription)
	if err != nil {
		return false, nil, err
	}
	if msgBytes == nil {
		// Without error, it means there are no requests on this queue currently
		return false, nil, nil
	}

    // We got a task request - process it:
    fq.logger.Info("Got request in:", filePath, "length", len(msgBytes))

	qt.Msg = msgBytes
	qt.ShortQName = qt.Subscription

	fq.logger.Debug("About to handle task request: ", filePath)
	rsc, ack, err := qt.Handler(ctx, qt)
	if !ack {
		fq.logger.Debug("Got NACK on task request: ", filePath)
	} else {
		fq.logger.Debug("Got ACK on task request: ", filePath)
	}

	return true, rsc, err
}

// HasWork will look at the local file queue to see if there is any pending work.  The function
// is called in an attempt to see if there is any point in processing new work without a
// lot of overhead.
//
func (fq *LocalQueue) HasWork(ctx context.Context, subscription string) (hasWork bool, err kv.Error) {
	queueDirPath := subscription
	lock := &FileDirLock{
		dirPath: queueDirPath,
		timeout: fq.timeout,
	}
	if err := lock.Lock(); err != nil {
		return false, err
	}
	defer lock.UnLock()

	itemInfo, err := fq.getOldestItem(subscription)
	if err != nil {
		return false, err
	}
	return itemInfo != nil, nil
}

// Responder is used to open a connection to an existing response queue if
// one was made available and also to provision a channel into which the
// runner can place report messages
func (fq *LocalQueue) Responder(ctx context.Context, subscription string, encryptKey *rsa.PublicKey) (sender chan *runnerReports.Report, err kv.Error) {
	return nil, kv.NewError("Not implemented").With("stack", stack.Trace().TrimRuntime())
}
