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

package runtimeconfigurator_test

import (
	"context"
	"log"

	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/runtimeconfigurator"
)

func ExampleNewClient() {
	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}
	stub, cleanup, err := runtimeconfigurator.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()
	client := runtimeconfigurator.NewClient(stub)

	// Now use the client.
	_ = client
}

func ExampleClient_NewVariable() {
	// Assume client was created at server startup.
	ctx := context.Background()
	client := openClient()

	// MyAppConfig is an example unmarshaled type for configuration stored in key
	// "projects/projectID/configs/configName/variables/appConfig". Runtime
	// Configurator stores individual variables as strings or binary data, so a
	// decoder automatically parses the data.
	type MyAppConfig struct {
		MsgOfTheDay string `json:"msg_of_the_day"`
	}
	decoder := runtimevar.NewDecoder(&MyAppConfig{}, runtimevar.JSONDecode)

	// Fill these in with the values from the Cloud Console.
	name := runtimeconfigurator.ResourceName{
		ProjectID: "projectID",
		Config:    "configName",
		Variable:  "appConfig",
	}

	// Create a variable object to watch for changes.
	v, err := client.NewVariable(name, decoder, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()

	// Read the current value. Calling Watch() again will block until the value
	// changes.
	snapshot, err := v.Watch(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Value will always be of the decoder's type.
	cfg := snapshot.Value.(*MyAppConfig)
	log.Println(cfg.MsgOfTheDay)
}

func openClient() *runtimeconfigurator.Client {
	return nil
}
