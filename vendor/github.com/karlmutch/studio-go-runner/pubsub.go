package runner

// This module contains the code that interfaces with the google pubsub system and
// authentication used by google

import (
	"context"
	"fmt"
	"os"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

var (
	StorageBucket     *storage.BucketHandle
	StorageBucketName string

	PubsubClient *pubsub.Client
)

func init() {
}

func getCred() (opts option.ClientOption, err error) {
	val, isPresent := os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
	if !isPresent {
		return nil, fmt.Errorf(`the environment variable GOOGLE_APPLICATION_CREDENTIALS was not set,
		fix this by creating a service account key using your Web based GCP console and then save the 
		resulting file into a safe location and define an environment variable 
		GOOGLE_APPLICATION_CREDENTIALS to point at this file`)
	}

	return option.WithServiceAccountFile(val), nil
}

type PubSub struct {
	Client *pubsub.Client
}

func NewPubSub(qName string) (ps *PubSub, err error) {
	cred, err := getCred()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	if ps.Client, err = pubsub.NewClient(ctx, qName, cred); err != nil {
		return nil, err
	}
	return ps, nil
}
