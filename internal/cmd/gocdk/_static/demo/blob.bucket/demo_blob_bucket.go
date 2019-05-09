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
var bucketErr error

func init() {
	// TODO(rvangent): BLOB_BUCKET_URL should be in each biome's config such
	// that it can be read here.
	bucketURL = os.Getenv("BLOB_BUCKET_URL")
	if bucketURL == "" {
		bucketURL = "mem://"
	}
	bucket, bucketErr = blob.OpenBucket(context.Background(), bucketURL)
}

// TODO(rvangent): This is pretty raw HTML. Should we have a common style sheet/etc. for demos?

type blobBucketBaseData struct {
	URL string
	Err error
}

type blobBucketListData struct {
	URL         string
	Err         error
	ListObjects []*blob.ListObject
}

type blobBucketViewData struct {
	URL         string
	Err         error
	Key         string
	ListObjects []*blob.ListObject
}

type blobBucketWriteData struct {
	URL           string
	Err           error
	Key           string
	WriteContents string
	WriteSuccess  bool
}

const (
	blobBucketTemplatePrefix = `
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
    It is currently using a blob.Bucket based on the URL "{{ .URL }}", which
    can be configured via the environment variable "BLOB_BUCKET_URL".
  </p>
  <p>
    See <a href="https://gocloud.dev/concepts/urls/">here</a> for more
    information about URLs in Go CDK APIs.
  </p>
  <ul>
    <li><a href="./list">List</a> the contents of the bucket</li>
    <li><a href="./view">View</a> the contents of a specific blob in the bucket</li>
    <li><a href="./write">Write</a> a new blob into the bucket</li>
</ul>
{{if .Err}}
  <p><strong>{{.Err}}</strong></p>
{{end}}`

	blobBucketTemplateSuffix = `
</body>
</html>`

	// Input: *blobBucketBaseData.
	blobBucketBaseTemplate = blobBucketTemplatePrefix + blobBucketTemplateSuffix

	// Input: *blobBucketListData.
	blobBucketListTemplate = blobBucketTemplatePrefix + `
  {{range .ListObjects}}
    <div>
      {{if .IsDir}}
        <a href="./list?prefix={{ .Key }}">{{ .Key }}</a>
      {{else}}
        <a href="./view?key={{ .Key }}">{{ .Key }}</a>
      {{end}}
    </div>
  {{end}}` + blobBucketTemplateSuffix

	// Input: *blobBucketViewData.
	blobBucketViewTemplate = blobBucketTemplatePrefix + `
  {{if .ListObjects}}
    <form>
    <p><label>
      Choose a blob to view:
      <select name="key">
        {{range .ListObjects}}
          <option value="{{.Key}}">{{.Key}}</option>
        {{end}}
      </select>
    </label></p>
    <input type="submit">
    </form>
  {{end}}` + blobBucketTemplateSuffix

	// Input: *blobBucketWriteData.
	blobBucketWriteTemplate = blobBucketTemplatePrefix + `
  {{if .WriteSuccess}}
    Wrote it!
  {{else}}
    <form>
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
  {{end}}` + blobBucketTemplateSuffix
)

var (
	blobBucketBaseTmpl  = template.Must(template.New("blob.Bucket base").Parse(blobBucketBaseTemplate))
	blobBucketListTmpl  = template.Must(template.New("blob.Bucket list").Parse(blobBucketListTemplate))
	blobBucketViewTmpl  = template.Must(template.New("blob.Bucket view").Parse(blobBucketViewTemplate))
	blobBucketWriteTmpl = template.Must(template.New("blob.Bucket write").Parse(blobBucketWriteTemplate))
)

func blobBucketBaseHandler(w http.ResponseWriter, req *http.Request) {
	input := &blobBucketBaseData{URL: bucketURL}
	if err := blobBucketBaseTmpl.Execute(w, input); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// blobBucketListHandler lists the items in a bucket, possibly under a "prefix"
// query parameter. Each listed directory is a link to list that directory,
// and each non-directory is a link to view that file.
func blobBucketListHandler(w http.ResponseWriter, req *http.Request) {
	input := &blobBucketListData{URL: bucketURL}
	defer func() {
		if err := blobBucketListTmpl.Execute(w, input); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	if bucketErr != nil {
		input.Err = bucketErr
		return
	}
	opts := &blob.ListOptions{
		Delimiter: "/",
		Prefix:    req.FormValue("prefix"),
	}
	iter := bucket.List(opts)
	for {
		obj, err := iter.Next(req.Context())
		if err == io.EOF {
			break
		}
		if err != nil {
			input.Err = fmt.Errorf("Failed to iterate to next blob.Bucket key: %v", err)
			return
		}
		input.ListObjects = append(input.ListObjects, obj)
	}
	if len(input.ListObjects) == 0 {
		input.Err = errors.New("no blobs in bucket")
	}
}

func blobBucketViewHandler(w http.ResponseWriter, req *http.Request) {
	input := &blobBucketViewData{
		URL: bucketURL,
		Key: req.FormValue("key"),
	}
	skipTemplate := false
	defer func() {
		if !skipTemplate {
			if err := blobBucketViewTmpl.Execute(w, input); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
	}()

	if bucketErr != nil {
		input.Err = bucketErr
		return
	}
	if input.Key == "" {
		// No key selected. Render a form with a dropdown to choose one.
		iter := bucket.List(nil)
		for {
			obj, err := iter.Next(req.Context())
			if err == io.EOF {
				break
			}
			if err != nil {
				input.Err = fmt.Errorf("failed to iterate to next blob.Bucket key: %v", err)
				return
			}
			input.ListObjects = append(input.ListObjects, obj)
		}
		if len(input.ListObjects) == 0 {
			input.Err = errors.New("no blobs in bucket")
		}
	} else {
		// A key was provided. Download the blob for that key.
		skipTemplate = true
		reader, err := bucket.NewReader(req.Context(), input.Key, nil)
		if err != nil {
			input.Err = fmt.Errorf("failed to create Reader: %v", err)
			return
		}
		defer reader.Close()
		io.Copy(w, reader)
		// TODO(rvangent): Consider setting Content-Type, Content-Length headers.
	}
}

func blobBucketWriteHandler(w http.ResponseWriter, req *http.Request) {
	input := &blobBucketWriteData{
		URL:           bucketURL,
		Key:           req.FormValue("key"),
		WriteContents: req.FormValue("contents"),
	}
	defer func() {
		if err := blobBucketWriteTmpl.Execute(w, input); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	if bucketErr != nil {
		input.Err = bucketErr
		return
	}
	if input.Key == "" && input.WriteContents == "" {
		return
	}
	if input.Key == "" {
		input.Err = errors.New("enter a non-empty key to write to")
		return
	}
	if input.WriteContents == "" {
		input.Err = errors.New("enter some content to write")
		return
	}
	if err := bucket.WriteAll(req.Context(), input.Key, []byte(input.WriteContents), nil); err != nil {
		input.Err = fmt.Errorf("write failed: %v", err)
	} else {
		input.WriteSuccess = true
	}
}
