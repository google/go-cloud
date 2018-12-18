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
	"fmt"
	"log"

	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/constantvar"
)

func Example_jsonVariable() {

	// DBConfig is the sample config struct we're going to parse our JSON into.
	type DBConfig struct {
		Host     string
		Port     int
		Username string
	}

	// Here's our sample JSON config.
	const jsonConfig = `{"Host": "gocloud.dev", "Port": 8080, "Username": "testuser"}`

	// We need a Decoder that decodes raw bytes into our config.
	decoder := runtimevar.NewDecoder(DBConfig{}, runtimevar.JSONDecode)

	// Next, a construct a *Variable using a constructor from one of the
	// runtimevar subpackages. This example uses constantvar.
	v := constantvar.NewBytes([]byte(jsonConfig), decoder)
	defer v.Close()

	// Call Watch to retrieve the value.
	snapshot, err := v.Watch(context.Background())
	if err != nil {
		log.Fatalf("Error in retrieving variable: %v", err)
	}
	// snapshot.Value will be of type DBConfig.
	fmt.Printf("Config: %+v\n", snapshot.Value.(DBConfig))

	// Output:
	// Config: {Host:gocloud.dev Port:8080 Username:testuser}
}

func Example_stringVariable() {
	// Next, a construct a *Variable using a constructor from one of the
	// runtimevar subpackages. This example uses constantvar.
	// The variable value is of type string, so we use StringDecoder.
	v := constantvar.NewBytes([]byte("hello world"), runtimevar.StringDecoder)
	defer v.Close()

	// Call Watch to retrieve the value.
	snapshot, err := v.Watch(context.Background())
	if err != nil {
		log.Fatalf("Error in retrieving variable: %v", err)
	}
	// snapshot.Value will be of type string.
	fmt.Printf("%q\n", snapshot.Value.(string))

	// Output:
	// "hello world"
}
