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

// TODO(jba): use the response subscription to dynamically update a "My Orders" page.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob"
	"gocloud.dev/docstore"
	_ "gocloud.dev/docstore/memdocstore"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	"gocloud.dev/requestlog"
	"gocloud.dev/server"
)

// A frontend is a web server that takes image-processing orders.
type frontend struct {
	requestTopic *pubsub.Topic
	responseSub  *pubsub.Subscription
	bucket       *blob.Bucket
	coll         *docstore.Collection
}

var (
	listTemplate      = template.Must(template.ParseFiles("list.htmlt"))
	orderFormTemplate = template.Must(template.ParseFiles("order-form.htmlt"))
)

// run starts the server on port and runs it indefinitely.
func (f *frontend) run(ctx context.Context, port int) error {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "index.html") })
	http.HandleFunc("/orders/", wrap(f.listOrders))
	http.HandleFunc("/orders/new", wrap(f.orderForm))
	http.HandleFunc("/createOrder", wrap(f.createOrder))

	rl := requestlog.NewNCSALogger(os.Stdout, func(err error) { fmt.Fprintf(os.Stderr, "%v\n", err) })
	s := server.New(nil, &server.Options{
		RequestLogger: rl,
	})
	return s.ListenAndServe(fmt.Sprintf(":%d", port))
}

// wrap turns handlers that return error into ordinary http.Handlers, by calling http.Error on non-nil
// errors.
func wrap(f func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := f(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// orderForm serves a page that lets the user input a new order.
func (*frontend) orderForm(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "GET" {
		http.Error(w, "bad method for orderForm: want GET", http.StatusBadRequest)
		return nil
	}
	return executeTemplate(orderFormTemplate, nil, w)
}

// createOrder handles a submitted order form.
func (f *frontend) createOrder(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "POST" {
		http.Error(w, "bad method for createOrder: want POST", http.StatusBadRequest)
		return nil
	}
	email := r.FormValue("email")
	if email == "" {
		http.Error(w, "email missing", http.StatusBadRequest)
		return nil
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := f.doCreateOrder(r.Context(), email, file, time.Now()); err != nil {
		return err
	}
	fmt.Fprintln(w, "Order received. Thank you.")
	return nil
}

// doCreateOrder returns the order ID it generates, for testing.
func (f *frontend) doCreateOrder(ctx context.Context, email string, file io.Reader, now time.Time) (id string, err error) {
	// Assign an ID for the order here, rather than in the processor.
	// That allows the processor to detect duplicate pub/sub messages.
	id = f.newID()
	req := &OrderRequest{
		ID:         id,
		InImage:    id + "-in",
		Email:      email,
		CreateTime: now,
	}

	// Copy the uploaded input file to the bucket.
	w, err := f.bucket.NewWriter(ctx, req.InImage, nil)
	if err != nil {
		return "", err
	}
	defer func() {
		err2 := w.Close()
		if err == nil {
			err = err2
		}
	}()

	_, err = io.Copy(w, file)
	if err != nil {
		return "", err
	}

	defer func() {
		// if we can't send the request, the image will never be processed.
		// Try to delete it.
		if err != nil {
			if err := f.bucket.Delete(ctx, req.InImage); err != nil {
				log.Printf("deleting orphan image %q: %v", req.InImage, err)
			}
		}
	}()

	// Publish the new order.
	bytes, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	if err := f.requestTopic.Send(ctx, &pubsub.Message{Body: bytes}); err != nil {
		return "", err
	}
	return id, nil
}

// listOrders lists all the orders in the database.
func (f *frontend) listOrders(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "GET" {
		http.Error(w, "bad method for listOrders: want GET", http.StatusBadRequest)
		return nil
	}
	ctx := r.Context()
	iter := f.coll.Query().Get(ctx)
	var orders []*Order
	for {
		var ord Order
		err := iter.Next(ctx, &ord)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		orders = append(orders, &ord)
	}
	return executeTemplate(listTemplate, orders, w)
}

// newID creates a new unique ID for an incoming order. It uses the current
// second, formatted in a readable way. The resulting IDs sort nicely and are
// easy to read, but of course are not suitable for production because there
// could be more than one request in a second and because the clock can be
// reset to the past, resulting in duplicates.
func (f *frontend) newID() string {
	return time.Now().Format("060102-150405")
}

// executeTemplate executes t into a buffer using data, and if that succeeds it
// writes the bytes to w.
func executeTemplate(t *template.Template, data interface{}, w http.ResponseWriter) error {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return err
	}
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("write failed: %v", err)
	}
	return nil
}
