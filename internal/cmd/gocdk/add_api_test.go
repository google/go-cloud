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
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortableAPIDemos(t *testing.T) {
	dir, cleanup, err := newTestModule()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	pctx := &processContext{
		workdir: dir,
		env:     os.Environ(),
		stdout:  ioutil.Discard,
		stderr:  ioutil.Discard,
	}
	ctx := context.Background()

	if err := run(ctx, pctx, []string{"init", "-m", "test", "--allow-existing-dir", dir}, new(bool)); err != nil {
		t.Fatalf("run init error: %+v", err)
	}

	// Call the main package run function as if 'add-api' were being called
	// from the command line for each of the portable APIs.
	for _, api := range portableAPIs {
		if err := run(ctx, pctx, []string{"add-api", api.name}, new(bool)); err != nil {
			t.Fatalf("run add-api error: %+v", err)
		}
	}

	// Build the binary.
	exePath := filepath.Join(dir, "add-api-test")
	if err := buildForServe(ctx, pctx, dir, exePath); err != nil {
		t.Fatal("buildForServe(...):", err)
	}

	// Update the environment with to use local implementations for each
	// portable API.
	pctx.env = pctx.overrideEnv(
		"BLOB_BUCKET_URL=mem://",
	)

	// Run the program, listening on a free port.
	logger := log.New(pctx.stderr, "gocdk demo server under test: ", log.Ldate|log.Ltime)
	alloc := &serverAlloc{exePath: exePath, port: findFreePort()}
	cmd, err := alloc.start(ctx, pctx, logger, pctx.workdir)
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer endServerProcess(cmd)

	tests := []struct {
		api           string
		description   string
		urlPath       string
		op            string
		urlValues     url.Values // only used if op=POST
		stringsToFind []string
	}{
		{
			api:         "blob",
			description: "base no slash",
			urlPath:     "/demo/blob",
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"This page demonstrates the use",
				"https://godoc.org/gocloud.dev/blob",
				`<a href="./list">List</a>`,
				`<a href="./view">View</a>`,
				`<a href="./write">Write</a>`,
			},
		},
		{
			api:         "blob",
			description: "base with slash",
			urlPath:     "/demo/blob/",
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"This page demonstrates the use",
				"https://godoc.org/gocloud.dev/blob",
				`<a href="./list">List</a>`,
				`<a href="./view">View</a>`,
				`<a href="./write">Write</a>`,
			},
		},
		{
			api:         "blob",
			description: "list: empty",
			urlPath:     "/demo/blob/list",
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"no blobs in bucket",
			},
		},
		{
			api:         "blob",
			description: "view: empty",
			urlPath:     "/demo/blob/list",
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"no blobs in bucket",
			},
		},
		{
			api:         "blob",
			description: "write: empty form",
			urlPath:     "/demo/blob/write",
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				`<input type="submit" value="Write It!">`, // form is shown
			},
		},
		{
			api:         "blob",
			description: "write: missing key",
			urlPath:     "/demo/blob/write",
			op:          "POST",
			urlValues:   map[string][]string{"contents": {"foo"}},
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"<strong>enter a non-empty key to write to</strong>",
				"foo", // previous entry for contents field is carried over
				`<input type="submit" value="Write It!">`, // form is shown
			},
		},
		{
			api:         "blob",
			description: "write: missing contents",
			urlPath:     "/demo/blob/write",
			op:          "POST",
			urlValues:   map[string][]string{"key": {"key1"}},
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"<strong>enter some content to write</strong>",
				"key1", // previous entry for key field is carried over
				`<input type="submit" value="Write It!">`, // form is shown
			},
		},
		{
			api:         "blob",
			description: "write: top level key",
			urlPath:     "/demo/blob/write",
			op:          "POST",
			urlValues:   map[string][]string{"key": {"key1"}, "contents": {"key1 contents"}},
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"Wrote it!",
			},
		},
		{
			api:         "blob",
			description: "write: subdirectory key",
			urlPath:     "/demo/blob/write",
			op:          "POST",
			urlValues:   map[string][]string{"key": {"subdir/key2"}, "contents": {"key2 contents"}},
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"Wrote it!",
			},
		},
		{
			api:         "blob",
			description: "list: no longer empty",
			urlPath:     "/demo/blob/list",
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				`<a href="./view?key=key1">key1</a>`,
				`<a href="./list?prefix=subdir%2f">subdir/</a>`,
			},
		},
		/*
			TODO(rvangent): Enable listing of a subdir; broken right now because
			serverAlloc.url doesn't handle query parameters correctly.
			{
				api:          "blob",
				description: "list: subdir",
				urlPath:    "/demo/blob/list?prefix=subdir%2f",
				op:          "GET",
				stringsToFind: []string{
					"<title>gocloud.dev/blob demo</title>",
					`<a href="./view?key=subdir%2fkey2">key2</a>`,
				},
			},
		*/
		// TODO(rvangent): Add tests for the view page.
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s (%s, %s)", test.api, test.description, test.urlPath), func(t *testing.T) {
			u := alloc.url(test.urlPath).String()
			var resp *http.Response
			var err error
			switch test.op {
			case "GET":
				resp, err = http.DefaultClient.Get(u)
			case "POST":
				resp, err = http.DefaultClient.PostForm(u, test.urlValues)
			default:
				t.Fatalf("invalid test.op: %q", test.op)
			}
			if err != nil {
				t.Fatalf("HTTP %q request failed: %v", test.op, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("HTTP request returned status code %v, want %v", resp.StatusCode, http.StatusOK)
			}
			bodyBytes, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("failed to read HTTP response body: %v", err)
			}
			body := string(bodyBytes)
			logBody := false // only log the body on failure, and only once per test
			for _, s := range test.stringsToFind {
				if !strings.Contains(body, s) {
					t.Errorf("didn't find %q in HTTP response body", s)
					logBody = true
				}
			}
			if logBody {
				t.Error("Full HTTP response body:\n\n", body)
			}
		})
	}
}

func findFreePort() int {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatalf("failed to Listen to localhost:0; no free ports?: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
