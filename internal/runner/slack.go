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

	slackOff = time.Now()
)

func init() {
	footer = GetHostName()
}

// msgToSlack accepts a color code and a test message for sending to slack
//
// Color codes are from https://github.com/golang/image/blob/master/colornames/table.go
//
// Should slack return an unexpected error messages will be dropped for one minute
//
func msgToSlack(channel string, color color.RGBA, msg string, detail []string) (err error) {

	if slackOff.After(time.Now()) {
		return errors.New("slack backed off due to earlier error").With("stack", stack.Trace().TrimRuntime())
	}

	if 0 == len(*slackRoom) && 0 == len(*slackHook) {
		return errors.New("no slack available for msgs").With("stack", stack.Trace().TrimRuntime())
	}

	webColor := fmt.Sprintf("#%02X%02X%02X", color.R, color.G, color.B)
	now := time.Now().Unix()

	attachment := slack.Attachment{
		Timestamp:  &now,
		Color:      &webColor,
		Fallback:   &msg,
		Text:       &msg,
		Footer:     &footer,
		FooterIcon: &footerIcon,
	}
	payload := slack.Payload{
		Channel:     channel,
		Attachments: []slack.Attachment{attachment},
	}

	for i, line := range detail {
		// Never ever send more than 20 attachments
		if i >= 20 {
			break
		}
		payload.Attachments = append(payload.Attachments,
			slack.Attachment{
				Text: &line,
			})
	}

	content, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", *slackHook, bytes.NewBuffer(content))
	if err != nil {
		slackOff = time.Now().Add(time.Minute)
		return errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		slackOff = time.Now().Add(time.Minute)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		slackOff = time.Now().Add(time.Minute)
	}
	resp.Body.Close()

	return nil
}

func WarningSlack(room string, msg string, detail []string) (err error) {
	rm := room
	if 0 == len(rm) {
		rm = *slackRoom
	}
	return msgToSlack(rm, colornames.Goldenrod, msg, detail)
}

func ErrorSlack(room string, msg string, detail []string) (err error) {
	rm := room
	if 0 == len(rm) {
		rm = *slackRoom
	}
	return msgToSlack(rm, colornames.Red, msg, detail)
}

func InfoSlack(room string, msg string, detail []string) (err error) {
	rm := room
	if 0 == len(rm) {
		rm = *slackRoom
	}
	return msgToSlack(rm, colornames.Forestgreen, msg, detail)
}
