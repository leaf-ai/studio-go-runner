package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/minio/minio-go/v7"
)

// This file contains the catalog for a model advertised as available for
// serving

type model struct {
	obj   *minio.ObjectInfo            // The S3 information for the index blob
	blobs map[string]*minio.ObjectInfo // Blobs that are referenced by the index
}

// NewModel will initialize a new model data structure
func NewModel() (m *model) {
	return &model{}
}

// Load is used to initialize an in memory representation of a model index obtained from the S3
// backing store
//
// obj.ETag can be used to skip the loading of the model index if the index has not changed from
// the value in knownTag.  If this is the case then a nil will returned
//
func (m *model) Load(ctx context.Context, client *minio.Client, bucket string, objInfo *minio.ObjectInfo, capacityLimit uint64) (err kv.Error) {

	// Load the index blob contents into an array n x 2 via a Buffer and then parsed using
	// the go csv encoder/decoder
	obj, errGo := client.GetObject(ctx, bucket, objInfo.Key, minio.GetObjectOptions{})
	if errGo != nil {
		return kv.Wrap(errGo).With("bucket", bucket, "key", objInfo.Key).With("stack", stack.Trace().TrimRuntime())
	}

	buffer := &bytes.Buffer{}
	n, errGo := io.CopyN(buffer, obj, objInfo.Size)
	if errGo != nil {
		return kv.Wrap(errGo).With("bucket", bucket, "key", objInfo.Key, "size", humanize.Bytes(uint64(objInfo.Size))).With("stack", stack.Trace().TrimRuntime())
	}
	if n != objInfo.Size {
		return kv.NewError("index size error").With("size", humanize.Bytes(uint64(objInfo.Size)), "read_size", humanize.Bytes(uint64(n))).With("stack", stack.Trace().TrimRuntime())
	}

	r := csv.NewReader(strings.NewReader(buffer.String()))
	entries, errGo := r.ReadAll()
	if errGo != nil {
		return kv.Wrap(errGo).With("bucket", bucket, "key", objInfo.Key).With("stack", stack.Trace().TrimRuntime())
	}

	// Check each key is valid and populate the indexes blob map
	newBlobs := make(map[string]*minio.ObjectInfo, len(entries))

	for _, entry := range entries {
		key := entry[0]

		info, errGo := client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
		if errGo != nil {
			return kv.Wrap(errGo).With("bucket", bucket, "key", key).With("stack", stack.Trace().TrimRuntime())
		}
		newBlobs[key] = &info
	}

	m.blobs = newBlobs

	return nil
}
