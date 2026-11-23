// Copyright 2026 The Go Cloud Development Kit Authors
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

package awsdynamodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	dyn "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dyn2Types "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/googleapis/gax-go/v2"
	"gocloud.dev/docstore"
	"gocloud.dev/gcerrors"
)

// fakeDB is a fake DynamoDB server that supports BatchGetItem on one table.
// Like DynamoDB, it compares numbers in keys by value.
type fakeDB struct {
	t       *testing.T
	keys    []string // key attribute names
	maxResp int      // if > 0, the most items a BatchGetItem call returns; if < 0, it returns none
	errAt   int      // if > 0, the number of the BatchGetItem call that fails
	onCall  func(n int)

	mu    sync.Mutex
	items map[string]map[string]any // by fakeKey
	calls []fakeCall                // each BatchGetItem call
}

type fakeCall struct {
	keys       []map[string]any
	projection string
	consistent bool
}

type attr = map[string]any

func newFakeDB(t *testing.T, keys ...string) *fakeDB {
	return &fakeDB{t: t, keys: keys, items: map[string]map[string]any{}}
}

// put adds an item. Values are DynamoDB JSON attribute values.
func (f *fakeDB) put(item map[string]any) {
	f.items[f.fakeKey(item)] = item
}

func (f *fakeDB) fakeKey(item map[string]any) string {
	var parts []string
	for _, k := range f.keys {
		av := item[k].(map[string]any)
		for typ, v := range av {
			s := v.(string)
			if typ == "N" {
				r, ok := new(big.Rat).SetString(s)
				if !ok {
					f.t.Fatalf("bad number %q", s)
				}
				s = r.RatString()
			}
			parts = append(parts, typ+":"+s)
		}
	}
	return strings.Join(parts, "|")
}

func (f *fakeDB) numCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeDB) client() *dyn.Client {
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	f.t.Cleanup(srv.Close)
	return dyn.NewFromConfig(aws.Config{
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider("key", "secret", ""),
		BaseEndpoint: aws.String(srv.URL),
	})
}

func (f *fakeDB) serve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TableName    string
		RequestItems map[string]struct {
			Keys                     []map[string]any
			ProjectionExpression     string
			ExpressionAttributeNames map[string]string
			ConsistentRead           bool
		}
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		f.t.Errorf("decoding request: %v", err)
	}
	var resp any
	switch op := r.Header.Get("X-Amz-Target"); op {
	case "DynamoDB_20120810.DescribeTable":
		resp = map[string]any{"Table": map[string]any{"TableName": req.TableName}}
	case "DynamoDB_20120810.BatchGetItem":
		f.mu.Lock()
		responses := map[string][]any{}
		unprocessed := map[string]any{}
		var n int
		for table, ka := range req.RequestItems {
			f.calls = append(f.calls, fakeCall{ka.Keys, ka.ProjectionExpression, ka.ConsistentRead})
			n = len(f.calls)
			keys := ka.Keys
			// Unlike DynamoDB, don't return the projection or consistency
			// settings with the unprocessed keys, so that tests check that
			// retries keep them anyway.
			if limit := max(f.maxResp, 0); f.maxResp != 0 && len(keys) > limit {
				unprocessed[table] = map[string]any{"Keys": keys[limit:]}
				keys = keys[:limit]
			}
			for _, key := range keys {
				if item, ok := f.items[f.fakeKey(key)]; ok {
					responses[table] = append(responses[table], item)
				}
			}
			// DynamoDB returns items in no particular order.
			slices.Reverse(responses[table])
		}
		errAt, onCall := f.errAt, f.onCall
		f.mu.Unlock()
		if onCall != nil {
			onCall(n)
		}
		if n == errAt {
			w.Header().Set("Content-Type", "application/x-amz-json-1.0")
			w.WriteHeader(http.StatusBadRequest)
			if _, err := fmt.Fprint(w, `{"__type":"com.amazonaws.dynamodb.v20120810#ResourceNotFoundException","message":"fake failure"}`); err != nil {
				f.t.Errorf("writing error response: %v", err)
			}
			return
		}
		resp = map[string]any{"Responses": responses, "UnprocessedKeys": unprocessed}
	default:
		f.t.Errorf("unexpected operation %q", op)
	}
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		f.t.Errorf("encoding response: %v", err)
	}
}

