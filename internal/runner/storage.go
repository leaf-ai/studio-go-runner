// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation for the storage sub system that will
// be used by the runner to retrieve storage from cloud providers or localized storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/leaf-ai/go-service/pkg/s3"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

// Storage defines an interface for implementations of a studioml artifact store
//
type Storage interface {
	// Fetch will retrieve contents of the named storage object using a prefix treating any items retrieved as individual files
	//
	Gather(ctx context.Context, keyPrefix string, outputDir string, tap io.Writer) (warnings []kv.Error, err kv.Error)

	// Fetch will retrieve contents of the named storage object and optionally unpack it into the
	// user specified output directory
	//
	Fetch(ctx context.Context, name string, unpack bool, output string, tap io.Writer) (warnings []kv.Error, err kv.Error)

	// Hoard will take a number of files for upload, deduplication is implemented outside of this interface
	//
	Hoard(ctx context.Context, srcDir string, keyPrefix string) (warnings []kv.Error, err kv.Error)

	// Deposit is a directory archive and upload, deduplication is implemented outside of this interface
	//
	Deposit(ctx context.Context, src string, dest string) (warnings []kv.Error, err kv.Error)

	// Hash can be used to retrieve the hash of the contents of the file.  The hash is
	// retrieved not computed and so is a lightweight operation common to both S3 and Google Storage.
	// The hash on some storage platforms is not a plain MD5 but uses multiple hashes from file
	// segments to increase the speed of hashing and also to reflect the multipart download
	// processing that was used for the file, for a full explanation please see
	// https://stackoverflow.com/questions/12186993/what-is-the-algorithm-to-compute-the-amazon-s3-etag-for-a-file-larger-than-5gb
	//
	Hash(ctx context.Context, name string) (hash string, err kv.Error)

	Close()
}

// StoreOpts is used to encapsulate a storage implementation with the runner and studioml data needed
//
type StoreOpts struct {
	Art       *Artifact
	ProjectID string
	Group     string
	Creds     string // The credentials file name
	Env       map[string]string
	Validate  bool
}

// NewStorage is used to create a receiver for a storage implementation
//
func NewStorage(ctx context.Context, spec *StoreOpts) (stor Storage, err kv.Error) {

	if spec == nil {
		return nil, kv.Wrap(err, "empty specification supplied").With("stack", stack.Trace().TrimRuntime())
	}

	uri, errGo := url.ParseRequestURI(spec.Art.Qualified)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	switch uri.Scheme {
	case "gs":
		return NewGSstorage(ctx, spec.ProjectID, spec.Creds, spec.Env, spec.Art.Bucket, spec.Validate)
	case "s3":
		uriPath := strings.Split(uri.EscapedPath(), "/")
		if len(spec.Art.Key) == 0 {
			spec.Art.Key = strings.Join(uriPath[2:], "/")
		}
		if len(spec.Art.Bucket) == 0 {
			spec.Art.Bucket = uriPath[1]
		}

		if len(uri.Host) == 0 {
			return nil, kv.NewError("S3/minio endpoint lacks a scheme, or the host name was not specified").With("stack", stack.Trace().TrimRuntime())
		}

		useSSL := uri.Scheme == "https"

		return s3.NewS3storage(ctx, spec.Creds, spec.Env, uri.Host,
			spec.Art.Bucket, spec.Art.Key, spec.Validate, useSSL)

	case "file":
		return NewLocalStorage()
	default:
		return nil, kv.NewError(fmt.Sprintf("unknown, or unsupported URI scheme %s, s3 or gs expected", uri.Scheme)).With("stack", stack.Trace().TrimRuntime())
	}
}
