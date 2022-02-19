// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation of storage that can use an internal cache along with the MD5
// hash of the files contents to avoid downloads that are not needed.

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/leaf-ai/studio-go-runner/internal/request"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	"github.com/lthibault/jitterbug"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/karlmutch/ccache"
	"github.com/karlmutch/go-shortid"
)

var (
	host = ""

	cacheHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_cache_hits",
			Help: "Number of artifact cache hits.",
		},
		[]string{"host", "hash"},
	)
	cacheMisses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_cache_misses",
			Help: "Number of artifact cache misses.",
		},
		[]string{"host", "hash"},
	)
)

func init() {
	host, _ = os.Hostname()
}

type objStore struct {
	store  Storage
	ErrorC chan kv.Error
}

// NewObjStore is used to instantiate an object store for the running that includes a cache
//
func NewObjStore(ctx context.Context, spec *StoreOpts, errorC chan kv.Error) (oStore *objStore, err kv.Error) {
	store, err := NewStorage(ctx, spec)
	if err != nil {
		return nil, err
	}

	return &objStore{
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

func groom(backingDir string, removedC chan os.FileInfo, errorC chan kv.Error) {
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
			case errorC <- kv.Wrap(err, fmt.Sprintf("cache dir %s refresh failure", backingDir)).With("stack", stack.Trace().TrimRuntime()):
			case <-time.After(time.Second):
				fmt.Printf("%s\n", kv.Wrap(err, fmt.Sprintf("cache dir %s refresh failed", backingDir)).With("stack", stack.Trace().TrimRuntime()))
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
					case errorC <- kv.Wrap(err, fmt.Sprintf("cache dir %s remove failed", backingDir)).With("stack", stack.Trace().TrimRuntime()):
					case <-time.After(time.Second):
						fmt.Printf("%s\n", kv.Wrap(err, fmt.Sprintf("cache dir %s remove failed", backingDir)).With("stack", stack.Trace().TrimRuntime()))
					}
				}
			}
		}
	}
}

// groomDir will scan the in memory cache and if there are files that are on disk
// but not in the cache they will be reaped
//
func groomDir(ctx context.Context, backingDir string, removedC chan os.FileInfo, errorC chan kv.Error) (triggerC chan struct{}) {
	triggerC = make(chan struct{})

	go func() {
		check := NewTrigger(triggerC, time.Second*30, &jitterbug.Norm{Stdev: time.Second * 3})
		defer check.Stop()

		for {
			select {
			case <-check.C:
				groom(backingDir, removedC, errorC)

			case <-ctx.Done():
				return
			}
		}
	}()

	return triggerC
}

// ClearObjStore can be used by clients to erase the contents of the object store cache
//
func ClearObjStore() (err kv.Error) {
	// The ccache works by having the in memory tracking cache as the record to truth.  if we
	// delete the files on disk then when they are fetched they will be invalidated.  If they expire
	// then nothing will be done by the groomer
	//
	cachedFiles, errGo := ioutil.ReadDir(backingDir)
	if errGo != nil {
		return kv.Wrap(errGo).With("backingDir", backingDir).With("stack", stack.Trace().TrimRuntime())
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
				return kv.Wrap(err, fmt.Sprintf("cache dir %s remove failed", backingDir)).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}
	return nil
}

// ObjStoreFootPrint can be used to determine what the current footprint of the
// artifact cache is
//
func ObjStoreFootPrint() (max int64) {
	return cacheMax
}