func openFake(t *testing.T, f *fakeDB, pkey, skey string) *docstore.Collection {
	t.Helper()
	return openFakeOpts(t, f, pkey, skey, nil)
}

func openFakeOpts(t *testing.T, f *fakeDB, pkey, skey string, opts *Options) *docstore.Collection {
	t.Helper()
	coll, err := OpenCollection(f.client(), "t", pkey, skey, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := coll.Close(); err != nil {
			t.Errorf("failed to Close: %v", err)
		}
	})
	return coll
}

func num(n string) attr { return attr{"N": n} }
func str(s string) attr { return attr{"S": s} }

// Each Get must get its own item, whatever Go type the document uses for its
// key, and however many BatchGetItem calls are needed.
func TestBatchGetNumericKeyTypes(t *testing.T) {
	ctx := context.Background()
	f := newFakeDB(t, "id")
	for i := 1; i <= 150; i++ {
		f.put(map[string]any{"id": num(fmt.Sprint(i)), "V": str(fmt.Sprintf("v%d", i))})
	}
	coll := openFake(t, f, "id", "")

	check := func(t *testing.T, n int, newDoc func(i int) any, got func(d any) (id any, v any)) {
		t.Helper()
		var docs []any
		actions := coll.Actions()
		for i := 1; i <= n; i++ {
			d := newDoc(i)
			docs = append(docs, d)
			actions.Get(d)
		}
		if err := actions.Do(ctx); err != nil {
			t.Fatal(err)
		}
		for i, d := range docs {
			id, v := got(d)
			if want := fmt.Sprintf("v%d", i+1); v != want || fmt.Sprint(id) != fmt.Sprint(i+1) {
				t.Errorf("Get %d: got id %v, V %v; want id %d, V %s", i+1, id, v, i+1, want)
			}
		}
	}
	type intDoc struct {
		ID int `docstore:"id"`
		V  string
	}
	type int32Doc struct {
		ID int32 `docstore:"id"`
		V  string
	}
	type uint16Doc struct {
		ID uint16 `docstore:"id"`
		V  string
	}
	type uint64Doc struct {
		ID uint64 `docstore:"id"`
		V  string
	}
	type int64Doc struct {
		ID int64 `docstore:"id"`
		V  string
	}
	type float64Doc struct {
		ID float64 `docstore:"id"`
		V  string
	}
	for _, n := range []int{2, 150} {
		t.Run(fmt.Sprintf("int/%d", n), func(t *testing.T) {
			check(t, n, func(i int) any { return &intDoc{ID: i} }, func(d any) (any, any) { return d.(*intDoc).ID, d.(*intDoc).V })
		})
		t.Run(fmt.Sprintf("int32/%d", n), func(t *testing.T) {
			check(t, n, func(i int) any { return &int32Doc{ID: int32(i)} }, func(d any) (any, any) { return d.(*int32Doc).ID, d.(*int32Doc).V })
		})
		t.Run(fmt.Sprintf("uint16/%d", n), func(t *testing.T) {
			check(t, n, func(i int) any { return &uint16Doc{ID: uint16(i)} }, func(d any) (any, any) { return d.(*uint16Doc).ID, d.(*uint16Doc).V })
		})
		t.Run(fmt.Sprintf("uint64/%d", n), func(t *testing.T) {
			check(t, n, func(i int) any { return &uint64Doc{ID: uint64(i)} }, func(d any) (any, any) { return d.(*uint64Doc).ID, d.(*uint64Doc).V })
		})
		t.Run(fmt.Sprintf("int64/%d", n), func(t *testing.T) {
			check(t, n, func(i int) any { return &int64Doc{ID: int64(i)} }, func(d any) (any, any) { return d.(*int64Doc).ID, d.(*int64Doc).V })
		})
		t.Run(fmt.Sprintf("float64/%d", n), func(t *testing.T) {
			check(t, n, func(i int) any { return &float64Doc{ID: float64(i)} }, func(d any) (any, any) { return d.(*float64Doc).ID, d.(*float64Doc).V })
		})
		t.Run(fmt.Sprintf("map/%d", n), func(t *testing.T) {
			check(t, n, func(i int) any { return map[string]any{"id": i} }, func(d any) (any, any) { return d.(map[string]any)["id"], d.(map[string]any)["V"] })
		})
	}
}

