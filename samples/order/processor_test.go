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
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/blob"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub"
)

func TestProcessor(t *testing.T) {
	ctx := context.Background()
	f, p, cleanup, err := setup()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	t.Run("Run", func(t *testing.T) {
		if err := copyFileToBucket("testdata/cat1", p.bucket); err != nil {
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

		req := &OrderRequest{
			ID:      "x",
			InImage: "cat1",
			Email:   "robin@example.com",
		}
		bytes, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.requestTopic.Send(ctx, &pubsub.Message{Body: bytes}); err != nil {
			t.Fatal(err)
		}
		msg, err := f.responseSub.Receive(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var got OrderResponse
		if err := json.Unmarshal(msg.Body, &got); err != nil {
			t.Fatal(err)
		}
		msg.Ack()
		want := OrderResponse{
			ID:       "x",
			OutImage: "cat1-out.png",
			Note:     "converted from jpeg to png",
		}
		if !cmp.Equal(got, want) {
			t.Errorf("\ngot  %+v\nwant %+v", got, want)
		}
	})

	t.Run("HandleOrder", func(t *testing.T) {
		for _, test := range []struct {
			filename string
			want     *OrderResponse
		}{
			{
				"cat1",
				&OrderResponse{ID: "cat1", OutImage: "cat1-out.png", Note: "converted from jpeg to png"},
			},
			{
				"bad-image",
				&OrderResponse{ID: "bad-image", Note: "processing failed: image: unknown format"},
			},
			{
				"cat1",
				nil, // duplicate order
			},
		} {
			if err := copyFileToBucket("testdata/"+test.filename, p.bucket); err != nil {
				t.Fatal(err)
			}
			got := p.handleOrder(ctx, &OrderRequest{ID: test.filename, InImage: test.filename})
			if !cmp.Equal(got, test.want) {
				t.Errorf("%s:\ngot  %+v\nwant %+v", test.filename, got, test.want)
			}
		}
	})
}

func copyFileToBucket(filename string, bucket *blob.Bucket) (err error) {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	w, err := bucket.NewWriter(context.Background(), filepath.Base(filename), nil)
	if err != nil {
		return err
	}
	defer func() {
		err2 := w.Close()
		if err == nil {
			err = err2
		}
	}()

	_, err = io.Copy(w, f)
	return err
}
