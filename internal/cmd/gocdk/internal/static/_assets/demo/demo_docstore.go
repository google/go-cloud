package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"time"

	"gocloud.dev/docstore"
	_ "gocloud.dev/docstore/awsdynamodb"
	_ "gocloud.dev/docstore/gcpfirestore"
	_ "gocloud.dev/docstore/memdocstore"
	_ "gocloud.dev/docstore/mongodocstore"
)

// TODO(rvangent): This file is user-visible, add many comments explaining
// how it works.

func init() {
	http.HandleFunc("/demo/docstore/", docstoreBaseHandler)
	http.HandleFunc("/demo/docstore/list", docstoreListHandler)
	http.HandleFunc("/demo/docstore/edit", docstoreEditHandler)
}

var collectionURL string
var collection *docstore.Collection
var collectionErr error

func init() {
	collectionURL = os.Getenv("DOCSTORE_COLLECTION_URL")
	if collectionURL == "" {
		collectionURL = "mem://mycollection/Key"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	collection, collectionErr = docstore.OpenCollection(ctx, collectionURL)
}

type MyDocument struct {
	Key   string
	Value string
	// TODO(rvangent): Rename this field to demonstrate setting it once
	// https://github.com/google/go-cloud/issues/2413 is fixed.
	DocstoreRevision interface{}
}

// TODO(rvangent): This is pretty raw HTML. Should we have a common style sheet/etc. for demos?

type docstoreBaseData struct {
	URL string
	Err error
}

type docstoreListData struct {
	URL  string
	Err  error
	Keys []string
}

type docstoreEditData struct {
	URL          string
	Err          error
	Create       bool
	Key          string
	Value        string
	Revision     string
	WriteSuccess bool
}

const (
	docstoreTemplatePrefix = `
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>gocloud.dev/docstore demo</title>
</head>
<body>
  <p>
    This page demonstrates the use of Go CDK's <a href="https://godoc.org/gocloud.dev/docstore">docstore</a> package.
  </p>
  <p>
    It is currently using a docstore.Collection based on the URL "{{ .URL }}", which
    can be configured via the environment variable "DOCSTORE_COLLECTION_URL".
  </p>
  <p>
    See <a href="https://gocloud.dev/concepts/urls/">here</a> for more
    information about URLs in Go CDK APIs.
  </p>
  <ul>
    <li><a href="./list">List</a> the documents in the collection</li>
    <li><a href="./edit?create=true">Create</a> a new document</li>
</ul>
{{if .Err}}
  <p><strong>{{.Err}}</strong></p>
{{end}}`

	docstoreTemplateSuffix = `
</body>
</html>`

	// Input: *docstoreBaseData.
	docstoreBaseTemplate = docstoreTemplatePrefix + docstoreTemplateSuffix

	// Input: *docstoreListData.
	docstoreListTemplate = docstoreTemplatePrefix + `
  {{range .Keys}}
    <div>
      <a href="./edit?key={{ . }}">{{ . }}</a>
    </div>
  {{end}}` + docstoreTemplateSuffix

	// Input: *docstoreEditData.
	docstoreEditTemplate = docstoreTemplatePrefix + `
  {{if .WriteSuccess}}
    Write succeeded!
  {{else}}
    <form method="POST">
      <input type="hidden" name="create" value="{{ .Create }}">
    {{if .Create}}
      <p><label>
        Enter a unique key:
        <br/>
        <input type="text" name="key">
      </label></p>
    {{else}}
      <p>Update document with key <b>{{ .Key }}</b>:</p>
      <input type="hidden" name="key" value="{{ .Key }}">
    {{end}}
    <p><label>
      Value:
      <br/>
      <input type="text" name="value" value="{{ .Value }}">
    </label></p>
    <p>Revision: {{ .Revision }}</p>
    <input type="submit" value="Write It!">
    </form>
  {{end}}` + docstoreTemplateSuffix
)

var (
	docstoreBaseTmpl = template.Must(template.New("docstore base").Parse(docstoreBaseTemplate))
	docstoreListTmpl = template.Must(template.New("docstore list").Parse(docstoreListTemplate))
	docstoreEditTmpl = template.Must(template.New("docstore edit").Parse(docstoreEditTemplate))
)

func docstoreBaseHandler(w http.ResponseWriter, req *http.Request) {
	data := &docstoreBaseData{URL: collectionURL}
	if err := docstoreBaseTmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// docstoreListHandler lists the items in a collection, possibly under a "prefix"
// query parameter. Each listed directory is a link to list that directory,
// and each non-directory is a link to view that file.
func docstoreListHandler(w http.ResponseWriter, req *http.Request) {
	data := &docstoreListData{URL: collectionURL}
	defer func() {
		if err := docstoreListTmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	if collectionErr != nil {
		data.Err = collectionErr
		return
	}
	// TODO(rvangent): The 1000 limit is arbitrary. Give an error if it is
	// reached? Or drop it?
	// TODO(rvangent): Consider adding filters on Key to demonstrate queries
	// better. Add OrderBy?
	iter := collection.Query().Limit(1000).Get(req.Context(), "Key")
	defer iter.Stop()
	for {
		doc := MyDocument{}
		err := iter.Next(req.Context(), &doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			data.Err = fmt.Errorf("failed to iterate to next docstore.Collection key: %v", err)
			return
		}
		data.Keys = append(data.Keys, doc.Key)
	}
	if len(data.Keys) == 0 {
		data.Err = errors.New("no documents in collection")
	}
}

func docstoreEditHandler(w http.ResponseWriter, req *http.Request) {
	data := &docstoreEditData{
		URL:    collectionURL,
		Create: req.FormValue("create") == "true",
		Key:    req.FormValue("key"),
		Value:  req.FormValue("value"),
	}
	defer func() {
		if err := docstoreEditTmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	if collectionErr != nil {
		data.Err = collectionErr
		return
	}

	if req.Method != "POST" {
		// Just preparing the form. For create, nothing to do; for edit,
		// load the existing document.
		if data.Create {
			return
		}
		if data.Key == "" {
			data.Err = errors.New("key must be provided to edit")
			return
		}
		doc := MyDocument{Key: data.Key}
		err := collection.Get(req.Context(), &doc)
		if err != nil {
			data.Err = fmt.Errorf("failed to get document: %v", err)
			return
		}
		data.Value = doc.Value
		data.Revision = fmt.Sprintf("%v", doc.DocstoreRevision)
		return
	}

	// POST
	if data.Key == "" {
		data.Err = errors.New("enter a non-empty key")
		return
	}
	doc := MyDocument{Key: data.Key, Value: data.Value}
	if data.Create {
		// Creating a new document.
		if err := collection.Create(req.Context(), &doc); err != nil {
			data.Err = fmt.Errorf("document creation failed: %v", err)
			return
		}
	} else {
		// Updating an existing document.
		// TODO(rvangent): I am not sure why this works without a revision;
		// see https://github.com/google/go-cloud/issues/2417.
		if err := collection.Replace(req.Context(), &doc); err != nil {
			data.Err = fmt.Errorf("document put failed: %v", err)
			return
		}
	}
	data.WriteSuccess = true
}
