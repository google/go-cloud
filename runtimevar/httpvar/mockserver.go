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

package httpvar

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

func newMockServer() (*httptest.Server, func()) {
	var response interface{} = nil

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if response == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, response)
	})
	mux.HandleFunc("/create-variable", func(w http.ResponseWriter, r *http.Request) {
		response = r.URL.Query().Get("value")
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/delete-variable", func(w http.ResponseWriter, r *http.Request) {
		response = nil
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	return server, server.Close
}
