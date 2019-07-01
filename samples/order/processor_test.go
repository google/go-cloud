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

func TestProcessorRun(t *testing.T) {
	f, p, cleanup, err := setup(testConfig("ProcessorRun"))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	ctx := context.Background()
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

	req := &OrderRequest{ID: "x"}
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
	want := OrderResponse{ID: "x"}
	if !cmp.Equal(got, want) {
		t.Errorf("got  %+v, want %+v", got, want)
	}
}

func TestProcessOrder(t *testing.T) {
	_, p, cleanup, err := setup(testConfig("ProcessOrder"))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	ctx := context.Background()
	for _, test := range []struct {
		filename string
		want     *Order
		wantErr  bool
	}{
		{
			"cat1",
			&Order{
				ID:       "cat1",
				Email:    "joe@example.com",
				InImage:  "cat1",
				OutImage: "cat1-out.png",
				Note:     "converted from jpeg to png",
			},
			false,
		},
		{
			"bad-image",
			nil,
			true,
		},
	} {
		if err := copyFileToBucket("testdata/"+test.filename, p.bucket); err != nil {
			t.Fatal(err)
		}
		got := &Order{
			ID:      test.filename,
			Email:   "joe@example.com",
			InImage: test.filename,
		}
		err := p.processOrder(ctx, got)
		if err == nil && test.wantErr {
			t.Errorf("%s: got nil, want error", test.filename)
		} else if err != nil && !test.wantErr {
			t.Errorf("%s: got error %v, want nil", test.filename, err)
		} else if err == nil && !cmp.Equal(got, test.want) {
			t.Errorf("%s:\ngot  %+v\nwant %+v", test.filename, got, test.want)
		}
	}
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
