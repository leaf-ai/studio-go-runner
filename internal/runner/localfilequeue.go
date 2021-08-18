// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This contains the implementation of a simple file directory-based task queue
// to be used to retrieve work within an StudioML Exchange

import (
	"bufio"
	"context"
	"crypto/rsa"
	"fmt"
	"github.com/rs/xid"
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
)

const (
	FileLockName = "lock.lock"
)

type FileDirLock struct {
	dir_path      string
	timeout_sec   int
}

func GetFileLock(file_path string, timeout_sec int) kv.Error {
	var lock_file *os.File = nil
	var err error
	defer func() { if lock_file != nil { lock_file.Close() } } ()

	deadline := time.Now().Add(time.Duration(timeout_sec) * time.Second)
	for time.Now().Before(deadline) {
		lock_file, err = os.OpenFile(file_path, os.O_CREATE | os.O_EXCL, 0)
		if err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
    return kv.NewError(fmt.Sprintf("Timeout trying to acquire %s", file_path)).With("stack", stack.Trace().TrimRuntime())
}

func UnlockFile(file_path string) kv.Error {
	error := os.Remove(file_path)
	return kv.Wrap(error).With("stack", stack.Trace().TrimRuntime())
}

func (lock *FileDirLock) Check() (err kv.Error) {
	fileInfo, errGo := os.Stat(lock.dir_path)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if !fileInfo.IsDir() {
		return kv.NewError(fmt.Sprintf("Not a directory: %s", lock.dir_path)).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

func (lock *FileDirLock) Lock() (err kv.Error) {
	if err := lock.Check(); err != nil {
		return err
	}
	lock_path := path.Join(lock.dir_path, FileLockName)
	return GetFileLock(lock_path, lock.timeout_sec)
}

func (lock *FileDirLock) UnLock() (err kv.Error) {
	if err := lock.Check(); err != nil {
		return err
	}
	lock_path := path.Join(lock.dir_path, FileLockName)
	return UnlockFile(lock_path)
}

// LocalQueue "project" is basically a local root directory
// containing queues sub-directories.
type LocalQueue struct {
	root_dir    string          // full file path to root queues "server" directory
    timeout_sec int	            // timeout in seconds for lock/unlock operations
	wrapper     wrapper.Wrapper // Decryption infoprmation for messages with encrypted payloads
	logger      *log.Logger
	lock        *FileDirLock
}

func NewLocalQueue(root string, w wrapper.Wrapper, logger *log.Logger) (fq *LocalQueue) {
	timeout := 10

	fqp := &LocalQueue{
		root_dir: root,
		timeout_sec: timeout,
		wrapper:  w,
		logger:   logger,
		lock: &FileDirLock{
			dir_path: root,
			timeout_sec: timeout,
		},
	}
	return fqp
}

func (fq *LocalQueue) IsEncrypted() (encrypted bool) {
	return nil != fq.wrapper
}

func (fq *LocalQueue) URL() (urlString string) {
	return fq.root_dir
}

func (fq *LocalQueue) GetRoot() (urlString string) {
	return fq.root_dir
}

func (fq *LocalQueue) EnsureQueueExists(queueName string) (queue_path string, err kv.Error) {
    fq.lock.Lock()
    defer fq.lock.UnLock()

    queue_path = path.Join(fq.root_dir, queueName)
	queue_stat, errGo := os.Stat(queue_path)
	if errGo == nil {
		if queue_stat.IsDir() {
			return queue_path, nil
		}
		// We already have regular file with the same name:
		return queue_path, kv.NewError("Regular file exists already").With("stack", stack.Trace().TrimRuntime()).With("path", queue_path)
	}
	if os.IsNotExist(errGo) {
		errGo = os.Mkdir(queue_path, os.ModeDir | 0o775)
	}
	if errGo != nil {
		return queue_path, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", queue_path)
	}
    return queue_path, nil
}

func (fq *LocalQueue) Publish(queueName string, contentType string, msg []byte) (err kv.Error) {
	queue_path := ""
	if queue_path, err = fq.EnsureQueueExists(queueName); err != nil {
		return err
	}
	// Get a unique file name for our queue item:
	file_name := path.Join(queue_path, xid.New().String())

	queue_lock := &FileDirLock{
		dir_path:    queue_path,
		timeout_sec: fq.timeout_sec,
	}
	if err = queue_lock.Lock(); err != nil {
		return err
	}
	defer queue_lock.UnLock()

	item_file, errGo := os.Create(file_name)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", file_name)
	}
	defer item_file.Close()

	written, errGo := item_file.Write(msg)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", file_name)
	}
	fq.logger.Debug(fmt.Sprintf("Wrote %d bytes payload (%s) to file %s", written, contentType, file_name))

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

	root_file, errGo := os.Open(fq.root_dir)
	if errGo != nil {
		return known, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", fq.root_dir)
	}
	defer root_file.Close()

	listInfo, errGo := root_file.Readdir(-1)
	if errGo != nil {
		return known, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", fq.root_dir)
	}

	for _, info := range listInfo {
		// We are looking for subdirectories in our root "server" directory:
		if !info.IsDir() {
			continue
		}
		dir_name := info.Name()
		if matcher != nil {
			if !matcher.MatchString(dir_name) {
				continue
			}
		}
		if mismatcher != nil {
			// We cannot allow an excluded queue
			if mismatcher.MatchString(dir_name) {
				continue
			}
		}
		known[path.Join(fq.root_dir, dir_name)] = info.ModTime()
	}
    return known, nil
}

func (fq *LocalQueue) GetKnown(ctx context.Context, matcher *regexp.Regexp, mismatcher *regexp.Regexp) (found map[string]task.QueueDesc, err kv.Error) {
	// We only know one "project", and that's us.
	found = make(map[string]task.QueueDesc, 1)
	queue_desc := task.QueueDesc{
		Proj: fq.root_dir,
		Mgt: "",
		Cred: "",
	}
	found[fq.root_dir] = queue_desc
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

	queue_path := path.Join(fq.root_dir, subscription)
	fileInfo, errGo := os.Stat(queue_path)
	if os.IsNotExist(errGo) {
		return false, nil
	}
	if errGo != nil {
		return false, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", queue_path)
	}
	if !fileInfo.IsDir() {
		return false, kv.NewError("Not a directory").With("stack", stack.Trace().TrimRuntime()).With("path", queue_path)
	}
	return true, nil
}

// GetShortQueueName is useful for storing queue specific information in collections etc
func (fq *LocalQueue) GetShortQName(qt *task.QueueTask) (shortName string, err kv.Error) {
	return qt.Subscription, nil
}

// Parameter subscription: full file path to FileQueue directory
func GetOldest(listInfo []os.FileInfo) int {
	result := -1
	if len(listInfo) == 0 {
		return result
	}
	min_time := time.Now().Add(time.Hour)
	for inx, item := range listInfo {
		if item.Name() != FileLockName && !item.IsDir() && item.ModTime().Before(min_time) {
			result = inx
			min_time = item.ModTime()
		}
	}
	return result
}

func ReadBytes(file_path string) (data []byte, err kv.Error) {
	// Read the whole file into []byte
	item_file, errGo := os.Open(file_path)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", file_path)
	}
	defer item_file.Close()

	stats, errGo := item_file.Stat()
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", file_path)
	}
	var item_size = stats.Size()
	data = make([]byte, item_size)
	bufr := bufio.NewReader(item_file)
	_, errGo = bufr.Read(data)
	if errGo != nil {
		return data, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", file_path)
	}
	return data, nil
}