// Integers that float64 can't hold exactly must still match their own items.
func TestBatchGetLargeIntegerKeys(t *testing.T) {
	ctx := context.Background()
	f := newFakeDB(t, "id")
	ids := []int64{1 << 53, 1<<53 + 1, 1<<53 + 2, math.MaxInt64, math.MinInt64}
	for _, id := range ids {
		f.put(map[string]any{"id": num(fmt.Sprint(id)), "V": str(fmt.Sprintf("v%d", id))})
	}
	f.put(map[string]any{"id": num(fmt.Sprint(uint64(math.MaxUint64))), "V": str("vmaxuint")})
	coll := openFake(t, f, "id", "")

	type doc struct {
		ID int64 `docstore:"id"`
		V  string
	}
	var docs []*doc
	actions := coll.Actions()
	for _, id := range ids {
		d := &doc{ID: id}
		docs = append(docs, d)
		actions.Get(d)
	}
	type udoc struct {
		ID uint64 `docstore:"id"`
		V  string
	}
	u := &udoc{ID: math.MaxUint64}
	actions.Get(u)
	if err := actions.Do(ctx); err != nil {
		t.Fatal(err)
	}
	for i, d := range docs {
		if want := fmt.Sprintf("v%d", ids[i]); d.ID != ids[i] || d.V != want {
			t.Errorf("got %+v, want {ID:%d V:%s}", *d, ids[i], want)
		}
	}
	if u.V != "vmaxuint" {
		t.Errorf("got %+v, want V vmaxuint", *u)
	}
}

// Keys of other types, and tables with a sort key.
func TestBatchGetOtherKeys(t *testing.T) {
	ctx := context.Background()
	f := newFakeDB(t, "pk", "sk")
	f.put(map[string]any{"pk": str("a"), "sk": num("1"), "V": str("a1")})
	f.put(map[string]any{"pk": str("a"), "sk": num("2"), "V": str("a2")})
	f.put(map[string]any{"pk": str("b"), "sk": num("1"), "V": str("b1")})
	f.put(map[string]any{"pk": str("c"), "sk": num("1.5"), "V": str("c1.5")})
	coll := openFake(t, f, "pk", "sk")

	type doc struct {
		PK string `docstore:"pk"`
		SK int    `docstore:"sk"`
		V  string
	}
	docs := []*doc{{PK: "b", SK: 1}, {PK: "a", SK: 2}, {PK: "a", SK: 1}, {PK: "a", SK: 3}}
	frac := map[string]any{"pk": "c", "sk": 1.5}
	actions := coll.Actions()
	for _, d := range docs {
		actions.Get(d)
	}
	actions.Get(frac)
	err := actions.Do(ctx)
	var alerr docstore.ActionListError
	if !errors.As(err, &alerr) || len(alerr) != 1 || alerr[0].Index != 3 || gcerrors.Code(alerr[0].Err) != gcerrors.NotFound {
		t.Fatalf("got %v, want NotFound for Get 3 only", err)
	}
	for i, want := range []string{"b1", "a2", "a1", ""} {
		if docs[i].V != want {
			t.Errorf("Get %d: got V %q, want %q", i, docs[i].V, want)
		}
	}
	if frac["V"] != "c1.5" {
		t.Errorf("fractional key: got %v, want V c1.5", frac)
	}
}