// InitObjStore sets up the backing store for our object store cache.  The size specified
// can be any byte amount.
//
// The triggerC channel is functional when the err value is nil, this channel can be used to manually
// trigger the disk caching sub system
//
func InitObjStore(ctx context.Context, backing string, size int64, removedC chan os.FileInfo, errorC chan kv.Error) (triggerC chan<- struct{}, err kv.Error) {
	if len(backing) == 0 {
		// If we dont have a backing store dont start the cache
		return nil, kv.NewError("empty cache directory name").With("stack", stack.Trace().TrimRuntime())
	}

	// Also make sure that the specified directory actually exists
	if stat, errGo := os.Stat(backing); errGo != nil || !stat.IsDir() {
		if errGo != nil {
			return nil, kv.Wrap(errGo, "cache directory does not exist").With("backing", backing).With("stack", stack.Trace().TrimRuntime())
		}
		return nil, kv.NewError("cache name specified is not a directory").With("backing", backing).With("stack", stack.Trace().TrimRuntime())
	}

	// Now load a list of the files in the cache directory which further checks
	// our ability to use the storage
	//
	cachedFiles, errGo := ioutil.ReadDir(backing)
	if errGo != nil {
		return nil, kv.Wrap(errGo, "cache directory not readable").With("backing", backing).With("stack", stack.Trace().TrimRuntime())
	}

	// Finally try to create and delete a working file
	id, errGo := shortid.Generate()
	if errGo != nil {
		return nil, kv.Wrap(errGo, "cache directory not writable").With("backing", backing).With("stack", stack.Trace().TrimRuntime())
	}
	tmpFile := filepath.Join(backing, id)

	errGo = ioutil.WriteFile(tmpFile, []byte{0}, 0600)
	if errGo != nil {
		return nil, kv.Wrap(errGo, "cache directory not writable").With("backing", backing).With("stack", stack.Trace().TrimRuntime())
	}
	os.Remove(tmpFile)

	// When the cache init is called we only want one caller at a time through and they
	// should only call the initializer function once, successfully, retries are permitted.
	//
	cacheInitSync.Lock()
	defer cacheInitSync.Unlock()

	if cache != nil {
		return nil, kv.Wrap(err, "cache is already initialized").With("stack", stack.Trace().TrimRuntime())
	}

	// Registry the monitoring items for measurement purposes by external parties,
	// these are only activated if the caching is being used
	if errGo = prometheus.Register(cacheHits); errGo != nil {
		select {
		case errorC <- kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()):
		default:
		}
	}
	if errGo = prometheus.Register(cacheMisses); errGo != nil {
		select {
		case errorC <- kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()):
		default:
		}
	}

	select {
	case errorC <- kv.NewError("cache enabled").With("stack", stack.Trace().TrimRuntime()):
	default:
	}

	// Store the backing store directory for the cache
	backingDir = backing
	cacheMax = size

	DownloaderFactory.SetBackingDir(backingDir)

	// The backing store might have partial downloads inside it.  We should clear those, ignoring kv.
	// and then re-create the partial download directory
	partialDir := filepath.Join(backingDir, ".partial")
	os.RemoveAll(partialDir)

	if errGo = os.MkdirAll(partialDir, 0700); err != nil {
		return nil, kv.Wrap(errGo, "unable to create the partial downloads dir ", partialDir).With("stack", stack.Trace().TrimRuntime())
	}

	// Size the cache appropriately, and track items that are in use through to their being released,
	// which prevents items being read from being groomed and then new copies of the same
	// data appearing
	cache = ccache.New(ccache.Configure().MaxSize(size).GetsPerPromote(1).ItemsToPrune(1))

	// Now populate the look-aside cache with the files found in the cache directory and their sizes
	for i, file := range cachedFiles {
		if file.IsDir() {
			continue
		}
		if file.Name()[0] != '.' {
			cache.Fetch(file.Name(), time.Hour*48,
				func() (interface{}, error) {
					return cachedFiles[i], nil
				})
		}
	}

	// Now start the directory groomer
	cacheInit.Do(func() {
		triggerC = groomDir(ctx, backingDir, removedC, errorC)
	})

	return triggerC, nil
}

// CacheProbe can be used to test the validity of the cache for a previously cached item.
//
func CacheProbe(key string) bool {
	return cache.Get(key) != nil && !cache.Get(key).Expired()
}

func (s *objStore) reportErr(ctx context.Context, err kv.Error) (exit bool) {
	select {
	case s.ErrorC <- err:
	case <-ctx.Done():
		return true
	default:
	}
	return false
}

// Hash will return the hash of a stored file or other blob.  This method can be used
// by a caching layer or by a client to obtain the unique content based identity of the
// resource being stored.
//
func (s *objStore) Hash(ctx context.Context, name string) (hash string, err kv.Error) {
	return s.store.Hash(ctx, name)
}

// Gather is used to retrieve files prefixed with a specific key.  It is used to retrieve the individual files
// associated with a previous Hoard operation
//
func (s *objStore) Gather(ctx context.Context, keyPrefix string, outputDir string, maxBytes int64, failFast bool) (size int64, warnings []kv.Error, err kv.Error) {
	// Retrieve individual files, without using the cache, tap is set to nil
	return s.store.Gather(ctx, keyPrefix, outputDir, maxBytes, nil, failFast)
}

func (s *objStore) tryLocalCache(ctx context.Context, cacheName string, hash string,
	unpack bool, output string, maxBytes int64,
	firstCall bool) (gotIt bool, size int64, warns []kv.Error, err kv.Error) {
	tm := time.Now()
	if _, errGo := os.Stat(cacheName); errGo == nil {
		spec := StoreOpts{
			Art: &request.Artifact{
				Qualified: fmt.Sprintf("file:///%s", cacheName),
			},
			Validate: true,
		}
		localFS, err := NewStorage(ctx, &spec)
		if err != nil {
			return false, 0, warns, err
		}
		// Because the file is already in the cache we don't supply a tap here
		size, w, err := localFS.Fetch(ctx, cacheName, unpack, output, maxBytes, nil)
		if err == nil {
			if firstCall {
				cacheHits.With(prometheus.Labels{"host": host, "hash": hash}).Inc()
			}
			fmt.Printf("==========CACHE got local item: %s in %v millisec\n", cacheName, time.Now().Sub(tm).Milliseconds())
			return true, size, warns, nil
		}
		warns = append(warns, w...)
		return false, size, warns, err
	}
	if firstCall {
		cacheMisses.With(prometheus.Labels{"host": host, "hash": hash}).Inc()
	}
	return false, 0, warns, nil
}

