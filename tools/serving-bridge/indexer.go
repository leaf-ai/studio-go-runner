// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"flag"
	"strings"
	"time"

	"github.com/leaf-ai/studio-go-runner/internal/runner"
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
	bucketOpt    = flag.String("AWS_BUCKET", "ModelServing", "The name of the bucket which will be scanned for CSV index files")

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

// WaitForMinioTest is intended to block until such time as a testing minio server is
// found.  It will also update the server CLI config items to reflect the servers presence.
//
func WaitForMinioTest(ctx context.Context) {
	if alive, err := runner.MinioTest.IsAlive(ctx); !alive || err != nil {
		if err != nil {
			logger.Info(err.Error())
		}
		return
	}
	*endpointOpt = runner.MinioTest.Address
	*accessKeyOpt = runner.MinioTest.AccessKeyId
	*secretKeyOpt = runner.MinioTest.SecretAccessKeyId
}

// serviceIndexes will on a regular interval check for new index-* files at a well known location
// and if are new, modified or deleted based on the state inside a tensorflow model serving configuration
// will dispatch a function to apply them to the configuration file
//
func serviceIndexes(ctx context.Context, retries *backoff.ExponentialBackOff, logger *log.Logger) {
	if retries.InitialInterval < minimumScanRate {
		retries.InitialInterval = minimumScanRate
		logger.Warn("specified scan interval too small, set to minimum", "retries", retries)
	}

	// If we are in test mode we check to see if the CLI overrides are in play and if not we
	// retrieve the credentials from the test framework
	if len(*accessKeyOpt) == 0 && len(*secretKeyOpt) == 0 {
		if TestMode {
			// Wait for the minio test server to be stood up if we discover that
			// the server is terminating return
			WaitForMinioTest(ctx)
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}

	cycleIndexes(ctx, retries, logger)
}

func cycleIndexes(ctx context.Context, retries *backoff.ExponentialBackOff, logger *log.Logger) {
	ticker := backoff.NewTickerWithTimer(retries, nil)
	for {
		select {
		case <-ticker.C:
			if err := scanEndpoint(ctx, *bucketOpt, retries); err != nil {
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

func scanEndpoint(ctx context.Context, bucket string, retries *backoff.ExponentialBackOff) (err kv.Error) {

	// Use 2 channels to denotw the start and completion of this function.  The channels being closed will
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

	_, span := global.Tracer(tracerName).Start(ctx, "endpoint-select")
	defer span.End()

	s3Client, errGo := minio.New(*endpointOpt, &minio.Options{
		Creds:  credentials.NewStaticV4(*accessKeyOpt, *secretKeyOpt, ""),
		Secure: false,
	})
	if errGo != nil {
		err = kv.Wrap(errGo).With("bucket", bucket, "indexPrefix", indexPrefix, "endpoint", *endpointOpt).With("stack", stack.Trace().TrimRuntime())
		span.SetStatus(codes.Unavailable, err.Error())
		return err
	}

	// Server connectivity has been successful so use the same retries strategies
	// when using queries against the working working service
	retries.Reset()

	ticker := backoff.NewTickerWithTimer(retries, nil)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err = doScan(ctx, s3Client, bucket, retries); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func doScan(ctx context.Context, client *minio.Client, bucket string, retries *backoff.ExponentialBackOff) (err kv.Error) {
	_, span := global.Tracer(tracerName).Start(ctx, "scan")
	defer span.End()

	span.SetAttributes(bucketKey.String(bucket))

	// Iterate the top level items in the bucket loading index csv file contents and
	// send them to a listener.
	infoC := client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix: indexPrefix,
	})

	for object := range infoC {
		if object.Err != nil {
			if minio.ToErrorResponse(object.Err).Code == "AccessDenied" {
				continue
			}
			err = kv.Wrap(object.Err).With("bucket", bucket, "indexPrefix", indexPrefix).With("stack", stack.Trace().TrimRuntime())
			span.SetStatus(codes.Unavailable, err.Error())
			return err
		}
		if !strings.HasSuffix(object.Key, indexSuffix) {
			continue
		}

		// Read the contents
		if err := getIndex(ctx, client, bucket, object, retries); err != nil {
			span.SetStatus(codes.Unavailable, err.Error())
			return err
		}
	}
	return nil
}

var (
	knownIndexes = map[string]*model{} // The list of known index files and their etags
)

// addModel can be used to inject a new object info structure into our collection
// model
//
func addModel(obj minio.ObjectInfo) (mdl *model, err kv.Error) {
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
	knownIndexes[newObj.Key] = mdl

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

	// After validating parameters see if we have an entry for this index already
	mdl, isPresent := knownIndexes[obj.Key]

	// If there is no existing index being tracked add one
	if !isPresent {
		if mdl, err = addModel(obj); err != nil {
			return err
		}
	}

	if mdl.obj.ETag == obj.ETag && isPresent {
		return nil
	}
	// Now reload the index file from S3 storage
	if err = mdl.Load(ctx, client, bucket, mdl.obj, largestIndexSize); err != nil {
		return err
	}

	defer func() {
		if err == nil {
			knownIndexes[obj.Key].obj.ETag = obj.ETag
		}
	}()

	return nil
}
