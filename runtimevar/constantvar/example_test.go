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

	// Construct a runtimevar.Variable that always returns cfg.
	v := constantvar.New(cfg)
	defer v.Close()

	// Verify the variable value.
	snapshot, err := v.Watch(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Value: %#v\n", snapshot.Value.(MyConfig))

	// Output:
	// Value: constantvar_test.MyConfig{Server:"foo.com", Port:80}
}

func ExampleNewBytes() {
	const (
		// cfgJSON is a JSON string that can be decoded into a MyConfig.
		cfgJSON = `{"Server": "foo.com", "Port": 80}`
		// badJSON is a JSON string that cannot be decoded into a MyConfig.
		badJSON = `{"Server": 42}`
	)

	// Create a decoder for decoding JSON strings into MyConfig.
	decoder := runtimevar.NewDecoder(MyConfig{}, runtimevar.JSONDecode)

	// Construct a runtimevar.Variable using badJSON. It will return an error
	// since the JSON doesn't decode successfully.
	v := constantvar.NewBytes([]byte(badJSON), decoder)
	_, err := v.Watch(context.Background())
	if err == nil {
		log.Fatal("Expected an error!")
	}
	v.Close()

	// Try again with valid JSON.
	v = constantvar.NewBytes([]byte(cfgJSON), decoder)
	defer v.Close()

	// Verify the variable value.
	snapshot, err := v.Watch(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Value: %#v\n", snapshot.Value.(MyConfig))

	// Output:
	// Value: constantvar_test.MyConfig{Server:"foo.com", Port:80}
}

func ExampleNewError() {
	var errFake = errors.New("my error")

	// Construct a runtimevar.Variable that always returns errFake.
	v := constantvar.NewError(errFake)
	defer v.Close()

	// The variable returns an error. It is wrapped by runtimevar, so it's
	// not equal to errFake.
	_, err := v.Watch(context.Background())
	if err == nil {
		log.Fatal("Expected an error!")
	}
	fmt.Println(err)

	// Output:
	// runtimevar: my error
}
