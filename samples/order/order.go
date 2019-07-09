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

// This application processes orders for converting images to PNG format. It
// consists of two components: a frontend, which serves web pages that people
// can use to place and view orders; and a processor, which performs the
// conversions. This binary can run both together in one process (the default),
// or it can run either on its own. Either way, the two components:
//  - communicate over a topic using the gocloud.dev/pubsub API;
//  - write orders to a database using the gocloud.dev/docstore API;
//  - and save image files to cloud storage using the gocloud.dev/blob API.
//
// This application assumes at-least-once processing. Make sure the pubsub
// implementation you provide to it has that behavior.
package main

import (
	"context"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"gocloud.dev/blob"
	"gocloud.dev/docstore"
	"gocloud.dev/pubsub"
)

var (
	requestTopicURL = flag.String("request-topic", "mem://requests", "gocloud.dev/pubsub URL for request topic")
	requestSubURL   = flag.String("request-sub", "mem://requests", "gocloud.dev/pubsub URL for request subscription")
	bucketURL       = flag.String("bucket", "", "gocloud.dev/blob URL for image bucket")
	collectionURL   = flag.String("collection", "mem://orders/ID", "gocloud.dev/docstore URL for order collection")

	port         = flag.Int("port", 10538, "HTTP port for frontend")
	runFrontend  = flag.Bool("frontend", true, "run the frontend")
	runProcessor = flag.Bool("processor", true, "run the image processor")
)

func main() {
	flag.Parse()
	conf := config{
		requestTopicURL: *requestTopicURL,
		requestSubURL:   *requestSubURL,
		bucketURL:       *bucketURL,
		collectionURL:   *collectionURL,
	}
	frontend, processor, cleanup, err := setup(conf)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// Run the frontend, or the processor, or both.
	// When we want to run both, one of them has to run in a goroutine.
	// So it's simpler to run both in goroutines, even if we only need
	// to run one.
	errc := make(chan error, 2)
	if *runFrontend {
		go func() { errc <- frontend.run(context.Background(), *port) }()
		log.Printf("listening on port %d", *port)
	} else {
		errc <- nil
	}
	if *runProcessor {
		go func() { errc <- processor.run(context.Background()) }()
		log.Println("processing")
	} else {
		errc <- nil
	}
	// Each of the goroutines will send once to errc, so receive two values.
	for i := 0; i < 2; i++ {
		if err := <-errc; err != nil {
			log.Fatal(err)
		}
	}
}

// config describes the URLs for the resources used by the order application.
type config struct {
	requestTopicURL string
	requestSubURL   string
	bucketURL       string
	collectionURL   string
}

// setup opens all the necessary resources for the application.
func setup(conf config) (_ *frontend, _ *processor, cleanup func(), err error) {

	addCleanup := func(f func()) {
		old := cleanup
		cleanup = func() { old(); f() }
	}

	defer func() {
		if err != nil {
			cleanup()
			cleanup = nil
		}
	}()

	ctx := context.Background()
	cleanup = func() {}

	reqTopic, err := pubsub.OpenTopic(ctx, conf.requestTopicURL)
	if err != nil {
		return nil, nil, nil, err
	}
	addCleanup(func() { reqTopic.Shutdown(ctx) })

	reqSub, err := pubsub.OpenSubscription(ctx, conf.requestSubURL)
	if err != nil {
		return nil, nil, nil, err
	}
	addCleanup(func() { reqSub.Shutdown(ctx) })

	burl := conf.bucketURL
	if burl == "" {
		dir, err := ioutil.TempDir("", "gocdk-order")
		if err != nil {
			return nil, nil, nil, err
		}
		burl = "file://" + filepath.ToSlash(dir)
		addCleanup(func() { os.Remove(dir) })
	}
	bucket, err := blob.OpenBucket(ctx, burl)
	if err != nil {
		return nil, nil, nil, err
	}
	addCleanup(func() { bucket.Close() })

	coll, err := docstore.OpenCollection(ctx, conf.collectionURL)
	if err != nil {
		return nil, nil, nil, err
	}
	addCleanup(func() { coll.Close() })

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
	return f, p, cleanup, nil
}
