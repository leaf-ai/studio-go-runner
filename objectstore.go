package runner

// This file contains the implementation of storage that can use an internal cache along with the MD5
// hash of the files contents to avoid downloads that are not needed.

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"

	"github.com/karlmutch/ccache"

	"github.com/karlmutch/go-shortid"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	cacheHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_cache_hits",
			Help: "Number of cache hits.",
		},
		[]string{"host", "hash"},
	)
	cacheMisses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_cache_misses",
			Help: "Number of cache misses.",
		},
		[]string{"host", "hash"},
	)

	host = ""
)

func init() {
	host, _ = os.Hostname()
}

type ObjStore struct {
	store  Storage
	ErrorC chan errors.Error
}

func NewObjStore(spec *StoreOpts, errorC chan errors.Error) (os *ObjStore, err errors.Error) {
	store, err := NewStorage(spec)
	if err != nil {
		return nil, err
	}

	return &ObjStore{
		store:  store,
		ErrorC: errorC,
	}, nil
}

var (
	backingDir = ""

	cacheMax      int64
	cacheInit     sync.Once
	cacheInitSync sync.Mutex
	cache         *ccache.Cache
)

func groom(backingDir string, removedC chan os.FileInfo, errorC chan errors.Error) {
	if cache == nil {
		return
	}
	cachedFiles, err := ioutil.ReadDir(backingDir)
	if err != nil {

		go func() {
			defer func() {
				recover()
			}()
			select {
			case errorC <- errors.Wrap(err, fmt.Sprintf("cache dir %s refresh failure", backingDir)).With("stack", stack.Trace().TrimRuntime()):
			case <-time.After(time.Second):
				fmt.Printf("%s\n", errors.Wrap(err, fmt.Sprintf("cache dir %s refresh failed", backingDir)).With("stack", stack.Trace().TrimRuntime()))
			}
		}()
		return
	}

	for _, file := range cachedFiles {
		// Is an expired or missing file in cache data structure, if it is not a directory delete it
		item := cache.Sample(file.Name())
		if item == nil || item.Expired() {
			info, err := os.Stat(filepath.Join(backingDir, file.Name()))
			if err == nil {
				if info.IsDir() {
					continue
				}
				select {
				case removedC <- info:
				case <-time.After(time.Second):
				}
				if err = os.Remove(filepath.Join(backingDir, file.Name())); err != nil {
					select {
					case errorC <- errors.Wrap(err, fmt.Sprintf("cache dir %s remove failed", backingDir)).With("stack", stack.Trace().TrimRuntime()):
					case <-time.After(time.Second):
						fmt.Printf("%s\n", errors.Wrap(err, fmt.Sprintf("cache dir %s remove failed", backingDir)).With("stack", stack.Trace().TrimRuntime()))
					}
				}
			}
		}
	}
}

// groomDir will scan the in memory cache and if there are files that are on disk
// but not in the cache they will be reaped
//
func groomDir(ctx context.Context, backingDir string, removedC chan os.FileInfo, errorC chan errors.Error) {
	// Run the checker for dangling files at time that dont fall on obvious boundaries
	check := time.NewTicker(time.Duration(36 * time.Second))
	defer check.Stop()

	for {
		select {
		case <-check.C:
			groom(backingDir, removedC, errorC)

		case <-ctx.Done():
			return
		}
	}
}

