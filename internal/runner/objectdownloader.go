// Copyright 2018-2022 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation of artifact objects downloaders.

import (
	"context"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
	"os"
	"path/filepath"
	"sync"
)

type ObjDownloader struct {
	sync.WaitGroup
	store       Storage
	cacheKey    string
	remoteName  string
	partialName string
	localName   string
	unpack      bool
	maxBytes    int64
	dataSize    int64
	result      kv.Error
	warnings    []kv.Error
}

type ObjDownloaderFactory struct {
	sync.Mutex
	loaders    map[string]*ObjDownloader
	backingDir string
}

var (
	DownloaderFactory ObjDownloaderFactory
)

func init() {
	DownloaderFactory = ObjDownloaderFactory{
		loaders:    map[string]*ObjDownloader{},
		backingDir: "",
	}
}

func (f *ObjDownloaderFactory) SetBackingDir(dir string) {
	f.backingDir = dir
}

func (f *ObjDownloaderFactory) RemoveDownloader(key string) {
	f.Lock()
	defer f.Unlock()

	if _, isPresent := f.loaders[key]; isPresent {
		delete(f.loaders, key)
	}
}

func (f *ObjDownloaderFactory) GetDownloader(ctx context.Context, store Storage,
	key string, name string, unpack bool, maxBytes int64) (loader *ObjDownloader, err kv.Error) {
	f.Lock()
	defer f.Unlock()

	if loader, isPresent := f.loaders[key]; isPresent {
		return loader, nil
	}
	// Create new artifact loader
	loader = &ObjDownloader{
		store:       store,
		cacheKey:    key,
		remoteName:  name,
		partialName: filepath.Join(f.backingDir, ".partial", key),
		localName:   filepath.Join(f.backingDir, key),
		unpack:      unpack,
		maxBytes:    maxBytes,
		dataSize:    0,
		result:      nil,
		warnings:    []kv.Error{},
	}
	loader.Add(1)
	go loader.download(ctx)
	f.loaders[key] = loader
	return loader, nil
}

func (d *ObjDownloader) cleanupPartial() {
	if errGo := os.Remove(d.partialName); errGo != nil {
		warn := kv.Wrap(errGo).With("partial", d.partialName, "file", d.remoteName, "stack", stack.Trace().TrimRuntime())
		d.warnings = append(d.warnings, warn)
	}
}

func (d *ObjDownloader) download(ctx context.Context) {
	var w []kv.Error
	d.dataSize, w, d.result = d.store.Fetch(ctx, d.remoteName, d.unpack, d.partialName, d.maxBytes, nil)
	d.warnings = append(d.warnings, w...)
	if d.result == nil {
		// Move our "partial" downloaded artifact to proper cache location
		if errGo := os.Rename(d.partialName, d.localName); errGo != nil {
			d.result = kv.Wrap(errGo, "file rename failure").With("stack", stack.Trace().TrimRuntime()).With("from", d.partialName).With("to", d.localName)
			d.cleanupPartial()
		}
	} else {
		d.cleanupPartial()
	}
	d.Done()
}
