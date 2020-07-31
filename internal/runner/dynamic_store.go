// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// This file contains the generic implementation of a disk backed store that
// is accessible as a collection.  It is used within the runner by
// modules storing configuration information and public key files etc.

// RefreshContext is used to track when the background service function has checked
// the file system backing store for new, updated, or deleted items
//
type RefreshContext struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// DynamicStore encapsulates an instance of a single directory that is backing the in-memory
// file contents, it also includes a reference to the next pending refresh and a function
// that is used when the directory files change
//
type DynamicStore struct {
	contents map[string]interface{} // The known items retrieved from the backing directory
	dir      string                 // backing directory
	refresh  RefreshContext         // Trigger for when the refresh od the backing store has occurred
	extract  DSExtract              // A custom function for decoding the contents of files on disk for loading into the collection
	sync.Mutex
}

func reportErr(err kv.Error, errorC chan<- kv.Error) {
	if err == nil {
		return
	}

	// Remove the entry for this function from the stack
	stk := stack.Trace().TrimRuntime()[1:]

	defer func() {
		_ = recover()
		if err != nil {
			fmt.Println(err.With("stack", stk).Error())
		}
	}()

	// Try to send the error and backoff to simply printing it if
	// we could not send it to the reporting module
	select {
	case errorC <- err.With("stack", stk):
	case <-time.After(time.Second):
		fmt.Println(err.With("stack", stk).Error())
	}
}

// NewDynamicStore is used to initialize a watched dynamic store of items that is backed by the file system.
// This is a non-blocking function that will spawn a go routine that uses the ctx context to
// stop when the context is done.
//
func NewDynamicStore(ctx context.Context, configuredDir string, extractFN DSExtract, refresh time.Duration, errorC chan<- kv.Error) (store *DynamicStore, err kv.Error) {
	store = &DynamicStore{
		contents: map[string]interface{}{},
		extract:  extractFN,
	}

	if err = store.Init(ctx, configuredDir, refresh, errorC); err != nil {
		return nil, err
	}
	return store, nil
}

// Init is used to initialize a directory watcher backed store
func (s *DynamicStore) Init(ctx context.Context, configuredDir string, refresh time.Duration, errorC chan<- kv.Error) (err kv.Error) {

	dir, errGo := filepath.Abs(configuredDir)
	if errGo != nil {
		return kv.Wrap(errGo).With("dir", dir)
	}

	go s.serviceDynamicStore(ctx, dir, refresh, errorC)

	return nil
}

// serviceDynamicStore will scan a directory for changes on a regular basis and update the
// file content based cache when changes are seen
//
func (s *DynamicStore) serviceDynamicStore(ctx context.Context, dir string, refresh time.Duration, errorC chan<- kv.Error) {
	// Wait until the directory exists and accessed at least once
	updatedEntries, errGo := ioutil.ReadDir(dir)
	// Record the last modified time for the file representing a signature key
	entries := make(map[string]time.Time, len(updatedEntries))

	// Set the last time an error was reported to more then 15 minutes ago so
	// that the first error is displayed immediately
	lastErrNotify := time.Now().Add(-1 * time.Hour)

	// Wait until we get at least one good read from the
	// directory being monitored for signatures
	for {
		if errGo == nil {
			break
		}

		// Only display this particular error
		if time.Since(lastErrNotify) > time.Duration(15*time.Minute) {
			if errGo != nil {
				reportErr(kv.Wrap(errGo).With("dir", dir), errorC)
			}
			lastErrNotify = time.Now()
		}

		select {
		case <-time.After(refresh):
			_, errGo = ioutil.ReadDir(dir)
		case <-ctx.Done():
			return
		}
	}

	// Once we know we have a working signatures storage directory save its location
	// so that test software can inject certificates of their own when running
	// with a production server under test
	refreshCtx, cancel := context.WithCancel(context.Background())
	s.Lock()
	s.refresh = RefreshContext{
		ctx:    refreshCtx,
		cancel: cancel,
	}
	s.dir = dir
	s.Unlock()

	// Event loop for the watcher until the context is done
	for {
		select {

		case <-time.After(refresh):

			// It is possible that the backing store directory is changed during runtime
			// so refresh the location
			s.Lock()
			dir = s.dir
			s.Unlock()

			// A lookaside collection for checking the presence of directory entries
			// that are no longer found on the disk
			deletionCheck := make(map[string]time.Time, len(entries))

			if updatedEntries, errGo = ioutil.ReadDir(dir); errGo != nil {
				reportErr(kv.Wrap(errGo).With("dir", dir), errorC)
				continue
			}

			for _, entry := range updatedEntries {

				if entry.IsDir() {
					continue
				}

				if entry.Name()[0] == '.' {
					continue
				}

				// Symbolic link checking
				if entry.Mode()&os.ModeSymlink != 0 {
					target, errGo := filepath.EvalSymlinks(filepath.Join(dir, entry.Name()))
					if errGo != nil {
						reportErr(kv.Wrap(errGo).With("dir", dir, "target", entry.Name()), errorC)
						continue
					}
					if entry, errGo = os.Stat(target); errGo != nil {
						reportErr(kv.Wrap(errGo).With("dir", dir, "target", entry.Name()), errorC)
						continue
					}
				}

				curEntry, isPresent := entries[entry.Name()]
				if !isPresent || curEntry.Round(time.Second) != entry.ModTime().Round(time.Second) {
					entries[entry.Name()] = entry.ModTime().Round(time.Second)
					if err := s.update(filepath.Join(dir, entry.Name())); err != nil {
						// info is a special file that is used to prevent the secret from not
						// being created by Kubernetes when there are no secrets to be mounted
						if entry.Name() != "info" {
							reportErr(err, errorC)
						}
					}
				}

				deletionCheck[entry.Name()] = curEntry
			}
			for name := range entries {
				if _, isPresent := deletionCheck[name]; !isPresent {
					// Have the update method check for the presence of the file,
					// it will cleanup if the file is not found
					s.update(filepath.Join(dir, name))
					// Now remove the missing from our small lookaside collection
					delete(entries, name)
				}
			}

			// Signal any waiters that the refresh has been processed and replace the context
			// used for this with a new one that can be waited on by observers
			s.Reset()

		case <-ctx.Done():
			return
		}
	}
}

