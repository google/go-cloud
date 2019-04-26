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

// A simple "hello world" application using server.Server, to be run on
// Google App Engine (GAE).
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"gocloud.dev/server"
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/", handle)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := server.New(r, nil)
	log.Printf("Listening on port %s", port)
	log.Fatal(srv.ListenAndServe(fmt.Sprintf(":%s", port)))
}

func handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	fmt.Fprint(w, "Hello world!")
}
