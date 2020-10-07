package main

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/leaf-ai/studio-go-runner/pkg/log"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// This file contains the implementations of various model index parsing and loading features

// TestEmptyModelLoad will populate an S3 bucket with an empty index file and check that it loads
//
func TestEmptyModelLoad(t *testing.T) {
	logger := log.NewLogger("TestEmptyModelLoad")
	defer logger.Warn("TestEmptyModelLoad")

	// Check that the S3 test server has been started locally
	WaitForMinioTest(context.Background())

	// Create the test bucket and then place an empty index into it
	s3Client, errGo := minio.New(*endpointOpt, &minio.Options{
		Creds:  credentials.NewStaticV4(*accessKeyOpt, *secretKeyOpt, ""),
		Secure: false,
	})
	if errGo != nil {
		err := kv.Wrap(errGo).With("endpoint", *endpointOpt).With("stack", stack.Trace().TrimRuntime())
		t.Fatal(err)
	}

	spew.Dump(s3Client)

	// Wait for the server to complete an update pass
	WaitForScan(context.Background())
}
