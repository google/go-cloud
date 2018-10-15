// Copyright 2018 The Go Cloud Authors
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

package server

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestListenAndServe(t *testing.T) {
	td := new(testDriver)
	s := New(&Options{Driver: td})
	err := s.ListenAndServe(":8080", http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	if !td.listenAndServeCalled {
		t.Error("ListenAndServe was not called from the supplied driver")
	}
}

type testDriver struct {
	listenAndServeCalled bool
}

func (td *testDriver) ListenAndServe(addr string, h http.Handler) error {
	td.listenAndServeCalled = true
	return nil
}

func (td *testDriver) Shutdown(ctx context.Context) error {
	return errors.New("this is a method for satisfying the interface")
}
