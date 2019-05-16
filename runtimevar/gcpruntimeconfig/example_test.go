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

package gcpruntimeconfig_test

import (
	"context"
	"log"

	"gocloud.dev/gcp"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/gcpruntimeconfig"
)

// MyConfig is a sample configuration struct.
type MyConfig struct {
	Server string
	Port   int
}

func ExampleOpenVariable() {
	// Your GCP credentials.
	// See https://cloud.google.com/docs/authentication/production
	// for more info on alternatives.
	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Connect to the Runtime Configurator service.
	client, cleanup, err := gcpruntimeconfig.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// Create a decoder for decoding JSON strings into MyConfig.
	decoder := runtimevar.NewDecoder(MyConfig{}, runtimevar.JSONDecode)

	// Fill these in with the values from the Cloud Console.
	// For this example, the GCP Cloud Runtime Configurator variable being
	// referenced should have a JSON string that decodes into MyConfig.
	name := gcpruntimeconfig.ResourceName{
		ProjectID: "gcp-project-id",
		Config:    "cfg-name",
		Variable:  "cfg-variable-name",
	}

	// Construct a *runtimevar.Variable that watches the variable.
	v, err := gcpruntimeconfig.OpenVariable(client, name, decoder, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()

	// We can now read the current value of the variable from v.
	snapshot, err := v.Latest(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	cfg := snapshot.Value.(MyConfig)
	_ = cfg
}

func Example_openVariableFromURL() {
	// runtimevar.OpenVariable creates a *runtimevar.Variable from a URL.
	ctx := context.Background()
	v, err := runtimevar.OpenVariable(ctx, "gcpruntimeconfig://myproject/myconfigid/myvar?decoder=string")
	if err != nil {
		log.Fatal(err)
	}

	snapshot, err := v.Latest(ctx)
	_, _ = snapshot, err
}
