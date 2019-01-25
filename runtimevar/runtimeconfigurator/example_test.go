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

package runtimeconfigurator_test

import (
	"context"
	"fmt"
	"log"

	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/runtimeconfigurator"
	"golang.org/x/oauth2/google"
)

// MyConfig is a sample configuration struct.
type MyConfig struct {
	Server string
	Port   int
}

// jsonCreds is a fake GCP JSON credentials file.
const jsonCreds = `
{
  "type": "service_account",
  "project_id": "my-project-id"
}
`

func ExampleNewVariable() {
	ctx := context.Background()

	// Get GCP credentials and dial the server.
	// Here we use a fake JSON credentials file, but you could also use
	// gcp.DefaultCredentials(ctx) to use the default GCP credentials from
	// the environment.
	// See https://cloud.google.com/docs/authentication/production
	// for more info on alternatives.
	creds, err := google.CredentialsFromJSON(ctx, []byte(jsonCreds))
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
	snapshot, err := v.Watch(ctx)
	if err != nil {
		// This is expected due to the fake credentials we used above.
		fmt.Println("Watch failed due to invalid credentials")
		return
	}
	// We'll never get here when running this sample, but the resulting
	// runtimevar.Snapshot.Value would be of type MyConfig.
	log.Printf("Snapshot.Value: %#v", snapshot.Value.(MyConfig))

	// Output:
	// Watch failed due to invalid credentials
}
