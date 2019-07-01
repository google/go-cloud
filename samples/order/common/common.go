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

// Package common holds types and functions that are used by both the processor
// and the frontend programs.
package common

import (
	"context"
	"io"
	"time"

	"gocloud.dev/blob"
)

// Order represents an order for a single picture operation.
type Order struct {
	ID               string    // unique ID, randomly generated
	Email            string    // email address of customer
	InImage          string    // name of input image
	OutImage         string    // name of output image
	CreateTime       time.Time // time the order was created
	FinishTime       time.Time // time the order was finished
	DocstoreRevision interface{}
}

// OrderRequest is a request for an order. It is the contents of the messages
// sent to the requests topic.
type OrderRequest struct {
	ID         string
	Email      string
	InImage    string
	CreateTime time.Time
}

// OrderResponse describes the result of an order. It is the contents of the
// messages sent to response topic.
type OrderResponse struct {
	ID       string
	OutImage string // if empty, error; Note contains the problem
	Note     string // for the customer
}

// CopyToBucket copies r to bucket under the given name.
func CopyToBucket(ctx context.Context, bucket *blob.Bucket, name string, r io.Reader) error {
	w, err := bucket.NewWriter(ctx, name, nil)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close() // We must close the writer, but we don't care about the error.
		return err
	}
	return w.Close()
}
