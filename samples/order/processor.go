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
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"log"
	"time"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob"
	"gocloud.dev/docstore"
	_ "gocloud.dev/docstore/memdocstore"
	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub"
)

// A processor holds the state for processing images.
type processor struct {
	requestSub    *pubsub.Subscription
	responseTopic *pubsub.Topic
	bucket        *blob.Bucket
	coll          *docstore.Collection
}

// run handles requests until the context is done or there is a fatal error.
func (p *processor) run(ctx context.Context) error {
	for {
		if err := p.handleRequest(ctx); err != nil {
			return err
		}
	}
	return nil
}

// handleRequest handles one image-processing request.
// A non-nil error from handleRequest will end request processing.
func (p *processor) handleRequest(ctx context.Context) error {
	msg, err := p.requestSub.Receive(ctx)
	if err != nil {
		// If we can't receive messages, we should stop processing.
		return err
	}

	var req OrderRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		// We can't unmarshal the message body. That could be due to a bug or
		// change in the frontend, or maybe some other program is sending
		// malformed messages.

		// Ack the message, because if we can't unmarshal it then no one else can either.
		msg.Ack()
		// Don't terminate processing; maybe this is just one bad message.
		log.Printf("unmarshaling request: %v", err)
		return nil
	}
	log.Printf("received %+v", req)
	res := p.handleOrder(ctx, &req)
	if res == nil {
		log.Printf("duplicate finished order %v", req.ID)
		// We've already processed this order, so ack the message.
		msg.Ack()
		return nil
	}
	bytes, err := json.Marshal(res)
	if err != nil {
		// If we constructed a response that we can't marshal, then we are
		// buggy. Ack the message because trying it again won't help: it will
		// fail the same way on all processors.
		msg.Ack()
		// Return an error to terminate processing.
		return err
	}
	if err := p.responseTopic.Send(ctx, &pubsub.Message{Body: bytes}); err != nil {
		// We failed to send, perhaps for network reasons.
		// It's unlikely that Ack (or Nack) would even work, so don't bother.
		// Return an error to terminate processing.
		return err
	}
	// We've successfully processed the image and sent the response.
	msg.Ack()
	log.Printf("sent %+v", res)
	return nil
}

// handleOrder processes the order request, converting errors into special responses.
func (p *processor) handleOrder(ctx context.Context, req *OrderRequest) *OrderResponse {
	res, err := p.processOrder(ctx, req)
	if err != nil {
		// TODO(jba): record error metric
		res = &OrderResponse{
			ID:   req.ID,
			Note: fmt.Sprintf("processing failed: %v", err),
		}
	}
	return res
}

// processOrder process the order request.
func (p *processor) processOrder(ctx context.Context, req *OrderRequest) (res *OrderResponse, err error) {
	// See if there is already a document for this order.
	order := &Order{ID: req.ID}
	err = p.coll.Get(ctx, order)
	if err != nil {
		if gcerrors.Code(err) != gcerrors.NotFound {
			return nil, err
		}
		// Normal case: the order wasn't found, because it hasn't been created
		// yet. Create it.
		order = &Order{
			ID:         req.ID,
			Email:      req.Email,
			InImage:    req.InImage,
			OutImage:   fmt.Sprintf("%s-out.png", req.InImage),
			CreateTime: req.CreateTime,
		}
		if err := p.coll.Create(ctx, order); err != nil {
			return nil, err
		}
	} else if order.FinishTime.IsZero() {
		// The order exists, but was not finished. Either it was abandoned by the processor that
		// was working on it (probably because the processor died), or it is in progress. Assume
		// that it was abandoned, and process it.
		// There is nothing to do here, since all the existing order fields are valid.
	} else {
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

	return &OrderResponse{
		ID:       req.ID,
		OutImage: order.OutImage,
		Note:     fmt.Sprintf("converted from %s to png", format),
	}, nil
}
