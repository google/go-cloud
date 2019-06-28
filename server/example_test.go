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

package server_test

import (
	"fmt"
	"net/http"
	"os"

	"gocloud.dev/requestlog"
	"gocloud.dev/server"
)

func ExampleServer_New() {
	// This example is used in https://gocloud.dev/howto/server/

	// Use the constructor function to create the server.
	srv := server.New(http.DefaultServeMux, nil)

	// Register a route.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, World!")
	})

	// Start the server.
	srv.ListenAndServe(":8080")
}

func ExampleServer_RequestLogger() {
	// This example is used in https://gocloud.dev/howto/server/

	// Create a logger, and assign it to the RequestLogger field of a
	// server.Options struct.
	srvOptions := &server.Options{
		RequestLogger: requestlog.NewNCSALogger(os.Stdout, func(error) {}),
	}

	// Pass the options to the Server constructor.
	srv := server.New(http.DefaultServeMux, srvOptions)

	// Register a route.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, World!")
	})

	// Start the server. You will see requests logged to STDOUT.
	if err := srv.ListenAndServe(":8080"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
