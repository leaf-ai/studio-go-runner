// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the implementation of a long lived component that scans for changes to the
// index files placed into S3 folder(s) and loads these into an in memory collection of known indexes

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/leaf-ai/go-service/pkg/log"
	"github.com/mitchellh/copystructure"

	"github.com/cenkalti/backoff/v4"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/label"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/dustin/go-humanize"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

const (
	minimumScanRate = time.Duration(3 * time.Second)

	largestIndexSize = 5 * 1024 * 1024 // The largest permissible index file size is 5MB

	indexPrefix = "index-"
	indexSuffix = ".csv"

	tracerName = "studio.ml/scanner"
)

var (
	bucketKey = label.Key("studio.ml/bucket")

	indexStartSync = make(chan struct{})
	indexEndSync   = make(chan struct{})
)

// IndexScanWait will block the caller until at least one complete update cycle
// is done
func IndexScanWait(ctx context.Context) {

	select {
	case <-ctx.Done():
	case <-indexStartSync:
	}

	select {
	case <-ctx.Done():
	case <-indexEndSync:
	}
}

// serviceIndexes will on a regular interval check for new index-* files at a well known location
// and if are new, modified or deleted based on the state inside a tensorflow model serving configuration
// will dispatch a function to apply them to the configuration file
//
func serviceIndexes(ctx context.Context, cfgUpdater *Listeners, retries *backoff.ExponentialBackOff, logger *log.Logger) {
	if retries.InitialInterval < minimumScanRate {
		retries.InitialInterval = minimumScanRate
		logger.Warn("specified scan interval too small, set to minimum", "retries", retries)
	}

	// Define a validation function for this component is be able to begin running
	// that tests for completeness of the first received configuration updates
	readyF := func(ctx context.Context, cfg Config, logger *log.Logger) (isValid bool) {
		if len(cfg.endpoint) == 0 {
			logger.Trace("endpoint configuration is empty", "stack", stack.Trace().TrimRuntime())
			return false
		}
		_, _, errGo := net.SplitHostPort(cfg.endpoint)
		if errGo != nil {
			logger.Debug("debug", "", strings.Replace(SpewSmall.Sdump(cfg), "\n", "", -1), "err", errGo.Error(), "stack", stack.Trace().TrimRuntime())
		}

		return errGo == nil
	}

	cfg, updatedCfgC := cfgWatcherStart(ctx, cfgUpdater, readyF, logger)

	cycleIndexes(ctx, cfg, updatedCfgC, retries, logger)
}

// cfgIndexes is used to receive configuration updates and wait for an initially valid configuration.
//
// The caller supplies a validation function, isReady, this performs the check of the configuration
// for the first update.  This function will block until that first valid update is found.
//
// Once past the first check the configuration watcher will then return after starting an asynchronous
// update notifier.
//
func cfgWatcherStart(ctx context.Context, cfgUpdater *Listeners,
	isReady func(ctx context.Context, cfg Config, logger *log.Logger) (readyForUse bool),
	logger *log.Logger) (cfg Config, updatedCfgC chan Config) {

	updatedCfgC = make(chan Config, 1)
	// Only close the configuration update channel when the server is terminating
	go func() {
		<-ctx.Done()
		close(updatedCfgC)
	}()

	if cfgUpdater != nil {
		updaterHndl, err := cfgUpdater.Add(updatedCfgC)
		if err != nil {
			logger.Warn("dynamic configuration changes not supported in the current deployment")
		} else {
			defer cfgUpdater.Delete(updaterHndl)
		}
	}

	// Even if the cfg does not change there might be an external state change that
	// brings the sydtem into a ready state so we should recheck everynow and again
	refresh := time.NewTicker(30 * time.Second)
	defer refresh.Stop()

	// Before starting make sure we get at least the starting configuration
	cfg = Config{}
	func() {
		for {
			select {
			case <-refresh.C:
				if isReady(ctx, cfg, logger) {
					return
				}
			case cfg = <-updatedCfgC:
				if isReady(ctx, cfg, logger) {
					return
				}
			case <-ctx.Done():
				logger.Warn("indexer could not be started using an initial configuration before the server was terminated")
				return
			}
		}
	}()
	return cfg, updatedCfgC
}