func (fq *LocalQueue) Get(subscription string) (Msg []byte, MsgID string, err kv.Error) {

	fq.logger.Debug(fmt.Sprintf("Enter: GET data for sub: %s", subscription))
	defer fq.logger.Debug(fmt.Sprintf("Exit: GET data for sub: %s", subscription))

	queue_dir_path := subscription
	lock := &FileDirLock{  dir_path: queue_dir_path,
		                   timeout_sec: fq.timeout_sec,
	                    }
    if err := lock.Lock(); err != nil {
		return nil, "", err
	}
	defer lock.UnLock()

	root_file, errGo := os.Open(queue_dir_path)
	if errGo != nil {
		return nil, "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", queue_dir_path)
	}
	defer root_file.Close()

	listInfo, errGo := root_file.Readdir(-1)
	if errGo != nil {
		return nil, "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", queue_dir_path)
	}
	item_inx := GetOldest(listInfo)
	if item_inx < 0 {
		fq.logger.Debug(fmt.Sprintf("No item was selected in queue: %s", queue_dir_path))
		// Nothing is found in our "queue"
		return nil, "", nil
	}
	MsgID = path.Join(queue_dir_path, listInfo[item_inx].Name())
	// Read the whole file into []byte
	Msg, err = ReadBytes(MsgID)
	if err != nil {
		return Msg, MsgID, err
	}

	if errGo = os.Remove(MsgID); errGo != nil {
		return Msg, MsgID, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", MsgID)
	}
	return Msg, MsgID, nil
}

// Work will connect to the FileQueue "server" identified in the receiver, fq, and will see if any work
// can be found on the queue identified by the go runner subscription and present work
// to the handler for processing
//
func (fq *LocalQueue) Work(ctx context.Context, qt *task.QueueTask) (msgProcessed bool, resource *server.Resource, err kv.Error) {

	fq.logger.Debug(fmt.Sprintf("Enter: WORK for sub: %s", qt.Subscription))
	defer fq.logger.Debug(fmt.Sprintf("Exit: WORK for sub: %s", qt.Subscription))

	msg_bytes, file_path, err := fq.Get(qt.Subscription)
	if err != nil {
		return false, nil, err
	}
	if msg_bytes == nil {
		// Without error, it means there are no requests on this queue currently
		return false, nil, nil
	}

    // We got a task request - process it:
    fq.logger.Info(fmt.Sprintf("Got request in %s: len %d bytes", file_path, len(msg_bytes)))

	qt.Msg = msg_bytes
	qt.ShortQName = qt.Subscription

	fq.logger.Debug("About to handle task request: %s", file_path)
	rsc, ack, err := qt.Handler(ctx, qt)
	if !ack {
		fq.logger.Debug("Got NACK on task request: %s", file_path)
	} else {
		fq.logger.Debug("Got ACK on task request: %s", file_path)
	}

	return true, rsc, err
}

// HasWork will look at the local file queue to see if there is any pending work.  The function
// is called in an attempt to see if there is any point in processing new work without a
// lot of overhead.
//
func (fq *LocalQueue) HasWork(ctx context.Context, subscription string) (hasWork bool, err kv.Error) {
	return true, nil
}

// Responder is used to open a connection to an existing response queue if
// one was made available and also to provision a channel into which the
// runner can place report messages
func (fq *LocalQueue) Responder(ctx context.Context, subscription string, encryptKey *rsa.PublicKey) (sender chan *runnerReports.Report, err kv.Error) {
	return nil, kv.NewError("Not implemented").With("stack", stack.Trace().TrimRuntime())
}
