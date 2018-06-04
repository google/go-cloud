// Copyright 2018 Google LLC
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
	"bytes"
	"context"
	"database/sql"
	"flag"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/health"
	"github.com/google/go-cloud/health/sqlhealth"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/server"
	"github.com/google/go-cloud/wire"
	"github.com/gorilla/mux"
	"go.opencensus.io/trace"
)

var envFlag string

func main() {
	// Determine environment to set up based on flag.
	flag.StringVar(&envFlag, "env", "local", "environment to run under")
	addr := flag.String("listen", ":8080", "port to listen for HTTP on")
	flag.Parse()

	ctx := context.Background()
	var app *app
	var cleanup func()
	var err error
	switch envFlag {
	case "gcp":
		app, cleanup, err = setupGCP(ctx)
	case "aws":
		app, cleanup, err = setupAWS(ctx)
	case "local":
		app, cleanup, err = setupLocal(ctx)
	default:
		log.Fatalf("unknown -env=%s", envFlag)
	}
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// Set up URL routes.
	r := mux.NewRouter()
	r.HandleFunc("/", app.index)
	r.HandleFunc("/sign", app.sign)
	r.HandleFunc("/blob/{key:.+}", app.serveBlob)

	// Listen and serve HTTP.
	log.Printf("Running, connected to %q cloud", envFlag)
	log.Fatal(app.srv.ListenAndServe(*addr, r))
}

// app is the main server struct for Guestbook.
type app struct {
	backends

	// The following fields are protected by mu:
	mu   sync.RWMutex
	motd string // message of the day
}

// backends is a set of platform-independent services that the Guestbook uses.
type backends struct {
	srv    *server.Server
	db     *sql.DB
	bucket *blob.Bucket
}

func newApp(b backends, v *runtimevar.Variable) *app {
	a := &app{backends: b}
	go a.watchMOTDVar(v)
	return a
}

// watchMOTDVar listens for changes in v and updates the app's message of the
// day. It is run in a separate goroutine.
func (app *app) watchMOTDVar(v *runtimevar.Variable) {
	ctx := context.Background()
	for {
		snap, err := v.Watch(ctx)
		if err != nil {
			log.Printf("watch MOTD variable: %v", err)
			continue
		}
		log.Println("updated MOTD to", snap.Value)
		app.mu.Lock()
		app.motd = snap.Value.(string)
		app.mu.Unlock()
	}
}

// index serves the server's landing page.
// It lists the 100 most recent greetings, shows a cloud environment banner, and
// displays the message of the day.
func (app *app) index(w http.ResponseWriter, r *http.Request) {
	var data struct {
		MOTD      string
		Env       string
		BannerSrc string
		Greetings []greeting
	}
	app.mu.RLock()
	data.MOTD = app.motd
	app.mu.RUnlock()
	switch envFlag {
	case "gcp":
		data.Env = "GCP"
		data.BannerSrc = "/blob/gcp.png"
	case "aws":
		data.Env = "AWS"
		data.BannerSrc = "/blob/aws.png"
	case "local":
		data.Env = "Local"
		data.BannerSrc = "/blob/gophers.jpg"
	}

	dbCtx, dbSpan := trace.StartSpan(r.Context(), "sampleapp/MySQL.SELECT")
	defer dbSpan.End()
	const query = "SELECT content FROM (SELECT content, post_date FROM greetings ORDER BY post_date DESC LIMIT 100) AS recent_greetings ORDER BY post_date ASC;"
	q, err := app.db.QueryContext(dbCtx, query)
	if err != nil {
		log.Println("main page SQL error:", err)
		http.Error(w, "could not load greetings", http.StatusInternalServerError)
		return
	}
	defer q.Close()
	for q.Next() {
		var g greeting
		if err := q.Scan(&g.Content); err != nil {
			log.Println("main page SQL error:", err)
			http.Error(w, "could not load greetings", http.StatusInternalServerError)
			return
		}
		data.Greetings = append(data.Greetings, g)
	}
	dbSpan.End()
	if err := q.Err(); err != nil {
		log.Println("main page SQL error:", err)
		http.Error(w, "could not load greetings", http.StatusInternalServerError)
		return
	}
	buf := new(bytes.Buffer)
	if err := tmpl.Execute(buf, data); err != nil {
		log.Println("template error:", err)
		http.Error(w, "could not render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Println("writing response:", err)
	}
}

type greeting struct {
	Content string
}

var tmpl = template.Must(template.New("index.html").Parse(`<!DOCTYPE html>
<title>Guestbook - {{.Env}}</title>
<style type="text/css">
html, body {
	font-family: Helvetica, sans-serif;
}
blockquote {
	font-family: cursive, Helvetica, sans-serif;
}
.banner {
	max-height: 7em;
}
.greeting {
	font-size: 85%;
}
.motd {
	font-weight: bold;
}
</style>
<h1>Guestbook</h1>
<div><img class="banner" src="{{.BannerSrc}}"></div>
{{with .MOTD}}<p class="motd">Admin says: {{.}}</p>{{end}}
{{range .Greetings}}
<div class="greeting">
	Someone wrote:
	<blockquote>{{.Content}}</blockquote>
</div>
{{end}}
<form action="/sign" method="POST">
	<div><textarea name="content" rows="3"></textarea></div>
	<div><input type="submit" value="Sign"></div>
</form>
`))

// sign is a form action handler for adding a greeting.
func (app *app) sign(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}
	content := r.FormValue("content")
	if content == "" {
		http.Error(w, "content must not be empty", http.StatusBadRequest)
		return
	}
	const sqlStmt = "INSERT INTO greetings (content) VALUES (?);"
	dbCtx, dbSpan := trace.StartSpan(r.Context(), "sampleapp/MySQL.INSERT")
	_, err := app.db.ExecContext(dbCtx, sqlStmt, content)
	dbSpan.End()
	if err != nil {
		log.Println("sign SQL error:", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// serveBlob handles a request for a static asset by retrieving it from a bucket.
func (app *app) serveBlob(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	blobRead, err := app.bucket.NewReader(r.Context(), key)
	if err != nil {
		// TODO(light): Distinguish 404.
		// https://github.com/google/go-cloud/issues/2
		log.Println("serve blob:", err)
		http.Error(w, "blob read error", http.StatusInternalServerError)
		return
	}
	// TODO(light): Get content type from blob storage.
	// https://github.com/google/go-cloud/issues/9
	switch {
	case strings.HasSuffix(key, ".png"):
		w.Header().Set("Content-Type", "image/png")
	case strings.HasSuffix(key, ".jpg"):
		w.Header().Set("Content-Type", "image/jpeg")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Length", strconv.FormatInt(blobRead.Size(), 10))
	io.Copy(w, blobRead)
}

// appSet is the Wire provider set for the Guestbook application that does not
// depend on the underlying platform.
var appSet = wire.NewSet(
	backends{},
	newApp,
	appHealthChecks,
	trace.AlwaysSample,
)

// appHealthChecks returns a health check for the database. This will signal
// to Kubernetes or other orchestrators that the server should not receive
// traffic until the server is able to connect to its database.
func appHealthChecks(db *sql.DB) ([]health.Checker, func()) {
	dbCheck := sqlhealth.New(db)
	list := []health.Checker{dbCheck}
	return list, func() {
		dbCheck.Stop()
	}
}
