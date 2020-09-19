// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"flag"
	"time"

	"github.com/leaf-ai/studio-go-runner/pkg/log"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/davecgh/go-spew/spew"
)

const (
	minimumScanRate = time.Duration(3 * time.Second)
)

var (
	endpoint  = flag.String("AWS_ENDPOINT", "", "In the case of minio this should be a hostname, for aws please use \"s3.amazonaws.co\"")
	accessKey = flag.String("AWS_ACCESS_KEY_ID", "", "mandatory credentials for accessing S3 storage")
	secretKey = flag.String("AWS_SECRET_ACCESS_KEY", "", "mandatory credentials for accessing S3 storage")
)

// serviceIndexes will on a regular interval check for new index-* files at a well known location
// and if are new, modified or deleted based on the state inside a tensorflow model serving configuration
// will dispatch a function to apply them to the configuration file
//
func serviceIndexes(ctx context.Context, interval time.Duration, logger *log.Logger) {
	if interval < minimumScanRate {
		interval = minimumScanRate
		logger.Warn("specified scan interval too small, set to minimum", "interval", interval)
	}

	s3Client, err := minio.New(*endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(*accessKey, *secretKey, ""),
		Secure: false,
	})
	if err != nil {
		logger.Warn(err.Error())
	}
	logger.Info(spew.Sdump(s3Client))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}
