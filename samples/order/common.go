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
	"time"
)

// Order represents an order for a single image operation.
type Order struct {
	ID               string    // unique ID, randomly generated
	Email            string    // email address of customer
	InImage          string    // name of input image
	OutImage         string    // name of output image; empty if there was an error
	CreateTime       time.Time // time the order was created
	FinishTime       time.Time // time the order was finished
	Note             string    // note to the customer from the processor, describing success or error
	DocstoreRevision any
}

// OrderRequest is a request for an order. It is the contents of the messages
// sent to the requests topic.
type OrderRequest struct {
	ID         string
	Email      string
	InImage    string
	CreateTime time.Time
}
