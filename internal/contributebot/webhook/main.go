// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
	"google.golang.org/api/pubsub/v1"
	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/urlfetch"
)

func main() {
	http.HandleFunc("/", handleDefault)
	http.HandleFunc("/webhook", handleWebhook)
	appengine.Main()
}

// handleDefault serves a static landing page for checking whether the
// service is running.
func handleDefault(w http.ResponseWriter, r *http.Request) {
	const responseData = `<!DOCTYPE html>
<title>Go Cloud Contribute Bot</title>
<h1>Go Cloud Contribute Bot</h1>
<p>Hello, you've reached <a href="https://github.com/google/go-cloud">Go Cloud</a>'s contribute bot!</p>`
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Only GET or HEAD allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(responseData)))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == "GET" {
		io.WriteString(w, responseData)
	}
}

// handleWebhook serves the webhook endpoint called by GitHub.
// See https://developer.github.com/webhooks/ for more details.
func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx := appengine.NewContext(r)
	id := r.Header.Get("X-GitHub-Delivery")
	payload, err := github.ValidatePayload(r, []byte(os.Getenv("CONTRIBUTEBOT_WEBHOOK_SECRET")))
	if err != nil {
		log.Errorf(ctx, "Validate %s: %v", id, err)
		http.Error(w, "Payload signature did not match", http.StatusBadRequest)
		return
	}
	if err := publishEvent(ctx, r.Header, payload); err != nil {
		log.Errorf(ctx, "Publish %s: %v", id, err)
		http.Error(w, "PubSub publish failed", http.StatusInternalServerError)
		return
	}
}

// publishEvent sends a payload to the Pub/Sub topic.
func publishEvent(ctx context.Context, h http.Header, payload []byte) error {
	client, err := newPubSubClient(ctx)
	if err != nil {
		return err
	}
	projectID := appengine.AppID(ctx)
	topic := "projects/" + projectID + "/topics/contributebot-github-events"
	call := client.Projects.Topics.Publish(topic, &pubsub.PublishRequest{
		Messages: []*pubsub.PubsubMessage{
			{
				Data: base64.URLEncoding.EncodeToString(payload),
				Attributes: map[string]string{
					"X-GitHub-Event":    h.Get("X-GitHub-Event"),
					"X-GitHub-Delivery": h.Get("X-GitHub-Delivery"),
				},
			},
		},
	})
	if _, err := call.Context(ctx).Do(); err != nil {
		return err
	}
	return nil
}

// newPubSubClient constructs a Pub/Sub REST client authenticated with
// the App Engine service account.
func newPubSubClient(ctx context.Context) (*pubsub.Service, error) {
	tok, expiry, err := appengine.AccessToken(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("create pubsub client: %v")
	}
	hc := &http.Client{
		Transport: &oauth2.Transport{
			Source: oauth2.StaticTokenSource(&oauth2.Token{
				AccessToken: tok,
				TokenType:   "Bearer",
				Expiry:      expiry,
			}),
			Base: &urlfetch.Transport{Context: ctx},
		},
	}
	srv, err := pubsub.New(hc)
	if err != nil {
		return nil, fmt.Errorf("create pubsub client: %v")
	}
	return srv, nil
}
