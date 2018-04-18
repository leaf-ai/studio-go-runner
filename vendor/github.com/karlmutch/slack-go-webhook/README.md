[![GoDoc](https://godoc.org/github.com/ashwanthkumar/slack-go-webhook?status.svg)](https://godoc.org/github.com/ashwanthkumar/slack-go-webhook)

# slack-go-webhook

Go Lang library to send messages to Slack via Incoming Webhooks. Original author was Ashwanth Kumar 
and was modified to remove dependencies that set unhealthy requirements such as CLI curl.

## Usage
```go
package main

import "github.com/karlmutch/slack-go-webhook"
import "fmt"

func main() {

    attachment1 := slack.Attachment {}
    attachment1.AddField(slack.Field { Title: "Author", Value: "Ashwanth Kumar" }).AddField(slack.Field { Title: "Status", Value: "Completed" })
    payload := slack.Payload {
      Text: "Hello from <https://github.com/ashwanthkumar/slack-go-webhook|slack-go-webhook>, a Go-Lang library to send slack webhook messages.\n<https://golangschool.com/wp-content/uploads/golang-teach.jpg|golang-img>",
      Username: "robot",
      Channel: "#general",
      IconEmoji: ":monkey_face:",
      Attachments: []slack.Attachment{attachment1},
    }
    webhookUrl := "https://hooks.slack.com/services/foo/bar/baz"

    // Use golangs http client libraries to perform the request
}
```

## License
Licensed under the Apache License, Version 2.0: http://www.apache.org/licenses/LICENSE-2.0

