package runner

// This module contains the code that interfaces with the google pubsub system and
// authentication used by google

import (
	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/storage"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	OAuthConfig *oauth2.Config

	StorageBucket     *storage.BucketHandle
	StorageBucketName string

	PubsubClient *pubsub.Client
)

func init() {
}
