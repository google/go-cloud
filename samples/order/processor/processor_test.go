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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/blob"
	"gocloud.dev/docstore"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	"gocloud.dev/samples/order/common"
)

func TestRun(t *testing.T) {
	const (
		requestTopic  = "mem://requests"
		responseTopic = "mem://responses"
	)

	ctx := context.Background()
	dir, err := ioutil.TempDir("", "gocdk-order-process")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	reqTopic, err := pubsub.OpenTopic(ctx, requestTopic)
	if err != nil {
		t.Fatal(err)
	}

	p, err := newProcessor(ctx, requestTopic, responseTopic, "file:///"+dir, "mem://orders/ID")
	if err != nil {
		t.Fatal(err)
	}

	if err := copyFileToBucket("../testdata/cat1", p.bucket); err != nil {
		t.Fatal(err)
	}
	resSub, err := pubsub.OpenSubscription(ctx, responseTopic)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		if err := p.run(ctx); err != nil {
			if err != context.Canceled {
				t.Errorf("run: %v", err)
				cancel()
			}
		}
	}()

	req := &common.OrderRequest{
		ID:      "x",
		InImage: "cat1",
		Email:   "robin@example.com",
	}
	if err := sendJSON(ctx, reqTopic, req); err != nil {
		t.Fatal(err)
	}
	var got common.OrderResponse
	msg, err := receiveJSON(ctx, resSub, &got)
	if err != nil {
		t.Fatal(err)
	}
	msg.Ack()
	want := common.OrderResponse{
		ID:       "x",
		OutImage: "cat1-out.png",
		Note:     "converted from jpeg to png",
	}
	if !cmp.Equal(got, want) {
		t.Errorf("\ngot  %+v\nwant %+v", got, want)
	}

}

func TestHandleOrder(t *testing.T) {
	ctx := context.Background()
	dir, err := ioutil.TempDir("", "gocdk-order-process")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	bucket, err := blob.OpenBucket(ctx, "file:///"+dir)
	if err != nil {
		t.Fatal(err)
	}
	coll, err := docstore.OpenCollection(ctx, "mem://orders/ID")
	if err != nil {
		t.Fatal(err)
	}
	p := &processor{bucket: bucket, coll: coll}

	for _, test := range []struct {
		filename string
		want     *common.OrderResponse
	}{
		{
			"cat1",
			&common.OrderResponse{ID: "cat1", OutImage: "cat1-out.png", Note: "converted from jpeg to png"},
		},
		{
			"bad-image",
			&common.OrderResponse{ID: "bad-image", Note: "processing failed: image: unknown format"},
		},
		{
			"cat1",
			nil, // duplicate order
		},
	} {
		if err := copyFileToBucket("../testdata/"+test.filename, bucket); err != nil {
			t.Fatal(err)
		}
		got := p.handleOrder(ctx, &common.OrderRequest{ID: test.filename, InImage: test.filename})
		if !cmp.Equal(got, test.want) {
			t.Errorf("%s:\ngot  %+v\nwant %+v", test.filename, got, test.want)
		}
	}
}

func copyFileToBucket(filename string, bucket *blob.Bucket) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return common.CopyToBucket(context.Background(), bucket, filepath.Base(filename), f)
}
