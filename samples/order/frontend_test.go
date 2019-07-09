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
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOrderForm(t *testing.T) {
	f, _, cleanup, err := setup(testConfig("OrderForm"))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/orders/new", nil)
	if err := f.orderForm(w, r); err != nil {
		t.Fatal(err)
	}
	res := w.Result()
	if res.StatusCode != 200 {
		t.Fatalf("got %d, want 200", res.StatusCode)
	}
	wantb, err := ioutil.ReadFile("order-form.htmlt")
	if err != nil {
		t.Fatal(err)
	}
	gotb, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	got := string(gotb)
	want := string(wantb)

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCreateOrder(t *testing.T) {
	f, _, cleanup, err := setup(testConfig("CreateOrder"))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	ctx := context.Background()
	file := strings.NewReader("an image")
	tm := time.Date(2019, 7, 1, 0, 0, 0, 0, time.Local)
	id, err := f.doCreateOrder(ctx, "pat@example.com", file, tm)
	if err != nil {
		t.Fatal(err)
	}

	r, err := f.bucket.NewReader(ctx, id+"-in", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	gotb, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	gots := string(gotb)
	wants := "an image"
	if gots != wants {
		t.Errorf("got %q, want %q", gots, wants)
	}
}

func TestListOrders(t *testing.T) {
	f, _, cleanup, err := setup(testConfig("ListOrders"))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	ctx := context.Background()
	// Clear the collection.
	if err := f.coll.Query().Delete(ctx); err != nil {
		t.Fatal(err)
	}
	orders := []*Order{
		{ID: "a", Email: "pat@example.com", InImage: "a-in", CreateTime: time.Now()},
		{ID: "b", Email: "mel@example.com", InImage: "b-in", CreateTime: time.Now()},
	}
	actions := f.coll.Actions()
	for _, ord := range orders {
		actions.Put(ord)
	}
	if err := actions.Do(ctx); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/orders", nil)
	if err := f.listOrders(w, r); err != nil {
		t.Fatal(err)
	}

	res := w.Result()
	if res.StatusCode != 200 {
		t.Fatalf("got %d, want 200", res.StatusCode)
	}
	gotb, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	got := string(gotb)
	for _, ord := range orders {
		if !strings.Contains(got, ord.ID) {
			t.Errorf("got %q, should contain %q", got, ord.ID)
		}
		if !strings.Contains(got, ord.Email) {
			t.Errorf("got %q, should contain %q", got, ord.Email)
		}
	}
}

func testConfig(name string) config {
	reqURL := "mem://requests-" + name
	return config{
		requestTopicURL: reqURL,
		requestSubURL:   reqURL,
		bucketURL:       "", // setup will use fileblob with a temporary dir
		collectionURL:   fmt.Sprintf("mem://orders-%s/ID", name),
	}
}
