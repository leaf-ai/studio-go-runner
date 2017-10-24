package runner

// This file contains a slack webhook messenger for gofer to be able to
// transmit messages to the DarkCycle devops channel
//
import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"net/http"
	"time"

	slack "github.com/karlmutch/slack-go-webhook"

	"golang.org/x/image/colornames"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

var (
	slackHook = flag.String("slack-hook", "", "The URL used by Slack for StudioML announcements")
	slackRoom = flag.String("slack-room", "", "The # or @ based slack channel to which messages will be sent")

	footer     = "studioml go runner"
	footerIcon = "https://38.media.tumblr.com/avatar_e7193ec7df1a_128.png"
)

func init() {
	footer = GetHostName()
}

// msgToSlack accepts a color code and a test message for sending to slack
//
// Color codes are from https://github.com/golang/image/blob/master/colornames/table.go
//
func msgToSlack(channel string, color color.RGBA, msg string, detail string) (err error) {

	if 0 == len(*slackRoom) && 0 == len(*slackHook) {
		return errors.Wrap(errors.New("no slack available for msgs")).With("stack", stack.Trace().TrimRuntime())
	}

	webColor := fmt.Sprintf("#%02X%02X%02X", color.R, color.G, color.B)
	now := time.Now().Unix()

	attachment := slack.Attachment{
		Color:      &webColor,
		Fallback:   &msg,
		Text:       &msg,
		Timestamp:  &now,
		Footer:     &footer,
		FooterIcon: &footerIcon,
	}
	payload := slack.Message{
		Channel:     channel,
		Attachments: []slack.Attachment{attachment},
	}

	if 0 != len(detail) {

		detailAttach := slack.Attachment{
			Text: &detail,
		}
		payload.Attachments = append(payload.Attachments, detailAttach)
	}

	content, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", *slackHook, bytes.NewBuffer(content))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func WarningSlack(msg string, detail string) (err error) {
	return msgToSlack(*slackRoom, colornames.Goldenrod, msg, detail)
}

func ErrorSlack(msg string, detail string) (err error) {
	return msgToSlack(*slackRoom, colornames.Red, msg, detail)
}

func InfoSlack(msg string, detail string) (err error) {
	return msgToSlack(*slackRoom, colornames.Forestgreen, msg, detail)
}
