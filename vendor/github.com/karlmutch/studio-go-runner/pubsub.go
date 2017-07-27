package runner

// This module contains the code that interfaces with the google pubsub system and
// authentication used by google

import (
	"fmt"
	"os"

	"cloud.google.com/go"
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
		return nil, fmt.Errorf("the environment variable GOOGLE_APPLICATION_CREDENTIALS was not set")
	}

	return option.WithServiceAccountFile(val), nil
}

type PubSub struct {
	Client *pubsub.Client
}

func NewPubSub(qName string) (pubsub PubSub, err error) {
	cred, err := getCred()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	if pubsub.Client, err = pubsub.NewClient(ctx, qName, cred); err != nil {
		return nil, err
	}
	return pubsub
}
