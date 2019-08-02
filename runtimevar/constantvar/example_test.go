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

func ExampleNew() {
	// Construct a *runtimevar.Variable that always returns "hello world".
	v := constantvar.New("hello world")
	defer v.Close()

	// We can now read the current value of the variable from v.
	snapshot, err := v.Latest(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(snapshot.Value.(string))

	// Output:
	// hello world
}

func ExampleNewBytes() {
	// Construct a *runtimevar.Variable with a []byte.
	v := constantvar.NewBytes([]byte(`hello world`), runtimevar.BytesDecoder)
	defer v.Close()

	// We can now read the current value of the variable from v.
	snapshot, err := v.Latest(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("byte slice of length %d\n", len(snapshot.Value.([]byte)))

	// Output:
	// byte slice of length 11
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
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/runtimevar/constantvar"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// runtimevar.OpenVariable creates a *runtimevar.Variable from a URL.

	v, err := runtimevar.OpenVariable(ctx, "constant://?val=hello+world&decoder=string")
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()
	// PRAGMA: On gocloud.dev, hide the rest of the function.
	snapshot, err := v.Latest(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(snapshot.Value.(string))

	// Output
	// hello world
}
