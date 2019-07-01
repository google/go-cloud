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

// TODO(rvangent): This file is user-visible, add many comments explaining
// how it works.

// How long to wait for a message before giving up.
const waitForMessage = 500 * time.Millisecond

func init() {
	http.HandleFunc("/demo/pubsub/", pubsubSendHandler)
	http.HandleFunc("/demo/pubsub/send", pubsubSendHandler)
	http.HandleFunc("/demo/pubsub/receive", pubsubReceiveHandler)
}

var topicURL string
var topic *pubsub.Topic
var topicErr error

var subscriptionURL string
var subscription *pubsub.Subscription
var subscriptionErr error

func init() {
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
    This page demonstrates the use of Go CDK's <a href="https://godoc.org/gocloud.dev/pubsub">pubsub</a> package.
  </p>
  <p>
    It is currently using a pubsub.Topic based on the URL "{{ .TopicURL }}",
    which can be configured via the environment variable "PUBSUB_TOPIC_URL",
    and a pubsub.Subscription based on the URL "{{ .SubscriptionURL }}", which
    can be configured via the environment variable "PUBSUB_SUBSCRIPTION_URL".
  </p>
  <p>
    See <a href="https://gocloud.dev/concepts/urls/">here</a> for more
    information about URLs in Go CDK APIs.
  </p>
  <ul>
    <li><a href="./send">Send</a> a message to the Topic</li>
    <li><a href="./receive">Receive</a> messages from the Subscription</li>
  </ul>
  {{if .Err }}
    <p><strong>{{ .Err }}</strong></p>
  {{end}}`

	pubsubTemplateSuffix = `
</body>
</html>`

	pubsubSendTemplate = pubsubTemplatePrefix + `
  {{if .Success }}
    <p>Message sent!</p>
  {{end}}
  <form>
      <p><label>
        Enter a message to send to the topic:
        <br/>
        <textarea rows="4" cols="50" name="msg">{{ .Msg }}</textarea>
    </label></p>
    <br/>
    <input type="submit" value="Send Message">
  </form>` + pubsubTemplateSuffix

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

func pubsubSendHandler(w http.ResponseWriter, req *http.Request) {
	input := &pubsubData{
		TopicURL:        topicURL,
		SubscriptionURL: subscriptionURL,
		Msg:             req.FormValue("msg"),
	}
	defer func() {
		if err := pubsubSendTmpl.Execute(w, input); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	if topicErr != nil {
		input.Err = topicErr
		return
	}
	if input.Msg != "" {
		input.Err = topic.Send(req.Context(), &pubsub.Message{Body: []byte(input.Msg)})
		if input.Err == nil {
			input.Success = true
		}
	}
}

func pubsubReceiveHandler(w http.ResponseWriter, req *http.Request) {
	input := &pubsubData{
		TopicURL:        topicURL,
		SubscriptionURL: subscriptionURL,
	}
	defer func() {
		if err := pubsubReceiveTmpl.Execute(w, input); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	if subscriptionErr != nil {
		input.Err = subscriptionErr
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), waitForMessage)
	defer cancel()
	msg, err := subscription.Receive(ctx)
	if err == nil {
		input.Msg = string(msg.Body)
		input.Success = true
		msg.Ack()
	} else if err != context.DeadlineExceeded {
		input.Err = err
	}
}
