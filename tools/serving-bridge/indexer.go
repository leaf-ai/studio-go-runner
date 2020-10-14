// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the implementation of a long lived component that scans for changes to the
// index files placed into S3 folder(s) and loads these into an in memory collection of known indexes

import (
	"context"
	"flag"
	"net"
	"strings"
	"time"

	"github.com/leaf-ai/studio-go-runner/pkg/log"
	"github.com/mitchellh/copystructure"

	"github.com/cenkalti/backoff/v4"

	"go.opentelemetry.io/otel/api/global"
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
	endpointOpt  = flag.String("AWS_ENDPOINT", "", "In the case of minio this should be a hostname, for aws please use \"s3.amazonaws.com\"")
	accessKeyOpt = flag.String("AWS_ACCESS_KEY_ID", "", "mandatory credentials for accessing S3 storage")
	secretKeyOpt = flag.String("AWS_SECRET_ACCESS_KEY", "", "mandatory credentials for accessing S3 storage")
	bucketOpt    = flag.String("AWS_BUCKET", "model-serving", "The name of the bucket which will be scanned for CSV index files")

	bucketKey = label.Key("studio.ml/bucket")

	updateStartSync = make(chan struct{})
	updateEndSync   = make(chan struct{})
)

// WaitForScan will block the caller until at least one complete update cycle
// is done
func WaitForScan(ctx context.Context) {

	select {
	case <-ctx.Done():
	case <-updateStartSync:
	}

	select {
	case <-ctx.Done():
	case <-updateEndSync:
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

	logger.Warn("Debug", "stack", stack.Trace().TrimRuntime())

	updatedCfgC := make(chan Config, 1)
	defer close(updatedCfgC)

	if cfgUpdater != nil {
		updaterHndl, err := cfgUpdater.Add(updatedCfgC)
		if err != nil {
			logger.Warn("dynamic configuration changes not supported in the current deployment")
		} else {
			defer cfgUpdater.Delete(updaterHndl)
		}
	}

	logger.Warn("Debug", "stack", stack.Trace().TrimRuntime())
	// Before starting make sure we get at least the starting configuration
	cfg := Config{}
	func() {
		for {
			select {
			case cfg = <-updatedCfgC:
				if _, _, err := net.SplitHostPort(cfg.endpoint); err == nil {
					return
				}
			case <-ctx.Done():
				logger.Warn("indexer could not be started using an initial configuration before the server was terminated")
				return
			}
		}
	}()
	cycleIndexes(ctx, cfg, updatedCfgC, retries, logger)
}

func cycleIndexes(ctx context.Context, cfg Config, updatedCfgC chan Config, retries *backoff.ExponentialBackOff, logger *log.Logger) {

	logger.Debug("indexer initialized")

	ticker := backoff.NewTickerWithTimer(retries, nil)
	for {
		select {
		case newCfg := <-updatedCfgC:
			cpy, errGo := copystructure.Copy(newCfg)
			if errGo != nil {
				logger.Warn("updated configuration could not be used", "error", errGo.Error(), "stack", stack.Trace().TrimRuntime())
				continue
			}
			cfg = cpy.(Config)
		case <-ticker.C:

			if err := scanEndpoint(ctx, cfg, retries); err != nil {
				logger.Warn(err.Error())
				continue
			}
			ticker.Stop()
			return
		case <-ctx.Done():
			return
		}
	}
}

func scanEndpoint(ctx context.Context, cfg Config, retries *backoff.ExponentialBackOff) (err kv.Error) {

	_, span := global.Tracer(tracerName).Start(ctx, "endpoint-select")
	defer span.End()

	// Server connectivity has been successful so use the same retries strategies
	// when using queries against the working working service
	retries.Reset()

	ticker := backoff.NewTickerWithTimer(retries, nil)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err = doScan(ctx, cfg, retries); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func doScan(ctx context.Context, cfg Config, retries *backoff.ExponentialBackOff) (err kv.Error) {

	// Use 2 channels to denote the start and completion of this function.  The channels being closed will
	// cause any and all listeners to receive a nil and reads to fail.  Listeners should listen to the start
	// channel close and then the end channels closing in order to be sure that the entire cycle of refreshing
	// the state of the server has been completed.
	//
	func() {
		defer func() {
			recover()
			updateStartSync = make(chan struct{})
		}()
		close(updateStartSync)
	}()

	defer func() {
		defer func() {
			recover()
			updateEndSync = make(chan struct{})
		}()
		close(updateEndSync)
	}()

	_, span := global.Tracer(tracerName).Start(ctx, "scan")
	defer span.End()

	span.SetAttributes(bucketKey.String(cfg.bucket))

	client, errGo := minio.New(cfg.endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.accessKey, cfg.secretKey, ""),
		Secure: false,
	})
	if errGo != nil {
		err = kv.Wrap(errGo).With("endpoint", cfg.endpoint).With("stack", stack.Trace().TrimRuntime())
		span.SetStatus(codes.Unavailable, err.Error())
		return err
	}

	if logger.IsTrace() {
		client.TraceOn(nil)
	}

	// Iterate the top level items in the bucket loading index csv file contents and
	// send them to a listener.

	logger.Trace("", "endpoint", cfg.endpoint, "bucket", cfg.bucket, "stack", stack.Trace().TrimRuntime())

	infoC := client.ListObjects(context.Background(), cfg.bucket, minio.ListObjectsOptions{
		UseV1:        true,
		WithMetadata: true,
		Prefix:       indexPrefix,
		Recursive:    true,
	})

	entries := map[string]minio.ObjectInfo{}

	for object := range infoC {
		logger.Trace("", "endpoint", cfg.endpoint, "bucket", cfg.bucket, "key", object.Key, "stack", stack.Trace().TrimRuntime())
		if object.Err != nil {
			if minio.ToErrorResponse(object.Err).Code == "AccessDenied" {
				continue
			}
			err = kv.Wrap(object.Err).With("bucket", cfg.bucket, "indexPrefix", indexPrefix).With("stack", stack.Trace().TrimRuntime())
			span.SetStatus(codes.Unavailable, err.Error())
			return err
		}
		if !strings.HasSuffix(object.Key, indexSuffix) {
			continue
		}

		// Read the contents
		if err := getIndex(ctx, client, cfg.bucket, object, retries); err != nil {
			span.SetStatus(codes.Unavailable, err.Error())
			return err
		}
		entries[object.Key] = object
	}

	// Remove any entries that had disappeared from the bucket
	if _, err = GetModelIndex().Groom(cfg.endpoint, entries); err != nil {
		return err
	}

	return nil
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

	endpoint := client.EndpointURL().String()

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
