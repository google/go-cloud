// Copyright 2018 The Go Cloud Authors
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

package filevar_test

import (
	"log"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/filevar"
)

// MyAppConfig is the unmarshaled type for myapp.conf file.
type MyAppConfig struct {
	MsgOfTheDay string `json:"msg_of_the_day"`
}

func ExampleNew() {
	// Configure a JSON decoder for myapp.json to unmarshal into a MyAppConfig object.
	v, err := filevar.New("/etc/myapp/myapp.json", runtimevar.NewDecoder(&MyAppConfig{}, runtimevar.JSONDecode), nil)
	if err != nil {
		log.Fatalf("Error in constructing variable: %v", err)
	}
	v.Close()
}
