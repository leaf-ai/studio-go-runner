// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"flag"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"

	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/leaf-ai/studio-go-runner/pkg/log"

	"github.com/cenkalti/backoff/v4"

	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/label"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	minimumScanRate = time.Duration(3 * time.Second)

	indexPrefix = "index-"

	tracerName = "studio.ml/scanner"
)

var (
	endpointOpt  = flag.String("AWS_ENDPOINT", "", "In the case of minio this should be a hostname, for aws please use \"s3.amazonaws.com\"")
	accessKeyOpt = flag.String("AWS_ACCESS_KEY_ID", "", "mandatory credentials for accessing S3 storage")
	secretKeyOpt = flag.String("AWS_SECRET_ACCESS_KEY", "", "mandatory credentials for accessing S3 storage")
	bucketOpt    = flag.String("AWS_BUCKET", "ModelServing", "The name of the bucket which will be scanned for CSV index files")

	bucketKey = label.Key("studio.ml/bucket")
)

// WaitForMinioTest is intended to block until such time as a testing minio server is
// found
func WaitForMinioTest(ctx context.Context) {
	if alive, err := runner.MinioTest.IsAlive(ctx); !alive || err != nil {
		if err != nil {
			logger.Info(err.Error())
		}
	}
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
		*endpointOpt = runner.MinioTest.Address
		*accessKeyOpt = runner.MinioTest.AccessKeyId
		*secretKeyOpt = runner.MinioTest.SecretAccessKeyId
	}

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
	}
	return nil
}
