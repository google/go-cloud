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

func TestPortableTypeDemos(t *testing.T) {
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

	// Call the main package run function as if 'add-ptype' were being called
	// from the command line for each of the portable types.
	for _, pt := range portableTypes {
		if err := run(ctx, pctx, []string{"add-ptype", pt.name}, new(bool)); err != nil {
			t.Fatalf("run add-ptype error: %+v", err)
		}
	}

	// Build the binary.
	exePath := filepath.Join(dir, "ptypedemotest")
	if err := buildForServe(ctx, pctx, dir, exePath); err != nil {
		t.Fatal("buildForServe(...):", err)
	}

	// Update the environment with to use local implementations for each
	// portable type.
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
		pt            string
		description   string
		urlPaths      []string
		op            string
		urlValues     url.Values // only used if op=POST
		wantStatus    int
		stringsToFind []string
	}{
		{
			pt:          "blob.Bucket",
			description: "base",
			urlPaths: []string{
				"/demo/blob.bucket",
				"/demo/blob.bucket/",
			},
			op:         "GET",
			wantStatus: http.StatusOK,
			stringsToFind: []string{
				"<title>blob.Bucket demo</title>",
				"This page demonstrates the use of a Go CDK blob.Bucket",
				`<a href="./list">List</a>`,
				`<a href="./view">View</a>`,
				`<a href="./write">Write</a>`,
			},
		},
		{
			pt:          "blob.Bucket",
			description: "list: empty",
			urlPaths:    []string{"/demo/blob.bucket/list"},
			op:          "GET",
			wantStatus:  http.StatusOK,
			stringsToFind: []string{
				"<title>blob.Bucket demo</title>",
				"no blobs in bucket",
			},
		},
		{
			pt:          "blob.Bucket",
			description: "view: missing key param",
			urlPaths:    []string{"/demo/blob.bucket/view"},
			op:          "GET",
			wantStatus:  http.StatusBadRequest,
		},
		// TODO(rvangent): Add tests as functionality is added, including:
		// -- /write shows a form
		// -- /write POST creates a blob (top level, plus one in subdir)
		// -- /list at the top level shows the subdirectory, with links
		// -- /list of the subdirectory shows the blob
		// -- /view missing key param shows a form with a dropdown
		// -- /view of the blob works
	}

	for _, test := range tests {
		t.Run(test.pt+":"+test.description, func(t *testing.T) {
			for _, urlPath := range test.urlPaths {
				u := alloc.url(urlPath).String()
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
				if resp.StatusCode != test.wantStatus {
					t.Fatalf("HTTP request returned status code %v, want %v", resp.StatusCode, test.wantStatus)
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
