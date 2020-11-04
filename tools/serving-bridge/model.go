package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"strings"
	"sync"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/minio/minio-go/v7"
)

// This file contains the catalog for a model advertised as available for
// serving

type model struct {
	obj     *minio.ObjectInfo            // The S3 information for the index blob
	baseDir string                       // The base directory for the model
	blobs   map[string]*minio.ObjectInfo // Blobs that are referenced by their index
}

type indexes struct {
	models map[string]map[string]*model
	sync.Mutex
}

var (
	knownIndexes = &indexes{
		models: map[string]map[string]*model{}, // The list of known index files and their etags
	}
)

func GetModelIndex() (index *indexes) {
	return knownIndexes
}

func (index *indexes) Add(endpoint string, modelKey string, mdl *model) {
	index.Lock()
	defer index.Unlock()
	if _, isPresent := index.models[endpoint]; !isPresent {
		index.models[endpoint] = map[string]*model{}
	}
	index.models[endpoint][modelKey] = mdl
}

func (index *indexes) GetBases() (bases []string, err kv.Error) {
	index.Lock()
	defer index.Unlock()

	for _, endpoint := range index.models {
		for _, mdl := range endpoint {
			bases = append(bases, mdl.baseDir)
		}
	}
	return bases, nil
}

func (index *indexes) Get(endpoint string, modelKey string) (mdl *model) {
	index.Lock()
	defer index.Unlock()
	if _, isPresent := index.models[endpoint]; !isPresent {
		return nil
	}
	mdl, isPresent := index.models[endpoint][modelKey]
	if !isPresent {
		return nil
	}
	return mdl
}

func (index *indexes) Set(endpoint string, modelKey string, etag string) (set bool) {
	mdl := index.Get(endpoint, modelKey)
	if mdl == nil {
		return false
	}
	mdl.obj.ETag = etag
	return true
}

// Groom we will examine the models for an endpoint looking for those models that
// are not present in the modelKeys collection and delete the endpoint entries that
// are not present in the modelKeys collection.  It is used to groom indexes of
// defunct entries and will return a collection of the deleted entries.
//
// If an error does occur and is returned the deleted variable returned will
// contain any deleted entries that had already been processed.
func (index *indexes) Groom(endpoint string, modelKeys map[string]minio.ObjectInfo) (err kv.Error) {
	index.Lock()
	defer index.Unlock()

	for key, _ := range index.models[endpoint] {
		if _, isPresent := modelKeys[key]; !isPresent {
			delete(index.models[endpoint], key)
		}
	}
	return nil
}

func (index *indexes) Delete(endpoint string, modelKey string) {
	index.Lock()
	defer index.Unlock()
	if _, isPresent := index.models[endpoint]; !isPresent {
		return
	}
	delete(index.models[endpoint], modelKey)
	if len(index.models[endpoint]) == 0 {
		delete(index.models, endpoint)
	}
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
	if _, errGo = io.Copy(buffer, obj); errGo != nil {
		return kv.Wrap(errGo).With("bucket", bucket, "key", objInfo.Key).With("stack", stack.Trace().TrimRuntime())
	}

	r := csv.NewReader(strings.NewReader(buffer.String()))
	entries, errGo := r.ReadAll()
	if errGo != nil {
		return kv.Wrap(errGo).With("bucket", bucket, "key", objInfo.Key).With("stack", stack.Trace().TrimRuntime())
	}

	// Check each key is valid and populate the indexes blob map
	newBlobs := make(map[string]*minio.ObjectInfo, len(entries))

	// Reset the base directory when re-reading the index
	m.baseDir = ""

	for _, entry := range entries {
		key := entry[1]

		info, errGo := client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
		if errGo != nil {
			return kv.Wrap(errGo).With("bucket", bucket, "key", key).With("stack", stack.Trace().TrimRuntime())
		}
		newBlobs[key] = &info

		// See if the base directory has been saved and if not save it
		if len(m.baseDir) == 0 {
			m.baseDir = entry[0]
		} else {
			if m.baseDir != entry[0] {
				return kv.NewError("base path mismatched within model").With("bucket", bucket, "key", key).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}

	m.blobs = newBlobs

	return nil
}
