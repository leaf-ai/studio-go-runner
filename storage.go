package runner

// This file contains the implementation for the storage sub system that will
// be used by the runner to retrieve storage from cloud providers or localized storage

import (
	"fmt"
	"io"
	"net/url"
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
	Art       *Modeldir
	ProjectID string
	Env       map[string]string
	Validate  bool
	Timeout   time.Duration
}

func NewStorage(spec *StoreOpts) (stor Storage, err errors.Error) {

	errors := errors.With("artifact", fmt.Sprintf("%#v", *spec.Art)).With("project", spec.ProjectID)

	if spec == nil {
		return nil, errors.Wrap(err, "empty specification supplied").With("stack", stack.Trace().TrimRuntime())
	}

	uri, errGo := url.ParseRequestURI(spec.Art.Qualified)
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	switch uri.Scheme {
	case "gs":
		stor, err = NewGSstorage(spec.ProjectID, spec.Env, spec.Art.Bucket, spec.Validate, spec.Timeout)
	case "s3":
		stor, err = NewS3storage(spec.ProjectID, spec.Env, uri.Host, spec.Art.Bucket, spec.Validate, spec.Timeout)
	default:
		return nil, errors.Wrap(fmt.Errorf("unknown, or unsupported URI scheme %s, s3 or gs expected", uri.Scheme)).With("stack", stack.Trace().TrimRuntime())
	}

	if err != nil {
		return nil, errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
	}

	return stor, nil
}
