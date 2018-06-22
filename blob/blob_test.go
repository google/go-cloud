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

func TestNewWriter(t *testing.T) {
	tests := []struct {
		name, passContentType, wantContentType string
		wantErr                                bool
	}{
		{
			name:            "ParseContentType",
			passContentType: `FORM-DATA;name="foo"`,
			wantContentType: `form-data; name=foo`,
		},
		{
			name:            "EmptyContentType",
			wantContentType: "application/octet-stream",
		},
		{
			name:            "InvalidContentType",
			passContentType: "application/octet/stream",
			wantErr:         true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spy := new(bucketSpy)
			b := NewBucket(spy)
			ctx := context.Background()
			opt := &WriterOptions{
				ContentType: tc.passContentType,
			}
			_, err := b.NewWriter(ctx, "foo", opt)
			if tc.wantErr && err == nil {
				t.Error("b.NewWriter: want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("b.NewWriter: want nil error, got %v", err)
			}
			if spy.writeContentType != tc.wantContentType {
				t.Errorf("b.NewWriter: got Content-Type %v, want %v", spy.writeContentType, tc.wantContentType)
			}
		})
	}
}

type bucketSpy struct {
	key    string
	offset int64
	length int64

	writeContentType string

	readCalled bool
}

func (b *bucketSpy) NewRangeReader(ctx context.Context, key string, offset, length int64) (driver.Reader, error) {
	b.readCalled = true
	b.key = key
	b.offset = offset
	b.length = length
	return readerStub{}, nil
}

func (b *bucketSpy) NewWriter(ctx context.Context, key string, contentType string, opt *driver.WriterOptions) (driver.Writer, error) {
	b.writeContentType = contentType
	return writerStub{}, nil
}

func (b *bucketSpy) Delete(context.Context, string) error {
	return errors.New("unimplemented")
}

type readerStub struct{}

func (readerStub) Read([]byte) (int, error) {
	return 0, errors.New("unimplemented")
}

func (readerStub) Attrs() *driver.ObjectAttrs {
	return nil
}

func (readerStub) Close() error {
	return errors.New("unimplemented")
}

type writerStub struct{}

func (writerStub) Write([]byte) (n int, err error) {
	panic("unimplemented")
}

func (writerStub) Close() error {
	panic("unimplemented")
}
