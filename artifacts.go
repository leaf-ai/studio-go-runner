package runner

// This file contains the implementation of artifacts that exist as a directory containing
// files on a file system or archives on a cloud storage style platform.
//
// artifacts can be watched for changes and transfers between a file system and
// storage platforms based upon their contents changing etc
//
import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	hasher "github.com/karlmutch/hashstructure"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

type ArtifactCache struct {
	upHashes map[string]uint64
	sync.Mutex

	// This can be used by the application layer to receive diagnostic and other information
	// about errors occuring inside the caching layers etc and surface these errors etc to
	// the logging system
	ErrorC chan errors.Error
}

func NewArtifactCache() (cache *ArtifactCache) {
	return &ArtifactCache{
		upHashes: map[string]uint64{},
		ErrorC:   make(chan errors.Error),
	}
}

func (cache *ArtifactCache) Close() {

	if cache.ErrorC != nil {
		defer func() {
			// Closing a close channel could cause a panic which is
			// acceptable while tearing down the cache
			recover()
		}()

		close(cache.ErrorC)
	}
}

func readAllHash(dir string) (hash uint64, err error) {
	files := []os.FileInfo{}
	dirs := []string{dir}
	for {
		newDirs := []string{}
		for _, aDir := range dirs {
			items, err := ioutil.ReadDir(aDir)
			if err != nil {
				return 0, errors.Wrap(err, fmt.Sprintf("failed to hash dir %s", aDir)).With("stack", stack.Trace().TrimRuntime())
			}
			for _, info := range items {
				if info.IsDir() {
					newDirs = append(newDirs, filepath.Join(aDir, info.Name()))
				}
				files = append(files, info)
			}
		}
		dirs = newDirs
		if len(dirs) == 0 {
			break
		}
	}

	return hasher.Hash(files, nil)
}

func (cache *ArtifactCache) Fetch(art *Modeldir, projectId string, group string, env map[string]string, dir string) (err error) {

	errors := errors.With("artifact", fmt.Sprintf("%#v", *art)).With("project", projectId)

	// Process the qualified URI and use just the path for now
	dest := filepath.Join(dir, group)
	if err = os.MkdirAll(dest, 0777); err != nil {
		return errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
	}

	storage, err := NewObjStore(
		&StoreOpts{
			Art:       art,
			ProjectID: projectId,
			Env:       env,
			Validate:  true,
			Timeout:   time.Duration(15 * time.Second),
		},
		cache.ErrorC)

	if err != nil {
		return errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
	}

	if err = storage.Fetch(art.Key, true, dest, 5*time.Second); err != nil {
		return errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
	}

	storage.Close()

	// Immutable artifacts need just to be downloaded and nothing else
	if !art.Mutable {
		return nil
	}

	if cache == nil {
		return
	}

	if err = cache.updateHash(dest); err != nil {
		return err
	}

	return nil
}

func (cache *ArtifactCache) updateHash(dir string) (err error) {
	hash, err := readAllHash(dir)
	if err != nil {
		return errors.Wrap(err).With("stack", stack.Trace().TrimRuntime()).With("dir", dir)
	}

	// Having obtained the artifact if it is mutable then we add a set of upload area hashes for all files and directories the artifact included
	cache.Lock()
	cache.upHashes[dir] = hash
	cache.Unlock()

	return nil
}

func (cache *ArtifactCache) checkHash(dir string) (isValid bool, err error) {

	cache.Lock()
	defer cache.Unlock()

	oldHash, isPresent := cache.upHashes[dir]

	if !isPresent {
		return false, nil
	}

	hash, err := readAllHash(dir)
	if err != nil {
		return false, err
	}
	return oldHash == hash, nil
}

// Restores the artifacts that have been marked mutable and that have changed
//
func (cache *ArtifactCache) Restore(art *Modeldir, projectId string, group string, env map[string]string, dir string) (uploaded bool, err error) {

	// Immutable artifacts need just to be downloaded and nothing else
	if !art.Mutable {
		return false, nil
	}

	errors := errors.With("artifact", fmt.Sprintf("%#v", *art)).With("project", projectId)

	source := filepath.Join(dir, group)
	isValid, err := cache.checkHash(source)
	if err != nil {
		return false, errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
	}
	if isValid {
		return false, nil
	}

	storage, err := NewObjStore(
		&StoreOpts{
			Art:       art,
			ProjectID: projectId,
			Env:       env,
			Validate:  true,
			Timeout:   time.Duration(15 * time.Second),
		},
		cache.ErrorC)
	if err != nil {
		return false, errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
	}
	defer storage.Close()

	// Check to see if the cache has a hash for the directory that has changed and
	// needs uploading
	//

	hash, errHash := readAllHash(dir)

	if err = storage.Deposit(source, art.Key, 5*time.Minute); err != nil {
		return false, errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
	}

	if errHash == nil {
		// Having obtained the artifact if it is mutable then we add a set of upload area hashes for all files and directories the artifact included
		cache.Lock()
		cache.upHashes[dir] = hash
		cache.Unlock()
	}

	return true, nil
}
