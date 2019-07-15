// This file demonstrates basic usage of the pubsub.Topic and
// pubsub.Subscription portable types.
//
// It initializes pubsub.Topic and pubsub.Subscription URLs based on the
// environment variables PUBSUB_TOPIC_URL and PUBSUB_SUBSCRIPTION_URL, and then
// registers handlers for "/demo/pubsub" on http.DefaultServeMux.

package main

import (
	"context"
	"html/template"
	"net/http"
	"os"
	"time"

	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/awssnssqs"
	_ "gocloud.dev/pubsub/azuresb"
	_ "gocloud.dev/pubsub/gcppubsub"
	_ "gocloud.dev/pubsub/mempubsub"
)

// Package variables for the pubsub.Topic and pubsub.Subscription URLs, and the
// initialized pubsub.Topic and pubsub.Subscription.
var (
	topicURL        string
	topic           *pubsub.Topic
	topicErr        error
	subscriptionURL string
	subscription    *pubsub.Subscription
	subscriptionErr error
)

func init() {
	// Register handlers. See https://golang.org/pkg/net/http/.
	http.HandleFunc("/demo/pubsub/", pubsubSendHandler)
	http.HandleFunc("/demo/pubsub/send", pubsubSendHandler)
	http.HandleFunc("/demo/pubsub/receive", pubsubReceiveHandler)

	// Initialize the pubsub.Topic and pubsub.Subscription using URLs from the
	// environment, defaulting to an in-memory driver.
	ctx := context.Background()
	topicURL = os.Getenv("PUBSUB_TOPIC_URL")
	if topicURL == "" {
		topicURL = "mem://mytopic"
	}
	topic, topicErr = pubsub.OpenTopic(ctx, topicURL)
	subscriptionURL = os.Getenv("PUBSUB_SUBSCRIPTION_URL")
	if subscriptionURL == "" {
		subscriptionURL = "mem://mytopic"
	}
	subscription, subscriptionErr = pubsub.OpenSubscription(ctx, subscriptionURL)
}

// pubsubData holds the input for each of the pages in the demo. Each page
// handler will initialize the struct and pass it to one of the templates.
type pubsubData struct {
	TopicURL        string
	SubscriptionURL string
	Err             error

	Success bool
	Msg     string
}

const (
	pubsubTemplatePrefix = `
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>gocloud.dev/pubsub demo</title>
</head>
<body>
  <p>
    This page demonstrates the use of Go CDK's <a href="https://gocloud.dev/howto/pubsub">pubsub</a> package.
  </p>
  <p>
    It is currently using a pubsub.Topic based on the URL "{{ .TopicURL }}",
    which can be configured via the environment variable "PUBSUB_TOPIC_URL",
    and a pubsub.Subscription based on the URL "{{ .SubscriptionURL }}", which
    can be configured via the environment variable "PUBSUB_SUBSCRIPTION_URL".
  </p>
  <ul>
    <li><a href="./send">Send</a> a message to the Subscription</li>
    <li><a href="./receive">Receive</a> a message from the Subscription</li>
  </ul>
  {{if .Err }}
    <p><strong>{{ .Err }}</strong></p>
  {{end}}`

	pubsubTemplateSuffix = `
</body>
</html>`

	// pubsubSendTemplate is the template for /demo/pubsub and /demo/pubsub/send. See pubsubSendHandler.
	// Input: *pubsubData.
	pubsubSendTemplate = pubsubTemplatePrefix + `
  {{if .Success }}
    <p>Message sent!</p>
  {{end}}
  <form method="POST">
    <p><label>
      Enter a message to send to the topic:
      <br/>
      <textarea rows="4" cols="50" name="msg"></textarea>
    </label></p>
    <br/>
    <input type="submit" value="Send Message">
  </form>` + pubsubTemplateSuffix

	// pubsubReceiveTemplate is the template for /demo/pubsub/receive. See pubsubReceiveHandler.
	// Input: *pubsubData.
	pubsubReceiveTemplate = pubsubTemplatePrefix + `
  {{if .Success }}
    <p><label>
      Received message:
      <br/>
      <textarea rows="4" cols="50" name="msg">{{ .Msg }}</textarea>
    </label></p>
  {{else}}
    <p>No message available.</p>
  {{end}}` + pubsubTemplateSuffix
)

var (
	pubsubSendTmpl    = template.Must(template.New("pubsub send").Parse(pubsubSendTemplate))
	pubsubReceiveTmpl = template.Must(template.New("pubsub receive").Parse(pubsubReceiveTemplate))
)

// pubsubSendHandler is the handler for /demo/pubsub.
//
// It shows a form that allows the user to enter a message to be sent to the
// topic.
func pubsubSendHandler(w http.ResponseWriter, req *http.Request) {
	data := &pubsubData{
		TopicURL:        topicURL,
		SubscriptionURL: subscriptionURL,
	}
	defer func() {
		if err := pubsubSendTmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	// Verify that topic initialization succeeded.
	if topicErr != nil {
		data.Err = topicErr
		return
	}

	// For GET, just render the form.
	if req.Method == http.MethodGet {
		return
	}

	// POST. Send the message.
	data.Err = topic.Send(req.Context(), &pubsub.Message{Body: []byte(req.FormValue("msg"))})
	if data.Err == nil {
		data.Success = true
	}
}

// pubsubReceiveHandler is the handler for /demo/pubsub/receive.
//
// It tries to read a message from the subscription, with a short timeout.
func pubsubReceiveHandler(w http.ResponseWriter, req *http.Request) {
	data := &pubsubData{
		TopicURL:        topicURL,
		SubscriptionURL: subscriptionURL,
	}
	defer func() {
		if err := pubsubReceiveTmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	// Verify that subscription initialization succeeded.
	if subscriptionErr != nil {
		data.Err = subscriptionErr
		return
	}

	// Try to read a message from the subscription, with a short timeout.
	// Without the timeout, Receive might block indefinitely if no messages
	// are available.
	ctx, cancel := context.WithTimeout(req.Context(), 250*time.Millisecond)
	defer cancel()
	msg, err := subscription.Receive(ctx)
	if err == nil {
		// We received a message. Add it to data to be shown on the page.
		data.Msg = string(msg.Body)
		data.Success = true
		msg.Ack()
	} else if err != context.DeadlineExceeded {
		// context.DeadlineExceeded means no messages were available; the template
		// shows "No messages available" text for that if Success = false.
		// For other errors, shows the error.
		data.Err = err
	}
}