// Reset is used to load a new pending refresh notification channel and trigger
// the old one
func (s *DynamicStore) Reset() {
	s.Lock()
	defer s.Unlock()

	s.refresh.cancel()
	s.refresh.ctx, s.refresh.cancel = context.WithCancel(context.Background())
}

type DSExtract func(data []byte) (item interface{}, err kv.Error)

// update is used to refresh the in memory collection of files when
// the disk directory file has been modified or added
func (s *DynamicStore) update(fn string) (err kv.Error) {
	data, errGo := ioutil.ReadFile(fn)
	if errGo != nil {
		if os.IsNotExist(errGo) {
			s.Lock()
			delete(s.contents, filepath.Base(fn))
			s.Unlock()
			return nil
		}
		return kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}

	pub, err := s.extract(data)
	if err != nil {
		return err.With("filename", fn)
	}

	s.Lock()
	s.contents[filepath.Base(fn)] = pub
	s.Unlock()

	return nil
}

func (s *DynamicStore) getRefresh() (doneCtx context.Context) {
	s.Lock()
	defer s.Unlock()

	return s.refresh.ctx
}

func (s *DynamicStore) getDir() (dir string) {
	s.Lock()
	defer s.Unlock()

	return s.dir
}

// get retrieves a signature that has a queue name supplied by the caller
// as an exact match
//
func (s *DynamicStore) get(q string) (item interface{}, err kv.Error) {
	s.Lock()
	item, isPresent := s.contents[q]
	s.Unlock()

	if !isPresent {
		return nil, kv.NewError("not found").With("queue", q).With("stack", stack.Trace().TrimRuntime())
	}
	return item, nil
}

// selection retrieves a signature that has a queue name supplied by the caller
// using the longest prefix matched queue name for the supplied queue name
// that can be found.
//
func (s *DynamicStore) selection(q string) (item interface{}, err kv.Error) {
	// The lock is kept until we are done to ensure once a prefix is matched to its longest length
	// that we still have the public key for it
	s.Lock()
	defer s.Unlock()
	if len(s.contents) == 0 {
		return nil, kv.NewError("not found").With("queue", q).With("stack", stack.Trace().TrimRuntime())
	}
	prefixes := make([]string, 0, len(s.contents))
	for k := range s.contents {
		prefixes = append(prefixes, k)
	}
	sort.Strings(prefixes)

	// Start with no valid match as a prefix
	bestMatch := ""
	wouldBeAt := 0

	// Roll through the sorted prefixes while there is a still a valid signature name prefix of the q (queue)
	// names, stop when the q supplied no longer satisfies the prefix and the one prior would be
	// the shortest signature prefix of the q name.
	for {
		if prefixes[wouldBeAt] == q {
			bestMatch = prefixes[wouldBeAt]
			break
		}
		if strings.HasPrefix(q, prefixes[wouldBeAt]) {
			if len(bestMatch) == 0 || len(bestMatch) < len(prefixes[wouldBeAt]) {
				bestMatch = prefixes[wouldBeAt]
			}
		}
		if wouldBeAt += 1; wouldBeAt >= len(prefixes) {
			break
		}
	}

	if len(bestMatch) == 0 {
		return nil, kv.NewError("not found").With("queue", q).With("stack", stack.Trace().TrimRuntime())
	}
	item = s.contents[bestMatch]
	return item, nil
}