// ClearObjStore can be used by clients to erase the contents of the object store cache
//
func ClearObjStore() (err errors.Error) {
	// The ccache works by having the in memory tracking cache as the record to truth.  if we
	// delete the files on disk then when they are fetched they will be invalidated.  If they expire
	// then nothing will be done by the groomer
	//
	cachedFiles, errGo := ioutil.ReadDir(backingDir)
	if errGo != nil {
		return errors.Wrap(errGo).With("backingDir", backingDir).With("stack", stack.Trace().TrimRuntime())
	}
	for _, file := range cachedFiles {
		if file.Name()[0] == '.' {
			continue
		}
		info, err := os.Stat(filepath.Join(backingDir, file.Name()))
		if err == nil {
			if info.IsDir() {
				continue
			}
			if err = os.Remove(filepath.Join(backingDir, file.Name())); err != nil {
				return errors.Wrap(err, fmt.Sprintf("cache dir %s remove failed", backingDir)).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}
	return nil
}

// ObjStoreFootPrint can be used to determine what the cxurrent footprint of the
// artifact cache is
//
func ObjStoreFootPrint() (max int64) {
	return cacheMax
}

// InitObjStore sets up the backing store for our object store cache.  The size specified
// can be any byte amount
//
func InitObjStore(ctx context.Context, backing string, size int64, removedC chan os.FileInfo, errorC chan errors.Error) (err errors.Error) {
	if len(backing) == 0 {
		// If we dont have a backing store dont start the cache
		return errors.New(fmt.Sprintf("cache '%s' directory does not exist", backing)).With("stack", stack.Trace().TrimRuntime())
	}

	// Also make sure that the specified directory actually exists
	if stat, errGo := os.Stat(backing); err != nil || !stat.IsDir() {
		return errors.Wrap(errGo, fmt.Sprintf("cache %s directory does not exist", backing)).With("stack", stack.Trace().TrimRuntime())
	}

	// Now load a list of the files in the cache directory which further checks
	// our ability to use the storage
	//
	cachedFiles, errGo := ioutil.ReadDir(backing)
	if errGo != nil {
		return errors.Wrap(errGo, fmt.Sprintf("cache %s directory not readable", backing)).With("stack", stack.Trace().TrimRuntime())
	}

	// Finally try to create and delete a working file
	id, errGo := shortid.Generate()
	if errGo != nil {
		return errors.Wrap(errGo, fmt.Sprintf("cache %s directory not writable", backing)).With("stack", stack.Trace().TrimRuntime())
	}
	tmpFile := filepath.Join(backing, id)

	errGo = ioutil.WriteFile(tmpFile, []byte{0}, 0600)
	if errGo != nil {
		return errors.Wrap(errGo, fmt.Sprintf("cache %s directory not writable", backing)).With("stack", stack.Trace().TrimRuntime())
	}
	os.Remove(tmpFile)

	// When the cache init is called we only want one caller at a time through and they
	// should only call the initializer function once, successfully, retries are permitted.
	//
	cacheInitSync.Lock()
	defer cacheInitSync.Unlock()

	if cache != nil {
		return errors.Wrap(err, "cache is already initialized").With("stack", stack.Trace().TrimRuntime())
	}

	// Registry the monitoring items for measurement purposes by external parties,
	// these are only activated if the caching is being used
	if errGo = prometheus.Register(cacheHits); errGo != nil {
		select {
		case errorC <- errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()):
		default:
		}
	}
	if errGo = prometheus.Register(cacheMisses); errGo != nil {
		select {
		case errorC <- errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()):
		default:
		}
	}
	select {
	case errorC <- errors.New("cache enabled").With("stack", stack.Trace().TrimRuntime()):
	default:
	}

	// Store the backing store directory for the cache
	backingDir = backing
	cacheMax = size

	// The backing store might have partial downloads inside it.  We should clear those, ignoring errors,
	// and then re-create the partial download directory
	partialDir := filepath.Join(backingDir, ".partial")
	os.RemoveAll(partialDir)

	if errGo = os.MkdirAll(partialDir, 0700); err != nil {
		return errors.Wrap(errGo, "unable to create the partial downloads dir ", partialDir).With("stack", stack.Trace().TrimRuntime())
	}

	// Size the cache appropriately, and track items that are in use through to their being released,
	// which prevents items being read from being groomed and then new copies of the same
	// data appearing
	cache = ccache.New(ccache.Configure().MaxSize(size).GetsPerPromote(1).ItemsToPrune(1))

	// Now populate the lookaside cache with the files found in the cache directory and their sizes
	for _, file := range cachedFiles {
		if file.IsDir() {
			continue
		}
		if file.Name()[0] != '.' {
			cache.Fetch(file.Name(), time.Hour*48,
				func() (interface{}, error) {
					return file, nil
				})
		}
	}

	// Now start the directory groomer
	cacheInit.Do(func() {
		go groomDir(ctx, backingDir, removedC, errorC)
	})

	return nil
}

func CacheProbe(key string) bool {
	return cache.Get(key) != nil && !cache.Get(key).Expired()
}

func (s *ObjStore) Hash(name string, timeout time.Duration) (hash string, err errors.Error) {
	return s.store.Hash(name, timeout)
}

