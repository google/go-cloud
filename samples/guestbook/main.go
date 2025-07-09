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

// guestbook is a sample application that records visitors' messages, displays a
// cloud banner, and an administrative message.
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
	"time"

	"github.com/google/wire"
	"github.com/gorilla/mux"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
	"gocloud.dev/runtimevar"

	"gocloud.dev/server"
	"gocloud.dev/server/health"
	"gocloud.dev/server/health/sqlhealth"
)

type cliFlags struct {
	bucket          string
	dbHost          string
	dbName          string
	dbUser          string
	dbPassword      string
	motdVar         string
	motdVarWaitTime time.Duration

	// GCP only.
	cloudSQLRegion    string
	runtimeConfigName string
}

var envFlag string

func main() {
	// Determine environment to set up based on flag.
	cf := new(cliFlags)
	flag.StringVar(&envFlag, "env", "local", "environment to run under (gcp, aws, azure, or local)")
	addr := flag.String("listen", ":8080", "port to listen for HTTP on")
	flag.StringVar(&cf.bucket, "bucket", "", "bucket name")
	flag.StringVar(&cf.dbHost, "db_host", "", "database host or Cloud SQL instance name")
	flag.StringVar(&cf.dbName, "db_name", "guestbook", "database name")
	flag.StringVar(&cf.dbUser, "db_user", "guestbook", "database user")
	flag.StringVar(&cf.dbPassword, "db_password", "", "database user password")
	flag.StringVar(&cf.motdVar, "motd_var", "", "message of the day variable location")
	flag.DurationVar(&cf.motdVarWaitTime, "motd_var_wait_time", 5*time.Second, "polling frequency of message of the day")
	flag.StringVar(&cf.cloudSQLRegion, "cloud_sql_region", "", "region of the Cloud SQL instance (GCP only)")
	flag.StringVar(&cf.runtimeConfigName, "runtime_config", "", "Runtime Configurator config resource (GCP only)")
	flag.Parse()

	ctx := context.Background()
	var srv *server.Server
	var cleanup func()
	var err error
	switch envFlag {
	case "gcp":
		srv, cleanup, err = setupGCP(ctx, cf)
	case "aws":
		srv, cleanup, err = setupAWS(ctx, cf)
	case "azure":
		if cf.dbHost == "" {
			cf.dbHost = "localhost"
		}
		if cf.dbPassword == "" {
			cf.dbPassword = "xyzzy"
		}
		srv, cleanup, err = setupAzure(ctx, cf)
	case "local":
		// The default MySQL instance is running on localhost
		// with this root password.
		if cf.dbHost == "" {
			cf.dbHost = "localhost"
		}
		if cf.dbPassword == "" {
			cf.dbPassword = "xyzzy"
		}
		srv, cleanup, err = setupLocal(ctx, cf)
	default:
		log.Fatalf("unknown -env=%s", envFlag)
	}
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// Listen and serve HTTP.
	log.Printf("Running, connected to %q cloud", envFlag)
	log.Fatal(srv.ListenAndServe(*addr))
}

// applicationSet is the Wire provider set for the Guestbook application that
// does not depend on the underlying platform.
var applicationSet = wire.NewSet(
	newApplication,
	appHealthChecks,
	newRouter,
	wire.Bind(new(http.Handler), new(*mux.Router)),
)

func newRouter(app *application) *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/", app.index)
	r.HandleFunc("/sign", app.sign)
	r.HandleFunc("/blob/{key:.+}", app.serveBlob)
	return r
}

// application is the main server struct for Guestbook. It contains the state of
// the most recently read message of the day.
type application struct {
	db      *sql.DB
	bucket  *blob.Bucket
	motdVar *runtimevar.Variable
}

// newApplication creates a new application struct based on the backends and the message
// of the day variable.
func newApplication(db *sql.DB, bucket *blob.Bucket, motdVar *runtimevar.Variable) *application {
	return &application{
		db:      db,
		bucket:  bucket,
		motdVar: motdVar,
	}
}

// index serves the server's landing page. It lists the 100 most recent
// greetings, shows a cloud environment banner, and displays the message of the
// day.
func (app *application) index(w http.ResponseWriter, r *http.Request) {
	var data struct {
		MOTD      string
		Env       string
		BannerSrc string
		Greetings []greeting
	}
	snap, err := app.motdVar.Latest(r.Context())
	if err != nil {
		log.Println("index page error:", err)
		http.Error(w, "could not load motd", http.StatusInternalServerError)
		return
	}
	data.MOTD = snap.Value.(string)

	switch envFlag {
	case "gcp":
		data.Env = "GCP"
		data.BannerSrc = "/blob/gcp.png"
	case "aws":
		data.Env = "AWS"
		data.BannerSrc = "/blob/aws.png"
	case "azure":
		data.Env = "Azure"
		data.BannerSrc = "/blob/azure.png"
	case "local":
		data.Env = "Local"
		data.BannerSrc = "/blob/gophers.jpg"
	}

	const query = "SELECT content FROM (SELECT content, post_date FROM greetings ORDER BY post_date DESC LIMIT 100) AS recent_greetings ORDER BY post_date ASC;"
	q, err := app.db.QueryContext(r.Context(), query)
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
	height: 125px;
	width: 250px;
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
func (app *application) sign(w http.ResponseWriter, r *http.Request) {
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
	_, err := app.db.ExecContext(r.Context(), sqlStmt, content)
	if err != nil {
		log.Println("sign SQL error:", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// serveBlob handles a request for a static asset by retrieving it from a bucket.
func (app *application) serveBlob(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]
	blobReader, err := app.bucket.NewReader(r.Context(), key, nil)
	if err != nil {
		log.Println("serve blob:", err)
		if gcerrors.Code(err) == gcerrors.NotFound {
			http.Error(w, "blob not found", http.StatusNotFound)
		} else {
			http.Error(w, "blob read error", http.StatusInternalServerError)
		}
		return
	}
	defer blobReader.Close()
	w.Header().Set("Content-Type", blobReader.ContentType())
	w.Header().Set("Content-Length", strconv.FormatInt(blobReader.Size(), 10))
	if _, err = io.Copy(w, blobReader); err != nil {
		log.Println("Copying blob:", err)
	}
}

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