type safeConfig struct {
	cfg *Config
	sync.Mutex
}

func cycleIndexes(ctx context.Context, cfg Config, updatedCfgC chan Config, retries *backoff.ExponentialBackOff, logger *log.Logger) {

	_, span := otel.Tracer(tracerName).Start(ctx, "cycle-indexes")
	defer func() {
		defer func() {
			recover()
		}()
		span.End()
	}()

	sharedCfg := &safeConfig{
		cfg: &cfg,
	}

	logger.Debug("cycleIndexes cfg starting", "endpoint", sharedCfg.endpoint)
	go func(ctx context.Context, sharedCfg *safeConfig) {
		for {
			select {
			case cfg := <-updatedCfgC:
				cpy, errGo := copystructure.Copy(cfg)
				if errGo != nil {
					logger.Warn("updated configuration could not be used", "error", errGo.Error(), "stack", stack.Trace().TrimRuntime())
					continue
				}
				copiedCfg := cpy.(Config)
				sharedCfg.Lock()
				sharedCfg.cfg = &copiedCfg
				sharedCfg.Unlock()

				logger.Debug("cycleIndexes cfg updated", "endpoint", sharedCfg.endpoint)
			case <-ctx.Done():
				return
			}
		}
	}(ctx, sharedCfg)

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
			// This is how long we wait between successful attempts to scan the indexes
		}

		// On any successful attempt to scan indexes the scanEndpoint will return and
		// we use that to reset the backoff timer for retries
		retries.Reset()
		if err := scanEndpoint(ctx, sharedCfg, retries); err != nil {
			logger.Warn(err.Error())
		}
	}
}

func scanEndpoint(ctx context.Context, sharedCfg *safeConfig, retries *backoff.ExponentialBackOff) (err kv.Error) {

	_, span := otel.Tracer(tracerName).Start(ctx, "endpoint-select")
	defer span.End()

	ticker := backoff.NewTickerWithTimer(retries, nil)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sharedCfg.Lock()
			endpoint := sharedCfg.cfg.endpoint
			sharedCfg.Unlock()

			if len(endpoint) == 0 {
				return kv.NewError("configuration not ready").With("endpoint", endpoint).With("stack", stack.Trace().TrimRuntime())
			}

			if err = doScan(ctx, sharedCfg, retries); err != nil {
				logger.Warn(err.Error())
				continue
			}
			return nil
		case <-ctx.Done():
			return nil
		}
	}
}

func doScan(ctx context.Context, sharedCfg *safeConfig, retries *backoff.ExponentialBackOff) (err kv.Error) {

	// Use 2 channels to denote the start and completion of this function.  The channels being closed will
	// cause any and all listeners to receive a nil and reads to fail.  Listeners should listen to the start
	// channel close and then the end channels closing in order to be sure that the entire cycle of refreshing
	// the state of the server has been completed.
	//
	func() {
		defer func() {
			recover()
			indexStartSync = make(chan struct{})
		}()
		close(indexStartSync)
	}()

	defer func() {
		defer func() {
			recover()
			indexEndSync = make(chan struct{})
		}()
		close(indexEndSync)
	}()

	_, span := otel.Tracer(tracerName).Start(ctx, "scan")
	defer func() {
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	sharedCfg.Lock()
	cfg := sharedCfg.cfg
	sharedCfg.Unlock()

	span.SetAttributes(bucketKey.String(cfg.bucket))

	client, errGo := minio.New(cfg.endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.accessKey, cfg.secretKey, ""),
		Secure: false,
	})
	if errGo != nil {
		return kv.Wrap(errGo).With("endpoint", cfg.endpoint).With("stack", stack.Trace().TrimRuntime())
	}

	if logger.IsTrace() {
		client.TraceOn(nil)
	}

	// Iterate the top level items in the bucket loading index csv file contents and
	// send them to a listener.

	infoC := client.ListObjects(ctx, cfg.bucket, minio.ListObjectsOptions{
		UseV1:        true,
		WithMetadata: true,
		Prefix:       indexPrefix,
		Recursive:    true,
	})

	entries, err := bucketInfoFetch(ctx, client, infoC, cfg, retries)
	if err != nil {
		return err
	}

	// Remove any entries that had disappeared from the bucket
	return GetModelIndex().Groom(cfg.endpoint, entries)
}

