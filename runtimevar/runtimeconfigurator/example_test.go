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

	"gocloud.dev/gcp"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/runtimeconfigurator"
)

// MyConfig is a sample configuration struct.
type MyConfig struct {
	Server string
	Port   int
}

func ExampleNew() {
	ctx := context.Background()

	// Get GCP credentials and dial the server.
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}
	client, cleanup, err := runtimeconfigurator.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// Create a decoder for decoding JSON strings into MyConfig.
	decoder := runtimevar.NewDecoder(MyConfig{}, runtimevar.JSONDecode)

	// Fill these in with the values from the Cloud Console.
	// For this example, the GCP Cloud Runtime Configurator variable being
	// referenced should have a JSON string that decodes into MyConfig.
	name := runtimeconfigurator.ResourceName{
		ProjectID: "projectID",
		Config:    "configName",
		Variable:  "appConfig",
	}

	// Construct a *runtimevar.Variable that watches the variable.
	v, err := runtimeconfigurator.NewVariable(client, name, decoder, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()

	// You can now read the current value of the variable from v.
	// The resulting runtimevar.Snapshot.Value will be of type MyConfig.
}
