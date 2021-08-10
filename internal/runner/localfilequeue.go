// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This contains the implementation of a simple file directory-based task queue
// to be used to retrieve work within an StudioML Exchange

import (
	"os"
	"context"
	"crypto/rsa"
	"fmt"
	"path"
	"regexp"
	"strings"
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

// FileQueue  encapsulated the configuration and extant extant client for a
// queue server
//
type FileQueue struct {
	root_dir    string          // full file path to root queues "server" directory
	queue_dir   string          // name of directory under root_dir
	                            // which implements a specific file queue
    timeout_sec int	            // timeout in seconds for lock/unlock operations
	wrapper     wrapper.Wrapper // Decryption infoprmation for messages with encrypted payloads
	logger      *log.Logger
	root_lock   *FileDirLock
	queue_lock  *FileDirLock
}

func NewFileQueue(root string, queue_subdir string, w wrapper.Wrapper, logger *log.Logger) (fq *FileQueue, err kv.Error) {
	timeout := 10

	fq = &FileQueue{
		root_dir: root,
		queue_dir: queue_subdir,
		timeout_sec: timeout,
		wrapper:  w,
		logger:   logger,
		queue_lock: &FileDirLock{
			dir_path: root,
			timeout_sec: timeout,
		},
		root_lock: &FileDirLock{
			dir_path: path.Join(root, queue_subdir),
			timeout_sec: timeout,
		},
	}
	return fq, nil
}

func (fq *FileQueue) IsEncrypted() (encrypted bool) {
	return nil != fq.wrapper
}

func (fq *FileQueue) URL() (urlString string) {
	return fq.root_dir
}

// Refresh will examine the local file queues "server" and extract a list of the queues
// that relate to StudioML work.
//
func (fq *FileQueue) Refresh(ctx context.Context, matcher *regexp.Regexp, mismatcher *regexp.Regexp) (known map[string]interface{}, err kv.Error) {

	//timeout := time.Duration(time.Minute)
	//if deadline, isPresent := ctx.Deadline(); isPresent {
	//	timeout = time.Until(deadline)
	//}
	known = map[string]interface{}{}

	if err = fq.queue_lock.Lock(); err != nil {
		return known, err
	}
	defer fq.queue_lock.UnLock()

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
		known[dir_name] = info.ModTime()
	}
    return known, nil
}

// GetKnown will connect to the rabbitMQ server identified in the receiver, rmq, and will
// query it for any queues that match the matcher regular expression
//
// found contains a map of keys that have an uncredentialed URL, and the value which is the user name and password for the URL
//
// The URL path is going to be the vhost and the queue name
//
func (fq *FileQueue) GetKnown(ctx context.Context, matcher *regexp.Regexp, mismatcher *regexp.Regexp) (found map[string]task.QueueDesc, err kv.Error) {
	known, err := fq.Refresh(ctx, matcher, mismatcher)
	if err != nil {
		return nil, err
	}
	for dir_name, _ := range known {
		fmt.Printf("Found: %s\n", dir_name)
	}
	return found, nil
}

// Exists will check that file queue named "subscription"
// does exist as sub-directory under root "server" directory.
//
func (fq *FileQueue) Exists(ctx context.Context, subscription string) (exists bool, err kv.Error) {
	if err := fq.root_lock.Lock(); err != nil {
		return false, err
	}
	defer fq.root_lock.UnLock()

	queue_path := path.Join(fq.root_dir, fq.queue_dir)
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
func (fq *FileQueue) GetShortQName(qt *task.QueueTask) (shortName string, err kv.Error) {
	splits := strings.SplitN(qt.Subscription, "/", 3)
	if len(splits) < 3 {
		return "", kv.NewError("malformed file queue subscription").With("stack", stack.Trace().TrimRuntime()).With("subscription", qt.Subscription)
	}
	shortName = fmt.Sprintf("%s/%s", splits[1], splits[2])
	return shortName, nil
}

// Work will connect to the rabbitMQ server identified in the receiver, rmq, and will see if any work
// can be found on the queue identified by the go runner subscription and present work
// to the handler for processing
//
func (fq *FileQueue) Work(ctx context.Context, qt *task.QueueTask) (msgProcessed bool, resource *server.Resource, err kv.Error) {

	return false, nil, err
}

// Publish is a shim method for tests to use for sending requestst to a queue
//
func (fq *FileQueue) Publish(key string, contentType string, msg []byte) (err kv.Error) {
	return nil
}

// HasWork will look at the local file queue to see if there is any pending work.  The function
// is called in an attempt to see if there is any point in processing new work without a
// lot of overhead.
//
func (fq *FileQueue) HasWork(ctx context.Context, subscription string) (hasWork bool, err kv.Error) {
	return true, nil
}

// Responder is used to open a connection to an existing response queue if
// one was made available and also to provision a channel into which the
// runner can place report messages
func (fq *FileQueue) Responder(ctx context.Context, subscription string, encryptKey *rsa.PublicKey) (sender chan *runnerReports.Report, err kv.Error) {
	return nil, kv.NewError("Not implemented").With("stack", stack.Trace().TrimRuntime())
}
