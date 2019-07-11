// This file demonstrates basic usage of the blob.Bucket portable type.
//
// It initializes a blob.Bucket URL based on the environment variable
// BLOB_BUCKET_URL, and then registers handlers for "/demo/blob" on
// http.DefaultServeMux.

package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/memblob"
	_ "gocloud.dev/blob/s3blob"
)

// Package variables for the blob.Bucket URL, and the initialized blob.Bucket.
var (
	bucketURL string
	bucket    *blob.Bucket
	bucketErr error
)

func init() {
	// Register handlers. See https://golang.org/pkg/net/http/.
	http.HandleFunc("/demo/blob/", blobBaseHandler)
	http.HandleFunc("/demo/blob/list", blobListHandler)
	http.HandleFunc("/demo/blob/view", blobViewHandler)
	http.HandleFunc("/demo/blob/write", blobWriteHandler)

	// Initialize the blob.Bucket using a URL from the environment, defaulting
	// to an in-memory bucket provider. Note that the in-memory bucket provider
	// starts out empty every time you run the application!
	bucketURL = os.Getenv("BLOB_BUCKET_URL")
	if bucketURL == "" {
		bucketURL = "mem://"
	}
	bucket, bucketErr = blob.OpenBucket(context.Background(), bucketURL)
}

// The structs below hold the input to each of the pages in the demo.
// Each page handler will initialize one of the structs and pass it to
// one of the templates.

type blobBaseData struct {
	URL string
	Err error
}

type blobListData struct {
	URL         string
	Err         error
	ListObjects []*blob.ListObject
}

type blobWriteData struct {
	URL           string
	Err           error
	Key           string
	WriteContents string
	WriteSuccess  bool
}

const (
	blobTemplatePrefix = `
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>gocloud.dev/blob demo</title>
</head>
<body>
  <p>
    This page demonstrates the use of Go CDK's <a href="https://gocloud.dev/howto/blob/">blob</a> package.
  </p>
  <p>
    It is currently using a blob.Bucket based on the URL "{{ .URL }}", which
    can be configured via the environment variable "BLOB_BUCKET_URL".
  </p>
  <ul>
    <li><a href="./list">List</a> the contents of the bucket</li>
    <li><a href="./write">Write</a> a new blob into the bucket</li>
</ul>
{{if .Err}}
  <p><strong>{{.Err}}</strong></p>
{{end}}`

	blobTemplateSuffix = `
</body>
</html>`

	// blobBaseTemplate is the template for /demo. See blobBaseHandler.
	// Input: *blobBaseData.
	blobBaseTemplate = blobTemplatePrefix + blobTemplateSuffix

	// blobListTemplate is the template for /demo/list. See blobListHandler.
	// Input: *blobListData.
	blobListTemplate = blobTemplatePrefix + `
  {{range .ListObjects}}
    <div>
      {{if .IsDir}}
        <a href="./list?prefix={{ .Key }}">{{ .Key }}</a>
      {{else}}
        <a href="./view?key={{ .Key }}">{{ .Key }}</a>
      {{end}}
    </div>
  {{end}}` + blobTemplateSuffix

	// blobWriteTemplate is the template for /demo/write. See blobWriteHandler.
	// Input: *blobWriteData.
	blobWriteTemplate = blobTemplatePrefix + `
  {{if .WriteSuccess}}
    Wrote it!
  {{else}}
    <form method="POST">
    <p><label>
      Blob key to write to (any previous blob will be overwritten):
      <br/>
      <input type="text" name="key" value="{{ .Key }}">
    </label></p>
    <p><label>
      Data to write:
      <br/>
      <textarea name="contents" rows="4" cols="40">{{ .WriteContents }}</textarea>
    </label></p>
    <input type="submit" value="Write It!">
    </form>
  {{end}}` + blobTemplateSuffix
)

var (
	blobBaseTmpl  = template.Must(template.New("blob base").Parse(blobBaseTemplate))
	blobListTmpl  = template.Must(template.New("blob list").Parse(blobListTemplate))
	blobWriteTmpl = template.Must(template.New("blob write").Parse(blobWriteTemplate))
)

// blobBaseHandler is the handler for /demo.
func blobBaseHandler(w http.ResponseWriter, req *http.Request) {
	data := &blobBaseData{URL: bucketURL}
	if err := blobBaseTmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// blobListHandler is the handler for /demo/list.
//
// It lists the keys in a bucket, possibly under a "prefix".
//
// Each key that represents a "directory" is rendered as a link to list the
// items in that subdirectory.
//
// Each key that represents a blob is rendered as a link to view the contents
// of the blob.
func blobListHandler(w http.ResponseWriter, req *http.Request) {
	data := &blobListData{URL: bucketURL}
	defer func() {
		if err := blobListTmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	// Verify that bucket initialization succeeded.
	if bucketErr != nil {
		data.Err = bucketErr
		return
	}
	// Use bucket.List to get an iterator over the keys in the bucket.
	//
	// Setting ListOptions.Delimiter to "/" means that blob keys with "/" in them
	// will be interpreted as "directories".
	//
	// Setting ListOptions.Prefix limits the results to keys with the prefix;
	// this can be used to list keys in a "directory".
	iter := bucket.List(&blob.ListOptions{Delimiter: "/", Prefix: req.FormValue("prefix")})
	for {
		obj, err := iter.Next(req.Context())
		if err == io.EOF {
			break
		}
		if err != nil {
			data.Err = fmt.Errorf("failed to iterate to next blob.Bucket key: %v", err)
			return
		}
		data.ListObjects = append(data.ListObjects, obj)
	}
	if len(data.ListObjects) == 0 {
		data.Err = errors.New("no blobs in bucket")
	}
}

// blobViewHandler is the handler for /demo/view.
//
// It expects a "key" query parameter, and renders the contents of the blob
// at that key.
func blobViewHandler(w http.ResponseWriter, req *http.Request) {
	// Render the base template if there's an error.
	data := &blobBaseData{URL: bucketURL}
	defer func() {
		if data.Err != nil {
			if err := blobBaseTmpl.Execute(w, data); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
	}()

	// Verify that bucket initialization succeeded.
	if bucketErr != nil {
		data.Err = bucketErr
		return
	}
	// Get a reader for the blob contents, and use io.Copy to write it to the
	// http.ResponseWriter.
	reader, err := bucket.NewReader(req.Context(), req.FormValue("key"), nil)
	if err != nil {
		data.Err = fmt.Errorf("failed to create Reader: %v", err)
		return
	}
	defer reader.Close()
	io.Copy(w, reader)
}

// blobWriteHandler is the handler for /demo/write.
func blobWriteHandler(w http.ResponseWriter, req *http.Request) {
	data := &blobWriteData{
		URL:           bucketURL,
		Key:           req.FormValue("key"),
		WriteContents: req.FormValue("contents"),
	}
	defer func() {
		if err := blobWriteTmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	// Verify that bucket initialization succeeded.
	if bucketErr != nil {
		data.Err = bucketErr
		return
	}

	// For GET, render the form.
	if req.Method == http.MethodGet {
		return
	}

	// POST.
	if data.Key == "" {
		data.Err = errors.New("enter a non-empty key to write to")
		return
	}
	if data.WriteContents == "" {
		data.Err = errors.New("enter some content to write")
		return
	}
	if err := bucket.WriteAll(req.Context(), data.Key, []byte(data.WriteContents), nil); err != nil {
		data.Err = fmt.Errorf("write failed: %v", err)
	} else {
		data.WriteSuccess = true
	}
}