// Fetch is used by client to retrieve resources from a concrete storage system.  This function will
// invoke storage system logic that may retrieve resources from a cache.
//
func (s *objStore) Fetch(ctx context.Context, name string, unpack bool, output string, maxBytes int64) (size int64, warns []kv.Error, err kv.Error) {

	// If there is no cache simply download the file, and so we supply a nil for the tap
	// for our tap
	if len(backingDir) == 0 {
		return s.store.Fetch(ctx, name, unpack, output, maxBytes, nil)
	}

	// Check for meta data, MD5, from the upstream and then examine our cache for a match
	hash, err := s.store.Hash(ctx, name)
	if err != nil {
		return 0, warns, err
	}

	cacheKey := hash + filepath.Ext(name)
	// triggers LRU to elevate the item being retrieved
	if len(cacheKey) != 0 {
		if item := cache.Get(cacheKey); item != nil {
			if !item.Expired() {
				item.Extend(48 * time.Hour)
			}
		}
	}

	// Construct local name for cache item,
	// preserving filename extension for correct file processing.
	localName := filepath.Join(backingDir, cacheKey)

	// Loop termination conditions include a timeout and successful completion
	// of the download
	attemptLocal := 0
	attemptDownload := 0
	maxAttempts := 3
	firstCall := true
	for {
		// Examine the local file cache and use the file from it if present
		gotIt, size, w, err := s.tryLocalCache(ctx, localName, hash, unpack, output, maxBytes, firstCall)
		firstCall = false
		warns = append(warns, w...)
		if gotIt {
			return size, warns, err
		}

		if err != nil {
			warns = append(warns, err)
			attemptLocal++
			if attemptLocal > maxAttempts {
				return 0, warns, kv.NewError("number of local cache retries exceeded").With("file", name).With("retry", attemptLocal).With("stack", stack.Trace().TrimRuntime())
			}
		}
		// That means local cache doesn't have this item, or failed to copy it to target location,
		// we proceed to downloading it:

		if ctx.Err() != nil {
			return 0, warns, kv.NewError("waiting for download terminated").With("stack", stack.Trace().TrimRuntime()).With("file", name)
		}

		attemptDownload++
		if attemptDownload > maxAttempts {
			return 0, warns, kv.NewError("number of download retries exceeded").With("file", name).With("retry", attemptDownload).With("stack", stack.Trace().TrimRuntime())
		}

		// Initiate fresh artifact download:
		tm := time.Now()
		downloader, err := DownloaderFactory.GetDownloader(ctx, s.store, cacheKey, name, unpack, maxBytes)
		if err != nil {
			return 0, warns, err
		}
		// Wait for downloader to finish:
		downloader.Wait()

		warns = append(warns, downloader.warnings...)
		if downloader.result == nil {
			dbgErr := kv.NewError(fmt.Sprintf("=====DOWNLOADED local item: %s for name: %s in %v millisec\n", localName, name, time.Now().Sub(tm).Milliseconds()))
			s.reportErr(ctx, dbgErr)

			// Our item has been put in local cache, cleanup used downloader
			// and repeat the main loop:
			if info, goErr := os.Stat(localName); goErr == nil {
				cache.Fetch(cacheKey, time.Hour*48,
					func() (interface{}, error) {
						return info, nil
					})
			} else {
				err = kv.Wrap(goErr).With("cache item", localName).With("stack", stack.Trace().TrimRuntime())
				if s.reportErr(ctx, err) {
					return 0, warns, err
				}
				warns = append(warns, err)
			}
			DownloaderFactory.RemoveDownloader(cacheKey)
		} else {
			if s.reportErr(ctx, downloader.result.With("stack", stack.Trace().TrimRuntime())) {
				return 0, warns, downloader.result
			}
			warns = append(warns, downloader.result)
			DownloaderFactory.RemoveDownloader(cacheKey)
		}
		select {
		case <-ctx.Done():
			return 0, warns, err
		default:
		}
	} // End of for {}
	// unreachable
}

// Hoard is used to place a directory with individual files into the storage resource within the storage implemented
// by a specific implementation.
//
func (s *objStore) Hoard(ctx context.Context, srcDir string, destPrefix string) (warns []kv.Error, err kv.Error) {
	// Place an item into the cache
	return s.store.Hoard(ctx, srcDir, destPrefix)
}

// Deposit is used to place a file or other storage resource within the storage implemented
// by a specific implementation.
//
func (s *objStore) Deposit(ctx context.Context, src string, dest string) (warns []kv.Error, err kv.Error) {
	// Place an item into the cache
	return s.store.Deposit(ctx, src, dest)
}

// Close is used to clean up any resources allocated to the storage by calling the implementation Close
// method.
//
func (s *objStore) Close() {
	s.store.Close()
}