func TestKeyString(t *testing.T) {
	c := &collection{partitionKey: "pk", sortKey: "sk"}
	key := func(pk, sk dyn2Types.AttributeValue) map[string]dyn2Types.AttributeValue {
		m := map[string]dyn2Types.AttributeValue{}
		if pk != nil {
			m["pk"] = pk
		}
		if sk != nil {
			m["sk"] = sk
		}
		return m
	}
	s := func(v string) dyn2Types.AttributeValue { return &dyn2Types.AttributeValueMemberS{Value: v} }
	n := func(v string) dyn2Types.AttributeValue { return &dyn2Types.AttributeValueMemberN{Value: v} }
	b := func(v ...byte) dyn2Types.AttributeValue { return &dyn2Types.AttributeValueMemberB{Value: v} }
	ks := func(item map[string]dyn2Types.AttributeValue) string {
		t.Helper()
		got, err := c.keyString(item)
		if err != nil {
			t.Fatalf("keyString(%v): %v", item, err)
		}
		return got
	}

	// Same key in DynamoDB, so same string.
	for _, pair := range [][2]map[string]dyn2Types.AttributeValue{
		{key(s("a"), n("1")), key(s("a"), n("1.0"))},
		{key(s("a"), n("1")), key(s("a"), n("1E0"))},
		{key(s("a"), n("100")), key(s("a"), n("1e2"))},
		{key(s("a"), n("0")), key(s("a"), n("-0"))},
		{key(s("a"), n("0.5")), key(s("a"), n(".50"))},
		{key(b(1, 2), n("1")), key(b(1, 2), n("1"))},
	} {
		if x, y := ks(pair[0]), ks(pair[1]); x != y {
			t.Errorf("%v and %v: got %q and %q, want equal", pair[0], pair[1], x, y)
		}
	}
	// Different keys, so different strings.
	for _, pair := range [][2]map[string]dyn2Types.AttributeValue{
		{key(s("a"), n("9007199254740992")), key(s("a"), n("9007199254740993"))},
		{key(s("a"), n("1")), key(s("a"), s("1"))},
		{key(s("1"), n("1")), key(n("1"), n("1"))},
		{key(s("a"), b('1')), key(s("a"), s("1"))},
		{key(s("a:1"), s("b")), key(s("a"), s("1:b"))},
		{key(s("S1:aS1:b"), s("c")), key(s("a"), s("b"))},
	} {
		if x, y := ks(pair[0]), ks(pair[1]); x == y {
			t.Errorf("%v and %v: both got %q, want different", pair[0], pair[1], x)
		}
	}
	// Keys DynamoDB would reject.
	for _, item := range []map[string]dyn2Types.AttributeValue{
		key(s("a"), nil),
		key(nil, s("a")),
		key(s("a"), &dyn2Types.AttributeValueMemberNULL{Value: true}),
		key(s("a"), s("")),
		key(s("a"), b()),
		key(s("a"), n("abc")),
		key(s("a"), &dyn2Types.AttributeValueMemberBOOL{Value: true}),
	} {
		if _, err := c.keyString(item); err == nil {
			t.Errorf("keyString(%v): got nil error, want error", item)
		}
	}
	// No sort key.
	c2 := &collection{partitionKey: "pk"}
	if _, err := c2.keyString(key(s("a"), nil)); err != nil {
		t.Errorf("partition key only: %v", err)
	}
}

