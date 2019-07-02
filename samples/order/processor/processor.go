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

// A processor processes images in orders. It receives requests by
// subscribing to the requests topic, and writes responses to the response
// topic.
//
// At present, it just encodes input images into PNG format.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"log"
	"os"
	"time"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob"
	"gocloud.dev/docstore"
	_ "gocloud.dev/docstore/memdocstore"
	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	"gocloud.dev/samples/order/internal/common"
)

// To use the default values of the flags, the following environment variables must be set:
//
// RABBIT_SERVER_URL to the URL of a running RabbitMQ server
// MONGO_SERVER_URL to the URL of a running MongoDB server
//
// Also, the RabbitMQ instance must have an order-requests exchange with an order-responses queue
// bound to it.
var (
	requestSubURL    = flag.String("request-sub", "rabbit://order-requests", "gocloud.dev/pubsub URL for request subscription")
	responseTopicURL = flag.String("response-topic", "rabbit://order-responses", "gocloud.dev/pubsub URL for response topic")
	bucketURL        = flag.String("bucket", "", "gocloud.dev/blob URL for image bucket")
	collectionURL    = flag.String("collection", "mongo://order-sample/orders?id_field=ID", "gocloud.dev/docstore URL for order collection")
)

func main() {
	flag.Parse()
	if *bucketURL == "" {
		*bucketURL = "file://" + os.TempDir()
	}
	ctx := context.Background()
	p, err := newProcessor(ctx, *requestSubURL, *responseTopicURL, *bucketURL, *collectionURL)
	if err != nil {
		log.Fatal(err)
	}
	if err := p.run(ctx); err != nil {
		log.Fatal(err)
	}
}

type processor struct {
	requestSub    *pubsub.Subscription
	responseTopic *pubsub.Topic
	bucket        *blob.Bucket
	coll          *docstore.Collection
}

func newProcessor(ctx context.Context, subURL, topicURL, bucketURL, collURL string) (*processor, error) {
	reqSub, err := pubsub.OpenSubscription(ctx, subURL)
	if err != nil {
		return nil, err
	}
	resTopic, err := pubsub.OpenTopic(ctx, topicURL)
	if err != nil {
		return nil, err
	}
	bucket, err := blob.OpenBucket(ctx, bucketURL)
	if err != nil {
		return nil, err
	}
	coll, err := docstore.OpenCollection(ctx, collURL)
	if err != nil {
		return nil, err
	}
	return &processor{
		requestSub:    reqSub,
		responseTopic: resTopic,
		bucket:        bucket,
		coll:          coll,
	}, nil
}

func (p *processor) run(ctx context.Context) error {
	for {
		if err := p.handleRequest(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (p *processor) handleRequest(ctx context.Context) error {
	msg, err := p.requestSub.Receive(ctx)
	if err != nil {
		return err
	}
	// Ack the message because we handled it, even on error.
	defer msg.Ack()

	var req common.OrderRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return err
	}
	log.Printf("received %+v", req)
	res := p.handleOrder(ctx, &req)
	if res == nil {
		log.Printf("duplicate finished order %v", req.ID)
		return nil
	}
	bytes, err := json.Marshal(res)
	if err != nil {
		return err
	}
	if err := p.responseTopic.Send(ctx, &pubsub.Message{Body: bytes}); err != nil {
		return err
	}
	log.Printf("sent %+v", res)
	return nil
}

// handleOrder processes the order request. A processing error is a kind of response.
func (p *processor) handleOrder(ctx context.Context, req *common.OrderRequest) *common.OrderResponse {
	res, err := p.processOrder(ctx, req)
	if err != nil {
		// TODO(jba): record error metric
		res = &common.OrderResponse{
			ID:   req.ID,
			Note: fmt.Sprintf("processing failed: %v", err),
		}
	}
	return res
}

func (p *processor) processOrder(ctx context.Context, req *common.OrderRequest) (res *common.OrderResponse, err error) {
	// See if there is already a document for this order.
	order := &common.Order{ID: req.ID}
	err = p.coll.Get(ctx, order)
	switch {
	case gcerrors.Code(err) == gcerrors.NotFound:
		// Normal case: the order hasn't been created yet. Do so.
		order = &common.Order{
			ID:         req.ID,
			Email:      req.Email,
			InImage:    req.InImage,
			OutImage:   fmt.Sprintf("%s-out.png", req.InImage),
			CreateTime: req.CreateTime,
		}
		if err := p.coll.Create(ctx, order); err != nil {
			return nil, err
		}

	case err == nil && order.FinishTime.IsZero():
		// The order exists, but was not finished. Either it was abandoned by the processor that
		// was working on it (probably because the processor died), or it is in progress. Assume
		// that it was abandoned, and process it.
		// There is nothing to do here, since all the existing order fields are valid.

	case err == nil && !order.FinishTime.IsZero():
		// The order exists and was finished. This is most likely the result of a pubsub redelivery.
		// We simply ignore it.
		return nil, nil
	}
	// At this point, there is an unfinished Order with ID == req.ID in the database.

	defer func() {
		// Mark the order complete by updating the finish time.
		err2 := p.coll.Update(ctx, order, docstore.Mods{"FinishTime": time.Now()})
		if err2 != nil {
			if err == nil {
				err = err2
			} else {
				err = fmt.Errorf("%v and %v", err, err2)
			}
		}
	}()

	// Read the input image from the bucket.
	r, err := p.bucket.NewReader(ctx, req.InImage, nil)
	if err != nil {
		return nil, err
	}

	defer r.Close()
	img, format, err := image.Decode(r)
	if err != nil {
		return nil, err
	}

	// Process and write the output image.
	w, err := p.bucket.NewWriter(ctx, order.OutImage, nil)
	if err != nil {
		return nil, err
	}
	if err := png.Encode(w, img); err != nil {
		w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	return &common.OrderResponse{
		ID:       req.ID,
		OutImage: order.OutImage,
		Note:     fmt.Sprintf("converted from %s to png", format),
	}, nil
}
