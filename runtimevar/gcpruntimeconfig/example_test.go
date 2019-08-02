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

	// Connect to the Runtime Configurator service.
	client, cleanup, err := gcpruntimeconfig.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	// You can use the VariableKey helper to construct a Variable key from
	// your project ID, config ID, and the variable name; alternatively,
	// you can construct the full string yourself (e.g.,
	// "projects/gcp-project-id/configs/config-id/variables/variable-name").
	// See https://cloud.google.com/deployment-manager/runtime-configurator/
	// for more details.
	//
	// For this example, the GCP Cloud Runtime Configurator variable being
	// referenced should have a JSON string that decodes into MyConfig.
	variableKey := gcpruntimeconfig.VariableKey("gcp-project-id", "config-id", "variable-name")

	// Construct a *runtimevar.Variable that watches the variable.
	v, err := gcpruntimeconfig.OpenVariable(client, variableKey, runtimevar.StringDecoder, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()
}

func Example_openVariableFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/runtimevar/gcpruntimeconfig"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// runtimevar.OpenVariable creates a *runtimevar.Variable from a URL.
	// The URL Host+Path are used as the GCP Runtime Configurator Variable key;
	// see https://cloud.google.com/deployment-manager/runtime-configurator/
	// for more details.

	v, err := runtimevar.OpenVariable(ctx, "gcpruntimeconfig://projects/myproject/configs/myconfigid/variables/myvar?decoder=string")
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()
}
