// This file demonstrates basic usage of the docstore.Collection portable type.
//
// It initializes a docstore.Collection URL based on the environment variable
// DOCSTORE_COLLECTION_URL, and then registers handlers for "/demo/docstore" on
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

	"gocloud.dev/docstore"
	_ "gocloud.dev/docstore/awsdynamodb"
	_ "gocloud.dev/docstore/gcpfirestore"
	_ "gocloud.dev/docstore/memdocstore"
	_ "gocloud.dev/docstore/mongodocstore"
)

// Package variables for the docstore.Collection URL, and the initialized docstore.Collection.
var (
	collectionURL string
	collection    *docstore.Collection
	collectionErr error
)

func init() {
	// Register handlers. See https://golang.org/pkg/net/http/.
	http.HandleFunc("/demo/docstore/", docstoreBaseHandler)
	http.HandleFunc("/demo/docstore/list", docstoreListHandler)
	http.HandleFunc("/demo/docstore/edit", docstoreEditHandler)

	// Initialize the docstore.Collection using a URL from the environment,
	// defaulting to an in-memory implementation. Note that the in-memory
	// docstore starts out empty every time you run the application!
	collectionURL = os.Getenv("DOCSTORE_COLLECTION_URL")
	if collectionURL == "" {
		collectionURL = "mem://mycollection/Key"
	}
	collection, collectionErr = docstore.OpenCollection(context.Background(), collectionURL)
}

// MyDocument holds the data for each document in the docstore collection.
type MyDocument struct {
	Key              string
	Value            string
	DocstoreRevision interface{}
}

// The structs below hold the input to each of the pages in the demo.
// Each page handler will initialize one of the structs and pass it to
// one of the templates.

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
    This page demonstrates the use of Go CDK's <a href="https://gocloud.dev/howto/docstore">docstore</a> package.
  </p>
  <p>
    It is currently using a docstore.Collection based on the URL "{{ .URL }}", which
    can be configured via the environment variable "DOCSTORE_COLLECTION_URL".
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

	// docstoreBaseTemplate is the template for /demo/docstore. See docstoreBaseHandler.
	// Input: *docstoreBaseData.
	docstoreBaseTemplate = docstoreTemplatePrefix + docstoreTemplateSuffix

	// docstoreListTemplate is the template for /demo/docstore/list. See docstoreListHandler.
	// Input: *docstoreListData.
	docstoreListTemplate = docstoreTemplatePrefix + `
  {{range .Keys}}
    <div>
      <a href="./edit?key={{ . }}">{{ . }}</a>
    </div>
  {{end}}` + docstoreTemplateSuffix

	// docstoreEditTemplate is the template for /demo/docstore/edit. See docstoreEditHandler.
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
    <input type="submit" value="Write It!">
    </form>
  {{end}}` + docstoreTemplateSuffix
)

var (
	docstoreBaseTmpl = template.Must(template.New("docstore base").Parse(docstoreBaseTemplate))
	docstoreListTmpl = template.Must(template.New("docstore list").Parse(docstoreListTemplate))
	docstoreEditTmpl = template.Must(template.New("docstore edit").Parse(docstoreEditTemplate))
)

// docstoreBaseHandler is the handler for /demo/docstore.
func docstoreBaseHandler(w http.ResponseWriter, req *http.Request) {
	data := &docstoreBaseData{URL: collectionURL}
	if err := docstoreBaseTmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// docstoreListHandler is the handler for /demo/docstore/list.
//
// It lists the keys for documents in the collection.
// Each item is shown as a link to edit that document.
func docstoreListHandler(w http.ResponseWriter, req *http.Request) {
	data := &docstoreListData{URL: collectionURL}
	defer func() {
		if err := docstoreListTmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

	// Verify that collection initialization succeeded.
	if collectionErr != nil {
		data.Err = collectionErr
		return
	}

	// Iterate over documents in the collection, only fetching the "Key" field.
	// The demo uses an arbitrary limit of 1000 documents; docstore supports
	// a variety of query filters.
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

// docstoreEditHandler is the handler for /demo/docstore/edit.
//
// It renders a form to create a new document, or to edit an existing one.
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

	// Verify that collection initialization succeeded.
	if collectionErr != nil {
		data.Err = collectionErr
		return
	}

	if req.Method == http.MethodGet {
		// Prepare the edit form.
		// For create, it will be blank.
		// For edit, load the existing document.
		if data.Create {
			return
		}
		if data.Key == "" {
			data.Err = errors.New("key must be provided to edit")
			return
		}
		// Fetch the full document at the given Key.
		doc := MyDocument{Key: data.Key}
		err := collection.Get(req.Context(), &doc)
		if err != nil {
			data.Err = fmt.Errorf("failed to get document: %v", err)
			return
		}
		data.Value = doc.Value
		return
	}

	// POST.
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
		if err := collection.Replace(req.Context(), &doc); err != nil {
			data.Err = fmt.Errorf("document replace failed: %v", err)
			return
		}
	}
	data.WriteSuccess = true
}
