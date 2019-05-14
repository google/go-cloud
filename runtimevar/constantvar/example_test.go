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

package constantvar_test

import (
	"context"
	"errors"
	"fmt"
	"log"

	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/constantvar"
)

// MyConfig is a sample configuration struct.
type MyConfig struct {
	Server string
	Port   int
}

func ExampleNew() {
	// cfg is our sample config.
	cfg := MyConfig{Server: "foo.com", Port: 80}

	// Construct a *runtimevar.Variable that always returns cfg.
	v := constantvar.New(cfg)
	defer v.Close()

	// We can now read the current value of the variable from v.
	snapshot, err := v.Latest(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	cfg = snapshot.Value.(MyConfig)
	fmt.Printf("%s running on port %d", cfg.Server, cfg.Port)

	// Output:
	// foo.com running on port 80
}

func ExampleNewBytes() {

	// Create a decoder for decoding JSON strings into MyConfig.
	decoder := runtimevar.NewDecoder(MyConfig{}, runtimevar.JSONDecode)

	// Construct a *runtimevar.Variable based on a JSON string.
	v := constantvar.NewBytes([]byte(`{"Server": "foo.com", "Port": 80}`), decoder)
	defer v.Close()

	// We can now read the current value of the variable from v.
	snapshot, err := v.Latest(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	cfg := snapshot.Value.(MyConfig)
	fmt.Printf("%s running on port %d", cfg.Server, cfg.Port)

	// Output:
	// foo.com running on port 80
}

func ExampleNewError() {
	// Construct a runtimevar.Variable that always returns errFake.
	var errFake = errors.New("my error")
	v := constantvar.NewError(errFake)
	defer v.Close()

	// We can now use Watch to read the current value of the variable
	// from v. Note that Latest would block here since it waits for
	// a "good" value, and v will never get one.
	_, err := v.Watch(context.Background())
	if err == nil {
		log.Fatal("Expected an error!")
	}
	fmt.Println(err)

	// Output:
	// runtimevar (code=Unknown): my error
}

func Example_openVariableFromURL() {
	// runtimevar.OpenVariable creates a *runtimevar.Variable from a URL.
	ctx := context.Background()
	v, err := runtimevar.OpenVariable(ctx, "constant://?val=hello+world&decoder=string")
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()

	snapshot, err := v.Latest(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(snapshot.Value.(string))

	// Output
	// hello world
}
