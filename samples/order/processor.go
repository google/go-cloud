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
	"strings"
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
	requestSub *pubsub.Subscription
	bucket     *blob.Bucket
	coll       *docstore.Collection
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
	order, err := createOrFindOrder(ctx, p.coll, &req)
	if err != nil {
		// There was a problem with the database, perhaps due to the network.
		// Nack the message; perhaps another processor can succeed.
		if msg.Nackable() {
			msg.Nack()
		}
		// Assume the database error is permanent: terminate processing.
		return err
	}
	if order == nil {
		log.Printf("duplicate finished order %v", req.ID)
		// We've already processed this order, so ack the message.
		msg.Ack()
		return nil
	}
	// At this point, order is an unfinished order in the database.
	// Process it.
	err = p.processOrder(ctx, order)
	// Any processing errors are saved as notes in the order.
	if err != nil {
		order.Note = fmt.Sprintf("processing failed: %v", err)
		order.OutImage = ""
	}
	// Save the finished order to the database.
	err = p.coll.Update(ctx, order, docstore.Mods{
		"OutImage":   order.OutImage,
		"Note":       order.Note,
		"FinishTime": time.Now(),
	})
	if err != nil {
		// We couldn't save the order to the database.
		// Nack the message; perhaps another processor can succeed.
		if msg.Nackable() {
			msg.Nack()
		}
		// Assume the database error is permanent: terminate processing.
		return err
	}
	// We've successfully processed the image.
	msg.Ack()
	return nil
}

// createOrFindOrder either creates a new order from req (the usual case), or returns an
// existing unfinished order. It returns a nil *Order if the order exists and is
// finished, that is, this request message is a duplicate.
// createOrFindOrder returns a non-nil error only for database problems.
func createOrFindOrder(ctx context.Context, coll *docstore.Collection, req *OrderRequest) (*Order, error) {
	// See if there is already a document for this order.
	order := &Order{ID: req.ID}
	err := coll.Get(ctx, order)
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
			CreateTime: req.CreateTime,
		}
		if err := coll.Create(ctx, order); err != nil {
			return nil, err
		}
		return order, nil
	}
	if order.FinishTime.IsZero() {
		// The order exists, but was not finished. Either it was abandoned by the processor that
		// was working on it (probably because the processor died), or it is in progress. Assume
		// that it was abandoned, and process it.
		return order, nil
	}
	// The order exists and was finished. This is most likely the result of a pubsub redelivery.
	// We simply ignore it.
	return nil, nil
}

// processOrder processes the order request.
func (p *processor) processOrder(ctx context.Context, order *Order) error {
	// Read the input image from the bucket.
	r, err := p.bucket.NewReader(ctx, order.InImage, nil)
	if err != nil {
		return err
	}

	defer r.Close()
	img, format, err := image.Decode(r)
	if err != nil {
		return err
	}

	// Process and write the output image.
	order.OutImage = fmt.Sprintf("%s-out.png", strings.TrimSuffix(order.InImage, "-in"))
	w, err := p.bucket.NewWriter(ctx, order.OutImage, nil)
	if err != nil {
		return err
	}
	if err := png.Encode(w, img); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	order.Note = fmt.Sprintf("converted from %s to png", format)
	return nil
}
