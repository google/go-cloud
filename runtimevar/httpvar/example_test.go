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

package httpvar_test

import (
	"context"
	"log"
	"net/http"

	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/httpvar"
)

// MyConfig is a sample configuration struct.
type MyConfig struct {
	Server string
	Port   int
}

func ExampleOpenVariable() {
	httpClient := http.DefaultClient
	decoder := runtimevar.NewDecoder(MyConfig{}, runtimevar.JSONDecode)

	// Construct a *runtimevar.Variable that watches the variable.
	v, err := httpvar.OpenVariable(httpClient, "http://example.com", decoder, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()

	// We can now read the current value of the variable from v.
	snapshot, err := v.Watch(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	cfg := snapshot.Value.(MyConfig)
	_ = cfg
}

func Example_openVariableFromURL() {
	// runtimevar.OpenVariable creates a *runtimevar.Variable from a URL.
	// The "decoder" query parameter is optional, and is removed before fetching
	// the URL.
	ctx := context.Background()
	v, err := runtimevar.OpenVariable(ctx, "http://myserver.com/foo.txt?decoder=string")
	if err != nil {
		log.Fatal(err)
	}

	snapshot, err := v.Latest(ctx)
	_, _ = snapshot, err
}