// A Get whose key DynamoDB would reject fails on its own with InvalidArgument,
// without being sent. The other Gets work.
func TestBatchGetBadKeys(t *testing.T) {
	ctx := context.Background()
	type doc struct {
		PK string `docstore:"pk"`
		SK string `docstore:"sk"`
		V  string
	}
	for _, test := range []struct {
		name string
		doc  any
	}{
		{"missing sort key", map[string]any{"pk": "a"}},
		{"nil sort key", map[string]any{"pk": "a", "sk": nil}},
		{"empty sort key", &doc{PK: "a"}},
		{"bool sort key", map[string]any{"pk": "a", "sk": true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newFakeDB(t, "pk", "sk")
			f.put(map[string]any{"pk": str("a"), "sk": str("x"), "V": str("ax")})
			f.put(map[string]any{"pk": str("a"), "sk": str("y"), "V": str("ay")})
			coll := openFake(t, f, "pk", "sk")

			// On its own.
			err := coll.Get(ctx, test.doc)
			if gcerrors.Code(err) != gcerrors.InvalidArgument {
				t.Fatalf("single Get: got %v, want InvalidArgument", err)
			}
			if n := f.numCalls(); n != 0 {
				t.Errorf("single Get: got %d BatchGetItem calls, want 0", n)
			}

			// With a good Get before and after it.
			good1 := &doc{PK: "a", SK: "x"}
			good2 := map[string]any{"pk": "a", "sk": "y"}
			err = coll.Actions().Get(good1).Get(test.doc).Get(good2, "V").Do(ctx)
			var alerr docstore.ActionListError
			if !errors.As(err, &alerr) || len(alerr) != 1 || alerr[0].Index != 1 || gcerrors.Code(alerr[0].Err) != gcerrors.InvalidArgument {
				t.Fatalf("got %v, want InvalidArgument for Get 1 only", err)
			}
			if good1.V != "ax" || good2["V"] != "ay" {
				t.Errorf("got %+v and %v, want V ax and ay", *good1, good2)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			var sent []string
			for _, call := range f.calls {
				for _, k := range call.keys {
					sent = append(sent, f.fakeKey(k))
				}
			}
			slices.Sort(sent)
			if want := []string{"S:a|S:x", "S:a|S:y"}; !slices.Equal(sent, want) {
				t.Errorf("sent keys %v, want %v", sent, want)
			}
		})
	}
}

// Two Gets of the same DynamoDB key, written differently, would make DynamoDB
// reject the request. The second one fails with InvalidArgument.
func TestBatchGetDuplicateKeys(t *testing.T) {
	ctx := context.Background()
	f := newFakeDB(t, "id")
	f.put(map[string]any{"id": num("1"), "V": str("v1")})
	f.put(map[string]any{"id": num("2"), "V": str("v2")})
	coll := openFake(t, f, "id", "")

	d1 := map[string]any{"id": 1}
	dup := map[string]any{"id": int64(1)}
	dupFloat := map[string]any{"id": 1.0}
	d2 := map[string]any{"id": 2}
	err := coll.Actions().Get(d1).Get(dup).Get(d2).Get(dupFloat).Do(ctx)
	var alerr docstore.ActionListError
	if !errors.As(err, &alerr) || len(alerr) != 2 {
		t.Fatalf("got %v, want 2 errors", err)
	}
	for _, e := range alerr {
		if (e.Index != 1 && e.Index != 3) || gcerrors.Code(e.Err) != gcerrors.InvalidArgument {
			t.Errorf("got error %v at %d, want InvalidArgument at 1 and 3", e.Err, e.Index)
		}
	}
	if d1["V"] != "v1" || d2["V"] != "v2" {
		t.Errorf("got %v and %v, want V v1 and v2", d1, d2)
	}
}

func shortBackoff(t *testing.T) {
	old := batchGetBackoff
	batchGetBackoff = gax.Backoff{Initial: time.Millisecond, Max: time.Millisecond}
	t.Cleanup(func() { batchGetBackoff = old })
}

// putN adds items with id 1 to n. (Docstore treats a zero key as missing.)
func (f *fakeDB) putN(n int) {
	for i := 1; i <= n; i++ {
		f.put(map[string]any{"id": num(fmt.Sprint(i)), "V": str(fmt.Sprintf("v%d", i))})
	}
}

// getN gets items with id 1 to n. The document and error for id i are at i-1.
func getN(ctx context.Context, coll *docstore.Collection, n int, fps ...docstore.FieldPath) ([]map[string]any, []error) {
	docs := make([]map[string]any, n)
	actions := coll.Actions()
	for i := range docs {
		docs[i] = map[string]any{"id": i + 1}
		actions.Get(docs[i], fps...)
	}
	errs := make([]error, n)
	var alerr docstore.ActionListError
	if err := actions.Do(ctx); errors.As(err, &alerr) {
		for _, e := range alerr {
			errs[e.Index] = e.Err
		}
	} else if err != nil {
		for i := range errs {
			errs[i] = err
		}
	}
	return docs, errs
}

// When DynamoDB returns only some of the items, the rest are asked for again,
// with the same projection and consistency settings.
func TestBatchGetUnprocessedKeys(t *testing.T) {
	ctx := context.Background()
	shortBackoff(t)
	f := newFakeDB(t, "id")
	f.putN(95) // ids 96 to 100 don't exist
	f.maxResp = 30
	coll := openFakeOpts(t, f, "id", "", &Options{ConsistentRead: true})

	docs, errs := getN(ctx, coll, 100, docstore.FieldPath("V"))
	for i, d := range docs {
		if i < 95 {
			if errs[i] != nil || d["V"] != fmt.Sprintf("v%d", i+1) {
				t.Errorf("Get %d: got %v, error %v; want V v%d", i, d, errs[i], i+1)
			}
		} else if gcerrors.Code(errs[i]) != gcerrors.NotFound {
			t.Errorf("Get %d: got error %v, want NotFound", i, errs[i])
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 4 {
		t.Errorf("got %d BatchGetItem calls, want 4", len(f.calls))
	}
	want := f.calls[0]
	for i, call := range f.calls {
		if call.projection == "" || call.projection != want.projection || !call.consistent {
			t.Errorf("call %d: got projection %q, consistent %t; want %q, true", i, call.projection, call.consistent, want.projection)
		}
	}
}

func TestBatchGetUnprocessedKeysBeforeDo(t *testing.T) {
	ctx := context.Background()
	shortBackoff(t)
	f := newFakeDB(t, "id")
	f.putN(10)
	f.maxResp = 3
	coll := openFake(t, f, "id", "")

	var n int
	actions := coll.Actions().BeforeDo(func(asFunc func(any) bool) error {
		var in *dyn.BatchGetItemInput
		if asFunc(&in) {
			n++
			ka := in.RequestItems["t"]
			ka.ConsistentRead = aws.Bool(true)
			in.RequestItems["t"] = ka
		}
		return nil
	})
	docs := make([]map[string]any, 10)
	for i := range docs {
		docs[i] = map[string]any{"id": i + 1}
		actions.Get(docs[i])
	}
	if err := actions.Do(ctx); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("BeforeDo called %d times, want 1", n)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 4 {
		t.Errorf("got %d BatchGetItem calls, want 4", len(f.calls))
	}
	for i, call := range f.calls {
		if !call.consistent {
			t.Errorf("call %d: lost the change made by BeforeDo", i)
		}
	}
}

// If a retry fails, Gets that already have their item keep it, and the others
// get the error.
func TestBatchGetUnprocessedKeysError(t *testing.T) {
	ctx := context.Background()
	shortBackoff(t)
	f := newFakeDB(t, "id")
	f.putN(10)
	f.maxResp = 4
	f.errAt = 2
	coll := openFake(t, f, "id", "")

	docs, errs := getN(ctx, coll, 10)
	for i, d := range docs {
		if i < 4 {
			if errs[i] != nil || d["V"] != fmt.Sprintf("v%d", i+1) {
				t.Errorf("Get %d: got %v, error %v; want V v%d", i, d, errs[i], i+1)
			}
		} else if errs[i] == nil || !strings.Contains(errs[i].Error(), "fake failure") {
			t.Errorf("Get %d: got error %v, want fake failure", i, errs[i])
		}
	}
	if got := f.numCalls(); got != 2 {
		t.Errorf("got %d BatchGetItem calls, want 2", got)
	}
}

// Retries stop when the context is done. Here it is canceled during the second
// call, so only the items from the first call are returned.
func TestBatchGetUnprocessedKeysCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	shortBackoff(t)
	f := newFakeDB(t, "id")
	f.putN(10)
	f.maxResp = 3
	f.onCall = func(n int) {
		if n == 2 {
			cancel()
		}
	}
	coll := openFake(t, f, "id", "")

	_, errs := getN(ctx, coll, 10)
	for i, err := range errs {
		if i < 3 {
			if err != nil {
				t.Errorf("Get %d: got error %v, want nil", i, err)
			}
		} else if gcerrors.Code(err) != gcerrors.Canceled {
			t.Errorf("Get %d: got error %v, want Canceled", i, err)
		}
	}
	if got := f.numCalls(); got != 2 {
		t.Errorf("got %d BatchGetItem calls, want 2", got)
	}
}

// If DynamoDB keeps processing none of the keys, batchGet gives up.
func TestBatchGetUnprocessedKeysNoProgress(t *testing.T) {
	ctx := context.Background()
	shortBackoff(t)
	f := newFakeDB(t, "id")
	f.putN(10)
	f.maxResp = -1
	coll := openFake(t, f, "id", "")

	_, errs := getN(ctx, coll, 10)
	for i, err := range errs {
		if gcerrors.Code(err) != gcerrors.ResourceExhausted {
			t.Errorf("Get %d: got error %v, want ResourceExhausted", i, err)
		}
	}
	if got := f.numCalls(); got != maxBatchGetNoProgress {
		t.Errorf("got %d BatchGetItem calls, want %d", got, maxBatchGetNoProgress)
	}
}

// Calls that process no keys don't count against the limit if a later call
// processes some.
func TestBatchGetUnprocessedKeysSlowProgress(t *testing.T) {
	ctx := context.Background()
	shortBackoff(t)
	f := newFakeDB(t, "id")
	f.putN(3)
	f.maxResp = -1
	f.onCall = func(n int) {
		f.mu.Lock()
		defer f.mu.Unlock()
		// Return one item on every fourth call.
		if n%4 == 3 {
			f.maxResp = 1
		} else {
			f.maxResp = -1
		}
	}
	coll := openFake(t, f, "id", "")

	docs, errs := getN(ctx, coll, 3)
	for i, d := range docs {
		if errs[i] != nil || d["V"] != fmt.Sprintf("v%d", i+1) {
			t.Errorf("Get %d: got %v, error %v; want V v%d", i, d, errs[i], i+1)
		}
	}
}