func bucketInfoFetch(ctx context.Context, client *minio.Client, infoC <-chan minio.ObjectInfo,
	cfg *Config, retries *backoff.ExponentialBackOff) (entries map[string]minio.ObjectInfo, err kv.Error) {

	_, span := otel.Tracer(tracerName).Start(ctx, "bucket-info-fetch")
	defer span.End()

	entries = map[string]minio.ObjectInfo{}
	for object := range infoC {
		if object.Err != nil {
			err = kv.Wrap(object.Err).With("bucket", cfg.bucket, "indexPrefix", indexPrefix).With("stack", stack.Trace().TrimRuntime())
			if minio.ToErrorResponse(object.Err).Code == "AccessDenied" {
				span.SetStatus(codes.Error, err.Error())
				return nil, err
			}
			// Not having a bucket is the same as deleting all of the model entries
			// so we continue with processing rather than stopping at this point
			if minio.ToErrorResponse(object.Err).Code == "NoSuchBucket" {
				logger.Debug("NoSuchBucket", "endpoint", cfg.endpoint, "bucket", cfg.bucket, "key", object.Key, "stack", stack.Trace().TrimRuntime())
				return nil, nil
			}
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		if !strings.HasSuffix(object.Key, indexSuffix) {
			continue
		}

		// Read the contents
		if err := getIndex(ctx, client, cfg.bucket, object, retries); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		entries[object.Key] = object
	}
	return entries, nil
}

// addModel can be used to inject a new object info structure into our collection
// model
//
func addModel(endpoint string, obj minio.ObjectInfo) (mdl *model, err kv.Error) {
	// Deep copy the original minio object information and place it into the model collection
	cpy, errGo := copystructure.Copy(obj)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	newObj, ok := cpy.(minio.ObjectInfo)
	if !ok {
		return nil, kv.NewError("unable to copy object info").With("stack", stack.Trace().TrimRuntime())
	}

	// To trigger a load clear out the ETag to invalidate the blobs etc
	newObj.ETag = ""

	mdl = &model{
		obj:   &newObj,
		blobs: map[string]*minio.ObjectInfo{},
	}

	GetModelIndex().Add(endpoint, newObj.Key, mdl)

	return mdl, nil
}

// getIndexes will load a single index file
func getIndex(ctx context.Context, client *minio.Client, bucket string, obj minio.ObjectInfo, retries *backoff.ExponentialBackOff) (err kv.Error) {
	if client == nil {
		return kv.NewError("S3 client not available").With("stack", stack.Trace().TrimRuntime())
	}

	if len(bucket) == 0 {
		return kv.NewError("Bucket name missing").With("stack", stack.Trace().TrimRuntime())
	}

	// Prevent excessive indexes that cannot possibly be valid from flooding the server
	if obj.Size > largestIndexSize {
		return kv.NewError("index too large").With("size", humanize.Bytes(uint64(obj.Size)), "limit", humanize.Bytes(largestIndexSize)).With("stack", stack.Trace().TrimRuntime())
	}

	// The endpoint format within this sever for minio does not include the
	// schema portion of the URL
	endpoint := client.EndpointURL().Hostname() + ":" + client.EndpointURL().Port()

	// Reset the exponential backoff
	retries.Reset()

	// After validating parameters see if we have an entry for this index already
	mdl := GetModelIndex().Get(endpoint, obj.Key)

	if mdl != nil {
		// If the model is in memory check its ETag to see if the payload has changed
		if mdl.obj.ETag == obj.ETag {
			return nil
		}
	} else {
		// If there is no existing index being tracked add one
		if mdl, err = addModel(endpoint, obj); err != nil {
			return err
		}
	}

	// Now reload the index file from S3 storage
	if err = mdl.Load(ctx, client, bucket, mdl.obj, largestIndexSize); err != nil {
		return err
	}

	defer func() {
		if err == nil {
			GetModelIndex().Set(endpoint, obj.Key, obj.ETag)
		}
	}()

	return nil
}
