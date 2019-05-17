// Copyright 2018 The Go Cloud Development Kit Authors
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

package constantvar

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
)

type harness struct {
	// vars stores the variable value(s) that have been set using CreateVariable.
	vars map[string][]byte
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	return &harness{vars: map[string][]byte{}}, nil
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	rawVal, found := h.vars[name]
	if !found {
		// The variable isn't set. Create a Variable that always returns an error.
		return &watcher{err: errNotExist}, nil
	}
	val, err := decoder.Decode(ctx, rawVal)
	if err != nil {
		// The variable didn't decode.
		return &watcher{err: err}, nil
	}
	return &watcher{value: val, t: time.Now()}, nil
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	h.vars[name] = val
	return nil
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	return errors.New("not supported")
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	return errors.New("not supported")
}

func (h *harness) Close() {}

func (h *harness) Mutable() bool { return false }

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (verifyAs) Name() string {
	return "verify As"
}

func (verifyAs) SnapshotCheck(s *runtimevar.Snapshot) error {
	var ss string
	if s.As(&ss) {
		return errors.New("Snapshot.As expected to fail")
	}
	return nil
}

func (verifyAs) ErrorCheck(v *runtimevar.Variable, err error) error {
	var ss string
	if v.ErrorAs(err, &ss) {
		return errors.New("runtimevar.ErrorAs expected to fail")
	}
	return nil
}

func TestNew(t *testing.T) {
	ctx := context.Background()

	// Use New with an error value; it should be plumbed through as a Value.
	errFail := errors.New("fail")
	v := New(errFail)
	defer v.Close()
	val, err := v.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val.Value != errFail {
		t.Errorf("got %v want %v", val.Value, errFail)
	}
}

func TestNewBytes(t *testing.T) {
	ctx := context.Background()
	content := "hello world"

	// Decode succeeds.
	v := NewBytes([]byte(content), runtimevar.StringDecoder)
	defer v.Close()
	val, err := v.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if val.Value != content {
		t.Errorf("got %v want %v", val.Value, content)
	}

	// Decode fails.
	var jsonData []string
	v = NewBytes([]byte(content), runtimevar.NewDecoder(jsonData, runtimevar.JSONDecode))
	defer v.Close()
	val, err = v.Watch(ctx)
	if err == nil {
		t.Errorf("got nil error and %v, want error", val)
	}
}

func TestNewError(t *testing.T) {
	ctx := context.Background()

	v := NewError(errors.New("fail"))
	defer v.Close()
	_, err := v.Watch(ctx)
	if err == nil {
		t.Errorf("got nil err want fail err")
	}
}

func TestOpenVariable(t *testing.T) {
	tests := []struct {
		URL          string
		WantErr      bool
		WantWatchErr bool
		Want         interface{}
	}{
		// Empty URL results in empty byte slice.
		{"constant://", false, false, []byte("")},
		// Invalid query param.
		{"constant://?param=value", true, false, nil},
		// String value.
		{"constant://?val=hello+world&decoder=string", false, false, "hello world"},
		// JSON value; val parameter is {"Foo": "Bar"}, URL-encoded.
		{"constant://?val=%7B%22Foo%22%3A%22Bar%22%7d&decoder=jsonmap", false, false, &map[string]interface{}{"Foo": "Bar"}},
		// Error.
		{"constant://?err=fail", false, true, nil},
		// Invalid decoder.
		{"constant://?decoder=notadecoder", true, false, nil},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.URL, func(t *testing.T) {
			v, err := runtimevar.OpenVariable(ctx, test.URL)
			if (err != nil) != test.WantErr {
				t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
			}
			if err != nil {
				return
			}
			defer v.Close()
			snapshot, err := v.Watch(ctx)
			if (err != nil) != test.WantWatchErr {
				t.Errorf("%s: got Watch error %v, want error %v", test.URL, err, test.WantWatchErr)
			}
			if err != nil {
				return
			}
			if !cmp.Equal(snapshot.Value, test.Want) {
				t.Errorf("%s: got snapshot value\n%v\n  want\n%v", test.URL, snapshot.Value, test.Want)
			}
		})
	}
}

func TestDecryptWithNoURLEnv(t *testing.T) {
	if _, err := runtimevar.OpenVariable(context.Background(), "constant://?decoder=decrypt"); err == nil {
		t.Error("got nil error, want environment variable not set")
	}
}
