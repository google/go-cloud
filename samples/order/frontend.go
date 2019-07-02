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
	"gocloud.dev/blob"
	"gocloud.dev/docstore"
	"gocloud.dev/pubsub"
)

// TODO(jba): implement

type frontend struct {
	requestTopic *pubsub.Topic
	responseSub  *pubsub.Subscription
	bucket       *blob.Bucket
	coll         *docstore.Collection
}