func (s *ObjStore) Fetch(name string, unpack bool, output string, timeout time.Duration) (warns []errors.Error, err errors.Error) {
	// Check for meta data, MD5, from the upstream and then examine our cache for a match
	hash, err := s.store.Hash(name, timeout)
	if err != nil {
		return warns, err
	}

	// If there is no cache simply download the file, and so we supply a nil for the tap
	// for our tap
	if len(backingDir) == 0 {
		cacheMisses.With(prometheus.Labels{"host": host, "hash": hash}).Inc()
		return s.store.Fetch(name, unpack, output, nil, timeout)
	}

	// triggers LRU to elevate the item being retrieved
	if len(hash) != 0 {
		if item := cache.Get(hash); item != nil {
			if !item.Expired() {
				item.Extend(48 * time.Hour)
			}
		}
	}

	// If there is caching we should loop until we have a good file in the cache, and
	// if appropriate based on the contents of the partial download directory be doing
	// or waiting for the download to happen, respecting the notion that only one of
	// the waiters should be downloading actively
	//
	stopAt := time.Now().Add(timeout)
	downloader := false

	// Loop termination conditions include a timeout and successful completion
	// of the download
	for {
		// Examine the local file cache and use the file from there if present
		localName := filepath.Join(backingDir, hash)
		if _, errGo := os.Stat(localName); errGo == nil {
			spec := StoreOpts{
				Art: &Artifact{
					Qualified: fmt.Sprintf("file:///%s", localName),
				},
				Validate: true,
				Timeout:  timeout,
			}
			localFS, err := NewStorage(&spec)
			if err != nil {
				return warns, err
			}
			// Because the file is already in the cache we dont supply a tap here
			if w, err := localFS.Fetch(localName, unpack, output, nil, timeout); err == nil {
				cacheHits.With(prometheus.Labels{"host": host, "hash": hash}).Inc()
				return warns, nil
			} else {

				// Drops through to allow for a fresh download, after saving the errors
				// as warnings for the caller so that caching failures can be observed
				// and diagnosed
				for _, warn := range w {
					warns = append(warns, warn)
				}
				warns = append(warns, err)
			}
		}
		cacheMisses.With(prometheus.Labels{"host": host, "hash": hash}).Inc()

		if stopAt.Before(time.Now()) {
			if downloader {
				return warns, errors.New("timeout downloading artifact").With("stack", stack.Trace().TrimRuntime()).With("file", name)
			} else {
				return warns, errors.New("timeout waiting for artifact").With("stack", stack.Trace().TrimRuntime()).With("file", name)
			}
		}
		downloader = false

		// Look for partial downloads, if a downloader is found then wait for the file to appear
		// inside the main directory
		//
		partial := filepath.Join(backingDir, ".partial", hash)
		if _, errGo := os.Stat(partial); errGo == nil {
			select {
			case <-time.After(13 * time.Second):
			}
			continue
		}

		// If there is no partial file yet try to create a partial file with
		// the exclusive and create flags set which avoids two threads
		// creating the file on top of each other
		//
		file, errGo := os.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if errGo != nil {
			select {
			case s.ErrorC <- errors.Wrap(errGo, "file open failure").With("stack", stack.Trace().TrimRuntime()).With("file", partial):
			default:
			}
			select {
			case <-time.After(13 * time.Second):
			}
			continue
		}
		downloader = true

		tapWriter := bufio.NewWriter(file)

		// Having gained the file to download into call the fetch method and supply the io.WriteClose
		// to the concrete downloader
		//
		w, err := s.store.Fetch(name, unpack, output, tapWriter, timeout)

		tapWriter.Flush()
		file.Close()

		// Save warnings from intermediate components, even if there are no
		// unrecoverable errors
		for _, warn := range w {
			warns = append(warns, warn)
		}

		if err == nil {
			info, errGo := os.Stat(partial)
			if errGo == nil {
				cache.Fetch(info.Name(), time.Hour*48,
					func() (interface{}, error) {
						return info, nil
					})
			} else {
				select {
				case s.ErrorC <- errors.Wrap(errGo, "file cache failure").With("stack", stack.Trace().TrimRuntime()).With("file", partial).With("file", localName):
				default:
				}
			}
			// Move the downloaded file from .partial into our base cache directory,
			// and need to handle the file from the applications perspective is done
			// by the Fetch, if the rename files there is nothing we can do about it
			// so simply continue as the application will have the data anyway
			if errGo = os.Rename(partial, localName); errGo != nil {
				select {
				case s.ErrorC <- errors.Wrap(errGo, "file rename failure").With("stack", stack.Trace().TrimRuntime()).With("file", partial).With("file", localName):
				default:
				}
			}

			return warns, nil
		} else {
			select {
			case s.ErrorC <- err:
			default:
			}
		}
	} // End of for {}
}

func (s *ObjStore) Deposit(src string, dest string, timeout time.Duration) (warns []errors.Error, err errors.Error) {
	// Place an item into the cache
	return s.store.Deposit(src, dest, timeout)
}

func (s *ObjStore) Close() {
	s.store.Close()
}
