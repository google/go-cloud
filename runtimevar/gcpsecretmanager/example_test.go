// Copyright 2020 The Go Cloud Development Kit Authors
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

package gcpsecretmanager_test

import (
	"context"
	"log"

	"gocloud.dev/gcp"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/gcpsecretmanager"
)

func ExampleOpenVariable() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Your GCP credentials.
	// See https://cloud.google.com/docs/authentication/production
	// for more info on alternatives.
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Connect to the GCP Secret Manager service.
	client, cleanup, err := gcpsecretmanager.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// You can use the SecretKey helper to construct a secret key from
	// your project ID and the secret ID; alternatively,
	// you can construct the full string yourself (e.g.,
	// "projects/gcp-project-id/secrets/secret-id").
	// gcpsecretmanager package will always use the latest secret value,
	// so `/version/latest` postfix must NOT be added to the secret key.
	// See https://cloud.google.com/secret-manager
	// for more details.
	//
	// For this example, the GCP Secret Manager secret being
	// referenced should have a JSON string that decodes into MyConfig.
	variableKey := gcpsecretmanager.SecretKey("gcp-project-id", "secret-id")

	// Construct a *runtimevar.Variable that watches the variable.
	v, err := gcpsecretmanager.OpenVariable(client, variableKey, runtimevar.StringDecoder, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()
}

func Example_openVariableFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/runtimevar/gcpsecretmanager"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// runtimevar.OpenVariable creates a *runtimevar.Variable from a URL.
	// The URL Host+Path are used as the GCP Secret Manager secret key;
	// see https://cloud.google.com/secret-manager
	// for more details.

	v, err := runtimevar.OpenVariable(ctx, "gcpsecretmanager://projects/myproject/secrets/mysecret?decoder=string")
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()
}
