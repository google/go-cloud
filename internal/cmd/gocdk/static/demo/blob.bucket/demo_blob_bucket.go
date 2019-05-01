// Copyright 2019 The Go Cloud Authors
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
	"html/template"
	"io"
	"net/http"
	"os"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/memblob"
)

// TODO(rvangent): This file is user-visible, add many comments explaining
// how it works.

func init() {
	http.HandleFunc("/demo/blob.bucket/", blobBucketBaseHandler)
	http.HandleFunc("/demo/blob.bucket/list", blobBucketListHandler)
	http.HandleFunc("/demo/blob.bucket/view", blobBucketViewHandler)
	http.HandleFunc("/demo/blob.bucket/write", blobBucketWriteHandler)
}

var bucketURL string
var bucket *blob.Bucket
var bucketErr string

func init() {
	// TODO(rvangent): Would sync.Once be better than init()? Or recreate the
	// blob.Bucket on each request?

	// TODO(rvangent): BLOB_BUCKET_URL should be in each biome's config such
	// that it can be read here.
	var err error
	bucketURL = os.Getenv("BLOB_BUCKET_URL")
	if bucketURL == "" {
		// TODO(rvangent): This is a terrible default, remove once "write" is working.
		bucketURL = "file:///tmp"
	}
	bucket, err = blob.OpenBucket(context.Background(), bucketURL)
	if err != nil {
		bucketErr = fmt.Sprintf("Failed to open blob.Bucket: %v", err)
	}
}

// TODO(rvangent): This is pretty raw HTML. Should we have a common style sheet/etc. for demos?

// Input: (string) blob.Bucket URL.
const blobBucketBaseTemplate = `
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>blob.Bucket demo</title>
</head>
<body>
  <p>
    This page demonstrates the use of a Go CDK blob.Bucket.
  </p>
  <p>
    It is currently using a blob.Bucket based on the URL "{{ . }}", which
    can be configured via the environment variable "BLOB_BUCKET_URL".
  </p>
  <p>
    See <a href="https://godoc.org/gocloud.dev#hdr-URLs">here</a> for more
    information about URLs in Go CDK APIs.
  </p>
  <ul>
    <li><a href="./list">List</a> the contents of the bucket</li>
    <li><a href="./view">View</a> the contents of a specific blob in the bucket</li>
    <li><a href="./write">Write</a> a new blob into the bucket</li>
  </ul>
</body>
</html>
`

func blobBucketBaseHandler(w http.ResponseWriter, req *http.Request) {
	tmpl := template.Must(template.New("base").Parse(blobBucketBaseTemplate))
	err := tmpl.Execute(w, bucketURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Input: []*blob.ListObject.
const blobBucketListTemplate = `
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>blob.Bucket demo</title>
</head>
<body>
  {{range .}}
    <div>
      {{if .IsDir}}
        <a href="/demo/blob/list?prefix={{ .Key }}">{{ .Key }}</a>
      {{else}}
        <a href="/demo/blob/view?key={{ .Key }}">{{ .Key }}</a>
      {{end}}
    </div>
  {{else}}
    <div>no blobs in bucket</div>
  {{end}}
</body>
</html>
`

// blobBucketListHandler lists the items in a bucket, possibly under a "prefix"
// query parameter. Each listed directory is a link to list that directory,
// and each non-directory is a link to view that file.
func blobBucketListHandler(w http.ResponseWriter, req *http.Request) {
	if bucketErr != "" {
		http.Error(w, bucketErr, http.StatusInternalServerError)
		return
	}
	opts := &blob.ListOptions{
		Delimiter: "/",
		Prefix:    req.FormValue("prefix"),
	}
	iter := bucket.List(opts)
	ctx := req.Context()
	var items []*blob.ListObject
	for {
		item, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to iterate to next blob.Bucket key: %v", err), http.StatusInternalServerError)
			return
		}
		items = append(items, item)
	}

	tmpl := template.Must(template.New("list").Parse(blobBucketListTemplate))
	err := tmpl.Execute(w, items)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func blobBucketViewHandler(w http.ResponseWriter, req *http.Request) {
	if bucketErr != "" {
		http.Error(w, bucketErr, http.StatusInternalServerError)
		return
	}
	// TODO(rvangent): By default, render a form with key name drop-down instead of
	// erroring out.
	key := req.FormValue("key")
	if key == "" {
		http.Error(w, "\"key\" is a required parameter.", http.StatusBadRequest)
		return
	}

	ctx := req.Context()
	reader, err := bucket.NewReader(ctx, key, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create Reader: %v\n", err), http.StatusNotFound)
		return
	}
	defer reader.Close()
	io.Copy(w, reader)
}

func blobBucketWriteHandler(w http.ResponseWriter, req *http.Request) {
	// TODO(rvangent): By default, render a form with key name and blob contents.
	// When the form is POSTed, write the blob contents to the key.
	http.Error(w, "Coming soon!", http.StatusNotImplemented)
}
