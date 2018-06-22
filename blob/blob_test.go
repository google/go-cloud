// Copyright 2018 Google LLC
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

package blob

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-x-cloud/blob/driver"
)

func TestNewRangeReader(t *testing.T) {
	t.Run("EntireBlob", func(t *testing.T) {
		const key = "foo"
		spy := new(bucketSpy)
		b := NewBucket(spy)
		ctx := context.Background()
		r, err := b.NewRangeReader(ctx, key, 0, -1)
		if err != nil {
			t.Fatalf("b.NewRangeReader(ctx, %q, 0, -1): %v", key, err)
		}
		defer r.Close()
		if !spy.readCalled {
			t.Fatalf("Driver's NewRangeReader method was never called")
		}
		if spy.key != key || spy.offset != 0 || spy.length != -1 {
			t.Errorf("blob.Bucket called NewRangeReader(ctx, %q, %d, %d); want NewRangeReader(ctx, %q, 0, -1)", spy.key, spy.offset, spy.length, key)
		}
	})
	t.Run("NegativeOffset", func(t *testing.T) {
		spy := new(bucketSpy)
		b := NewBucket(spy)
		ctx := context.Background()
		r, err := b.NewRangeReader(ctx, "foo", -1, -1)
		if err == nil {
			r.Close()
			t.Error("b.NewRangeReader(ctx, \"foo\", -1, -1) did not return error")
		}
		if spy.readCalled {
			t.Error("Driver's NewRangeReader method was called")
		}
	})
}

type bucketSpy struct {
	key    string
	offset int64
	length int64

	readCalled bool
}

func (b *bucketSpy) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
	b.readCalled = true
	b.key = key
	b.offset = offset
	b.length = length
	return readerStub{}, nil
}

func (b *bucketSpy) NewWriter(context.Context, string, string, *driver.WriterOptions) (driver.Writer, error) {
	return nil, errors.New("unimplemented")
}

func (b *bucketSpy) Delete(context.Context, string) error {
	return errors.New("unimplemented")
}

type readerStub struct{}

func (readerStub) Read([]byte) (int, error) {
	return 0, errors.New("unimplemented")
}

func (readerStub) Attrs() *driver.ObjectAttrs {
	return &driver.ObjectAttrs{}
}

func (readerStub) Close() error {
	return errors.New("unimplemented")
}
