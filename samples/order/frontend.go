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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob"
	"gocloud.dev/docstore"
	_ "gocloud.dev/docstore/memdocstore"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	"gocloud.dev/server"
	"gocloud.dev/server/requestlog"
)

// A frontend is a web server that takes image-processing orders.
type frontend struct {
	requestTopic *pubsub.Topic
	bucket       *blob.Bucket
	coll         *docstore.Collection
}

var (
	listTemplate      *template.Template
	orderFormTemplate *template.Template
)

func init() {
	// Work around a bug in go test where -coverpkg=./... uses the wrong
	// working directory (golang.org/issue/33016).
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if filepath.Base(dir) != "order" {
		// The bug puts us in a sibling directory.
		log.Printf("working around #33016, which put us in %s", dir)
		dir = filepath.Join(filepath.Dir(dir), "order")
	}
	listTemplate = template.Must(template.ParseFiles(filepath.Join(dir, "list.htmlt")))
	orderFormTemplate = template.Must(template.ParseFiles(filepath.Join(dir, "order-form.htmlt")))
}

// run starts the server on port and runs it indefinitely.
func (f *frontend) run(ctx context.Context, port int) error {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "index.html") })
	http.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "style.css") })
	http.HandleFunc("/orders/", wrapHTTPError(f.listOrders))
	http.HandleFunc("/orders/new", wrapHTTPError(f.orderForm))
	http.HandleFunc("/createOrder", wrapHTTPError(f.createOrder))
	http.HandleFunc("/show/", wrapHTTPError(f.showImage))

	rl := requestlog.NewNCSALogger(os.Stdout, func(err error) { fmt.Fprintf(os.Stderr, "%v\n", err) })
	s := server.New(nil, &server.Options{
		RequestLogger: rl,
	})
	return s.ListenAndServe(fmt.Sprintf(":%d", port))
}

// wrapHTTPError turns handlers that return error into ordinary http.Handlers,
// by calling http.Error on non-nil errors.
func wrapHTTPError(f func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) {
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

// doCreateOrder creates a new order.
// It is passed the customer's email address, an io.Reader for reading the input
// image, and the current time.
// It creates an Order in the database and sends an OrderRequest over the pub/sub topic.
// It returns the order ID it generates, for testing.
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
	_, err = io.Copy(w, file)
	if err != nil {
		_ = w.Close() // ignore error
		return "", err
	}
	if err := w.Close(); err != nil {
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

func (f *frontend) showImage(w http.ResponseWriter, r *http.Request) error {
	objKey := strings.TrimPrefix(r.URL.Path, "/show/")
	reader, err := f.bucket.NewReader(r.Context(), objKey, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("file %q not found", objKey), http.StatusNotFound)
		return nil
	}
	defer reader.Close()
	if _, err := io.Copy(w, reader); err != nil {
		log.Printf("copy from %q failed: %v", objKey, err)
	}
	return nil
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
func executeTemplate(t *template.Template, data any, w http.ResponseWriter) error {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return err
	}
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("write failed: %v", err)
	}
	return nil
}
