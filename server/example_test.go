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

	"github.com/google/go-cloud/server"
	_ "gocloud.dev/secrets/gcpkms"
)

func Example() {
	// This example is used in https://gocloud.dev/howto/server/

	// The Server constructor takes an http.Handler and an Options struct.
	srv := server.New(http.DefaultServeMux, nil)

	// Register a route.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, World!")
	})

	// Start the server.
	srv.ListenAndServe(":8080")
}

// func ExampleServer_() {
// 	// This example is used in https://gocloud.dev/howto/server/
// }
