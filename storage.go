package runner

// This file contains the implementation for the storage sub system that will
// be used by the runner to retrieve storage from cloud providers or localized storage

import (
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

type Storage interface {
	// Retrieve contents of the named storage object and optionally unpack it into the
	// user specified output directory
	//
	Fetch(name string, unpack bool, output string, tap io.Writer, timeout time.Duration) (err errors.Error)

	// File upload, deduplication is implemented outside of this interface
	//
	Deposit(src string, dest string, timeout time.Duration) (err errors.Error)

	// Hash can be used to retrive the hash of the contents of the file.  The hash is
	// retrieved not computed and so is a lightweight operation common to both S3 and Google Storage.
	// The hash on some storage platforms is not a plain MD5 but uses multiple hashes from file
	// segments to increase the speed of hashing and also to reflect the multipart download
	// processing that was used for the file, for a full explanation please see
	// https://stackoverflow.com/questions/12186993/what-is-the-algorithm-to-compute-the-amazon-s3-etag-for-a-file-larger-than-5gb
	//
	Hash(name string, timeout time.Duration) (hash string, err errors.Error)

	Close()
}

type StoreOpts struct {
	Art       *Artifact
	ProjectID string
	Group     string
	Creds     string // The credentials file name
	Env       map[string]string
	Validate  bool
	Timeout   time.Duration
}

func NewStorage(spec *StoreOpts) (stor Storage, err errors.Error) {

	if spec == nil {
		return nil, errors.Wrap(err, "empty specification supplied").With("stack", stack.Trace().TrimRuntime())
	}

	uri, errGo := url.ParseRequestURI(spec.Art.Qualified)
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	switch uri.Scheme {
	case "gs":
		return NewGSstorage(spec.ProjectID, spec.Creds, spec.Env, spec.Art.Bucket, spec.Validate, spec.Timeout)
	case "s3":
		uriPath := strings.Split(uri.EscapedPath(), "/")
		if len(spec.Art.Key) == 0 {
			spec.Art.Key = uriPath[2]
		}
		if len(spec.Art.Bucket) == 0 {
			spec.Art.Bucket = uriPath[1]
		}
		return NewS3storage(spec.ProjectID, spec.Creds, spec.Env, uri.Host, spec.Art.Bucket, spec.Art.Key, spec.Validate, spec.Timeout)
	case "file":
		return NewLocalStorage()
	default:
		return nil, errors.New(fmt.Sprintf("unknown, or unsupported URI scheme %s, s3 or gs expected", uri.Scheme)).With("stack", stack.Trace().TrimRuntime())
	}
}

// IsTar is used to test the extension to see if the presence of tar can be found
//
func IsTar(name string) bool {
	switch {
	case strings.Contains(name, ".tar."):
		return true
	case strings.HasSuffix(name, ".tgz"):
		return true
	case strings.HasSuffix(name, ".tar"):
		return true
	}
	return false
}

// MimeFromExt is used to characterize a mime type from a files extension
//
func MimeFromExt(name string) (fileType string) {
	switch filepath.Ext(name) {
	case ".gzip", ".gz":
		return "application/x-gzip"
	case ".zip":
		return "application/zip"
	case ".tgz": // Non standard extension as a result of staduioml python code
		return "application/bzip2"
	case ".tb2", ".tbz", ".tbz2", ".bzip2", ".bz2": // Standard bzip2 extensions
		return "application/bzip2"
	case ".tar":
		return "application/tar"
	default:
		return "application/octet-stream"
	}
}
