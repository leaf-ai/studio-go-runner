package runner

// This file contains the implementation of artifacts that exist as a directory containing
// files on a file system or archives on a cloud storage style platform.
//
// artifacts can be watched for changes and transfers between a file system and
// storage platforms based upon their contents changing etc
//
import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	hasher "github.com/karlmutch/hashstructure"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

// ArtifactCache is used to encapsulate and store hashes, typically file hashes, and
// prevent duplicated uploads from occurring needlessly
//
type ArtifactCache struct {
	upHashes map[string]uint64
	sync.Mutex

	// This can be used by the application layer to receive diagnostic and other information
	// about errors occurring inside the caching tracker etc and surface these errors etc to
	// the logging system
	ErrorC chan errors.Error
}

// NewArtifactCache initializes an hash tracker for artifact related files and
// passes it back to the caller.  The tracking structure can be used to track
// files that already been downloaded / uploaded and also includes a channel
// that can be used to receive error notifications
//
func NewArtifactCache() (cache *ArtifactCache) {
	return &ArtifactCache{
		upHashes: map[string]uint64{},
		ErrorC:   make(chan errors.Error),
	}
}

// Close will clean up the cache of hashes and close the error reporting channel
// associated with the cache tracker
//
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

func readAllHash(dir string) (hash uint64, err errors.Error) {
	files := []os.FileInfo{}
	dirs := []string{dir}
	for {
		newDirs := []string{}
		for _, aDir := range dirs {
			items, errGo := ioutil.ReadDir(aDir)
			if errGo != nil {
				return 0, errors.Wrap(errGo, fmt.Sprintf("failed to hash dir %s", aDir)).With("stack", stack.Trace().TrimRuntime())
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

	hash, errGo := hasher.Hash(files, nil)
	if errGo != nil {
		return 0, errors.Wrap(errGo, fmt.Sprintf("failed to hash files")).With("stack", stack.Trace().TrimRuntime())
	}
	return hash, nil
}

// Hash is used to obtain the hash of an artifact from the backing store implementation
// being used by the storage implementation
//
func (cache *ArtifactCache) Hash(ctx context.Context, art *Artifact, projectId string, group string, cred string, env map[string]string, dir string) (hash string, err errors.Error) {

	errors := errors.With("artifact", fmt.Sprintf("%#v", *art)).With("project", projectId).With("group", group)

	storage, err := NewObjStore(
		ctx,
		&StoreOpts{
			Art:       art,
			ProjectID: projectId,
			Group:     group,
			Creds:     cred,
			Env:       env,
			Validate:  true,
		},
		cache.ErrorC)

	if err != nil {
		return "", errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
	}

	defer storage.Close()
	return storage.Hash(ctx, art.Key)
}

// Fetch can be used to retrieve an artifact from a storage layer implementation, while
// passing through the lens of a caching filter that prevents unneeded downloads.
//
func (cache *ArtifactCache) Fetch(ctx context.Context, art *Artifact, projectId string, group string, cred string, env map[string]string, dir string) (warns []errors.Error, err errors.Error) {

	errors := errors.With("artifact", fmt.Sprintf("%#v", *art)).With("project", projectId).With("group", group)

	// Process the qualified URI and use just the path for now
	dest := filepath.Join(dir, group)
	if errGo := os.MkdirAll(dest, 0700); errGo != nil {
		return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("dest", dest)
	}

	storage, err := NewObjStore(
		ctx,
		&StoreOpts{
			Art:       art,
			ProjectID: projectId,
			Group:     group,
			Creds:     cred,
			Env:       env,
			Validate:  true,
		},
		cache.ErrorC)

	if err != nil {
		return warns, errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
	}

	if art.Unpack && !IsTar(art.Key) {
		return warns, errors.New("the unpack flag was set for an unsupported file format (tar gzip/bzip2 only supported)").With("stack", stack.Trace().TrimRuntime())
	}

	switch group {
	case "_metadata":
		warns, err = storage.Gather(ctx, "metadata/", dest)
	default:
		warns, err = storage.Fetch(ctx, art.Key, art.Unpack, dest)
	}
	storage.Close()

	if err != nil {
		return warns, errors.Wrap(err)
	}

	// Immutable artifacts need just to be downloaded and nothing else
	if !art.Mutable && !strings.HasPrefix(art.Qualified, "file://") {
		return warns, nil
	}

	if cache == nil {
		return warns, nil
	}

	if err = cache.updateHash(dest); err != nil {
		return warns, errors.Wrap(err)
	}

	return warns, nil
}

func (cache *ArtifactCache) updateHash(dir string) (err errors.Error) {
	hash, errGo := readAllHash(dir)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("dir", dir)
	}

	// Having obtained the artifact if it is mutable then we add a set of upload area hashes for all files and directories the artifact included
	cache.Lock()
	cache.upHashes[dir] = hash
	cache.Unlock()

	return nil
}

func (cache *ArtifactCache) checkHash(dir string) (isValid bool, err errors.Error) {

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

// Local returns the local disk based file name for the artifacts expanded archive files
//
func (cache *ArtifactCache) Local(group string, dir string, file string) (fn string, err errors.Error) {
	fn = filepath.Join(dir, group, file)
	if _, errOs := os.Stat(fn); errOs != nil {
		return "", errors.Wrap(errOs).With("stack", stack.Trace().TrimRuntime())
	}
	return fn, nil
}

// Restore the artifacts that have been marked mutable and that have changed
//
func (cache *ArtifactCache) Restore(ctx context.Context, art *Artifact, projectId string, group string, cred string, env map[string]string, dir string) (uploaded bool, warns []errors.Error, err errors.Error) {

	// Immutable artifacts need just to be downloaded and nothing else
	if !art.Mutable {
		return false, warns, nil
	}

	errors := errors.With("artifact", fmt.Sprintf("%#v", *art)).With("project", projectId).With("group", group).With("dir", dir)

	source := filepath.Join(dir, group)
	isValid, err := cache.checkHash(source)
	if err != nil {
		return false, warns, errors.Wrap(err).With("group", group, "stack", stack.Trace().TrimRuntime())
	}
	if isValid {
		return false, warns, nil
	}

	storage, err := NewObjStore(
		ctx,
		&StoreOpts{
			Art:       art,
			ProjectID: projectId,
			Creds:     cred,
			Env:       env,
			Validate:  true,
		},
		cache.ErrorC)
	if err != nil {
		return false, warns, err
	}
	defer storage.Close()

	// Check to see if the cache has a hash for the directory that has changed and
	// needs uploading
	//

	hash, errHash := readAllHash(dir)

	switch group {
	case "_metadata":
		if warns, err = storage.Hoard(ctx, source, "metadata"); err != nil {
			return false, warns, err.With("group", group)
		}
	default:
		if warns, err = storage.Deposit(ctx, source, art.Key); err != nil {
			return false, warns, err.With("group", group)
		}
	}

	if errHash == nil {
		// Having obtained the artifact if it is mutable then we add a set of upload area hashes for all files and directories the artifact included
		cache.Lock()
		cache.upHashes[dir] = hash
		cache.Unlock()
	}

	return true, warns, nil
}
