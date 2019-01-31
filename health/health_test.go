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

package health

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestNewHandler(t *testing.T) {
	s := httptest.NewServer(new(Handler))
	defer s.Close()
	code, err := check(s)
	if err != nil {
		t.Fatalf("GET %s: %v", s.URL, err)
	}
	if code != http.StatusOK {
		t.Errorf("got HTTP status %d; want %d", code, http.StatusOK)
	}
}

func TestChecker(t *testing.T) {
	c1 := &checker{err: errors.New("checker 1 down")}
	c2 := &checker{err: errors.New("checker 2 down")}
	h := new(Handler)
	h.Add(c1)
	h.Add(c2)
	s := httptest.NewServer(h)
	defer s.Close()

	t.Run("AllUnhealthy", func(t *testing.T) {
		code, err := check(s)
		if err != nil {
			t.Fatalf("GET %s: %v", s.URL, err)
		}
		if code != http.StatusInternalServerError {
			t.Errorf("got HTTP status %d; want %d", code, http.StatusInternalServerError)
		}
	})
	c1.set(nil)
	t.Run("PartialHealthy", func(t *testing.T) {
		code, err := check(s)
		if err != nil {
			t.Fatalf("GET %s: %v", s.URL, err)
		}
		if code != http.StatusInternalServerError {
			t.Errorf("got HTTP status %d; want %d", code, http.StatusInternalServerError)
		}
	})
	c2.set(nil)
	t.Run("AllHealthy", func(t *testing.T) {
		code, err := check(s)
		if err != nil {
			t.Fatalf("GET %s: %v", s.URL, err)
		}
		if code != http.StatusOK {
			t.Errorf("got HTTP status %d; want %d", code, http.StatusOK)
		}
	})
}

func check(s *httptest.Server) (code int, err error) {
	resp, err := http.Get(s.URL)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

type checker struct {
	mu  sync.Mutex
	err error
}

func (c *checker) CheckHealth() error {
	defer c.mu.Unlock()
	c.mu.Lock()
	return c.err
}

func (c *checker) set(e error) {
	defer c.mu.Unlock()
	c.mu.Lock()
	c.err = e
}
