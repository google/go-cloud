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

// TODO(rvangent): This file is user-visible, add many comments explaining
// how it works.

func init() {
	http.HandleFunc("/demo/blob/", blobBaseHandler)
	http.HandleFunc("/demo/blob/list", blobListHandler)
	http.HandleFunc("/demo/blob/view", blobViewHandler)
	http.HandleFunc("/demo/blob/write", blobWriteHandler)
}

var bucketURL string
var bucket *blob.Bucket
var bucketErr error

func init() {
	ctx := context.Background()

	bucketURL = os.Getenv("BLOB_BUCKET_URL")
	if bucketURL == "" {
		bucketURL = "mem://"
	}
	bucket, bucketErr = blob.OpenBucket(ctx, bucketURL)
}

// TODO(rvangent): This is pretty raw HTML. Should we have a common style sheet/etc. for demos?

type blobBaseData struct {
	URL string
	Err error
}

type blobListData struct {
	URL         string
	Err         error
	ListObjects []*blob.ListObject
}

type blobViewData struct {
	URL         string
	Err         error
	Key         string
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
    This page demonstrates the use of Go CDK's <a href="https://godoc.org/gocloud.dev/blob">blob</a> package.
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

	blobTemplateSuffix = `
</body>
</html>`

	// Input: *blobBaseData.
	blobBaseTemplate = blobTemplatePrefix + blobTemplateSuffix

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

	// Input: *blobViewData.
	blobViewTemplate = blobTemplatePrefix + `
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
  {{end}}` + blobTemplateSuffix

	// Input: *blobWriteData.
	blobWriteTemplate = blobTemplatePrefix + `
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
  {{end}}` + blobTemplateSuffix
)

var (
	blobBaseTmpl  = template.Must(template.New("blob base").Parse(blobBaseTemplate))
	blobListTmpl  = template.Must(template.New("blob list").Parse(blobListTemplate))
	blobViewTmpl  = template.Must(template.New("blob view").Parse(blobViewTemplate))
	blobWriteTmpl = template.Must(template.New("blob write").Parse(blobWriteTemplate))
)

func blobBaseHandler(w http.ResponseWriter, req *http.Request) {
	data := &blobBaseData{URL: bucketURL}
	if err := blobBaseTmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// blobListHandler lists the items in a bucket, possibly under a "prefix"
// query parameter. Each listed directory is a link to list that directory,
// and each non-directory is a link to view that file.
func blobListHandler(w http.ResponseWriter, req *http.Request) {
	data := &blobListData{URL: bucketURL}
	defer func() {
		if err := blobListTmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	if bucketErr != nil {
		data.Err = bucketErr
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
			data.Err = fmt.Errorf("Failed to iterate to next blob.Bucket key: %v", err)
			return
		}
		data.ListObjects = append(data.ListObjects, obj)
	}
	if len(data.ListObjects) == 0 {
		data.Err = errors.New("no blobs in bucket")
	}
}

func blobViewHandler(w http.ResponseWriter, req *http.Request) {
	data := &blobViewData{
		URL: bucketURL,
		Key: req.FormValue("key"),
	}
	skipTemplate := false
	defer func() {
		if !skipTemplate {
			if err := blobViewTmpl.Execute(w, data); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
	}()

	if bucketErr != nil {
		data.Err = bucketErr
		return
	}
	if data.Key == "" {
		// No key selected. Render a form with a dropdown to choose one.
		iter := bucket.List(nil)
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
	} else {
		// A key was provided. Download the blob for that key.
		skipTemplate = true
		reader, err := bucket.NewReader(req.Context(), data.Key, nil)
		if err != nil {
			data.Err = fmt.Errorf("failed to create Reader: %v", err)
			return
		}
		defer reader.Close()
		io.Copy(w, reader)
		// TODO(rvangent): Consider setting Content-Type, Content-Length headers.
	}
}

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

	if bucketErr != nil {
		data.Err = bucketErr
		return
	}
	if data.Key == "" && data.WriteContents == "" {
		return
	}
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
