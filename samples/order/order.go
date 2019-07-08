// Copyright 2019 The Go Cloud Development Kit Authors
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
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"gocloud.dev/blob"
	"gocloud.dev/docstore"
	"gocloud.dev/pubsub"
)

var (
	requestTopicURL  = flag.String("request-topic", "mem://requests", "gocloud.dev/pubsub URL for request topic")
	requestSubURL    = flag.String("request-sub", "mem://requests", "gocloud.dev/pubsub URL for request subscription")
	responseTopicURL = flag.String("response-topic", "mem://responses", "gocloud.dev/pubsub URL for response topic")
	responseSubURL   = flag.String("response-sub", "mem://responses", "gocloud.dev/pubsub URL for response subscription")
	bucketURL        = flag.String("bucket", "", "gocloud.dev/blob URL for image bucket")
	collectionURL    = flag.String("collection", "mem://orders/ID", "gocloud.dev/docstore URL for order collection")

	port         = flag.Int("port", 10538, "port for frontend")
	runFrontend  = flag.Bool("frontend", true, "run the frontend")
	runProcessor = flag.Bool("processor", true, "run the image processor")
)

func main() {
	flag.Parse()
	conf := config{
		requestTopicURL:  *requestTopicURL,
		requestSubURL:    *requestSubURL,
		responseTopicURL: *responseTopicURL,
		responseSubURL:   *responseSubURL,
		bucketURL:        *bucketURL,
		collectionURL:    *collectionURL,
	}
	frontend, processor, cleanup, err := setup(conf)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	errc := make(chan error, 2)
	if *runFrontend {
		go func() { errc <- frontend.run(context.Background(), *port) }()
		fmt.Printf("listening on port %d\n", *port)
	} else {
		errc <- nil
	}
	if *runProcessor {
		go func() { errc <- processor.run(context.Background()) }()
	} else {
		errc <- nil
	}
	for i := 0; i < 2; i++ {
		if err := <-errc; err != nil {
			log.Fatal(err)
		}
	}
}

// config describes the URLs for the resources used by the order application.
type config struct {
	requestTopicURL  string
	requestSubURL    string
	responseTopicURL string
	responseSubURL   string
	bucketURL        string
	collectionURL    string
}

// setup opens all the necessary resources for the application.
func setup(conf config) (_ *frontend, _ *processor, cleanup func(), err error) {
	// TODO(jba): simplify cleanup logic
	var cleanups []func()
	defer func() {
		// Clean up on error; return cleanup func on success.
		f := func() {
			for _, c := range cleanups {
				c()
			}
		}
		if err != nil {
			f()
			cleanup = nil
		} else {
			cleanup = f
		}
	}()

	ctx := context.Background()
	// TODO(jba): This application assumes at-least-once processing. Enforce that here if possible.
	reqTopic, err := pubsub.OpenTopic(ctx, conf.requestTopicURL)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, func() { reqTopic.Shutdown(ctx) })

	reqSub, err := pubsub.OpenSubscription(ctx, conf.requestSubURL)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, func() { reqSub.Shutdown(ctx) })

	resTopic, err := pubsub.OpenTopic(ctx, conf.responseTopicURL)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, func() { resTopic.Shutdown(ctx) })

	resSub, err := pubsub.OpenSubscription(ctx, conf.responseSubURL)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, func() { resSub.Shutdown(ctx) })

	burl := conf.bucketURL
	if burl == "" {
		dir, err := ioutil.TempDir("", "gocdk-order")
		if err != nil {
			return nil, nil, nil, err
		}
		burl = "file://" + filepath.ToSlash(dir)
		cleanups = append(cleanups, func() { os.Remove(dir) })
	}
	bucket, err := blob.OpenBucket(ctx, burl)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, func() { bucket.Close() })

	coll, err := docstore.OpenCollection(ctx, conf.collectionURL)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanups = append(cleanups, func() { coll.Close() })

	f := &frontend{
		requestTopic: reqTopic,
		bucket:       bucket,
		coll:         coll,
	}
	p := &processor{
		requestSub: reqSub,
		bucket:     bucket,
		coll:       coll,
	}
	return f, p, nil, nil
}
