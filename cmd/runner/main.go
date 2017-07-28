package main

import (
	"fmt"

	"github.com/karlmutch/studio-go-runner"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/mgutz/logxi/v1"
)

var (
	logger log.Logger
)

func init() {
	logger = log.New("runner")
}

func main() {

	logger.SetLevel(log.LevelInfo)

	projectID := "tfstudio-a8367"
	qName := "TFStudioTest"

	ps, err := runner.NewPubSub(projectID)
	if err != nil {
		logger.Fatal(fmt.Sprintf("could not start the pubsub listener due to %v", err))
	}
	defer ps.Client.Close()

	ctx := context.Background()
	topic, err := ps.Client.CreateTopic(ctx, projectID)
	if err != nil {
		if grpc.Code(err) != codes.AlreadyExists {
			logger.Fatal(fmt.Sprintf("could not create the topic for pubsub listener due to %v", err))
		}
	}
	defer topic.Stop()

	sub, err := ps.Client.CreateSubscription(ctx, qName+"_sub", pubsub.SubscriptionConfig{Topic: topic})
	if err != nil {
		if grpc.Code(err) != codes.AlreadyExists {
			logger.Fatal(fmt.Sprintf("could not create the subscription for our listener due to %#v", err))
		}
	}

	for {
		err := sub.Receive(ctx,
			func(ctx context.Context, msg *pubsub.Message) {
				logger.Debug(fmt.Sprintf("%#v", *msg))
				logger.Info(fmt.Sprintf("%s", string(msg.Data)))

				msg.Ack()
			})
		if err != nil {
			logger.Fatal(fmt.Sprintf("subscription receiver failed due to %v", err))
		}
	}

}
