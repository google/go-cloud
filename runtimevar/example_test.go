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

package runtimevar_test

import (
	"context"
	"log"

	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/filevar"
)

type DBConfig struct {
	Host     string
	Port     string
	Username string
	Name     string
}

func initVariable() (*runtimevar.Variable, func()) {
	// Construct a runtimevar.Variable object.
	v, err := filevar.New("/etc/myapp/db.json", runtimevar.NewDecoder(&DBConfig{}, runtimevar.JSONDecode), nil)
	if err != nil {
		log.Fatal(err)
	}

	return v, func() {
		v.Close()
	}
}

func Example() {
	v, cleanup := initVariable()
	defer cleanup()

	ctx := context.Background()
	// Call Watch to retrieve initial value before proceeding.
	snap, err := v.Watch(ctx)
	if err != nil {
		log.Fatalf("Error in retrieving initial variable: %v", err)
	}
	log.Printf("Value: %+v", snap.Value.(*DBConfig))

	// Get a Context with cancel func to stop the Watch call.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Have a separate goroutine that waits for changes.
	go func() {
		for ctx.Err() == nil {
			snap, err := v.Watch(ctx)
			if err != nil {
				// Handle errors.
				log.Printf("Error: %v", err)
				continue
			}
			// Use updated configuration accordingly.
			log.Printf("Value: %+v", snap.Value.(*DBConfig))
		}
	}()
	// Output:
}

func Example_NewDecoder_string() {
	// Configure a JSON decoder for myapp.json to unmarshal into a MyAppConfig object.
	v, err := filevar.New("/etc/myapp/myapp.json", runtimevar.NewDecoder(&MyAppConfig{}, runtimevar.JSONDecode), nil)
	if err != nil {
		log.Fatalf("Error in constructing variable: %v", err)
	}
	v.Close()
	// Output:
}

// MyAppConfig is the unmarshaled type for myapp.conf file.
type MyAppConfig struct {
	MsgOfTheDay string `json:"msg_of_the_day"`
}

func ExampleNew() {
	// Output:
}

/*
TODO: (note to self)
-- Use constantvar in some examples
-- Use filevar in another one, but actually create the value
*/
