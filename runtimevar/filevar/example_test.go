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

package filevar_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"

	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/filevar"
)

// MyConfig is a sample configuration struct.
type MyConfig struct {
	Server string
	Port   int
}

func ExampleNew() {
	// cfgJSON is a JSON string that can be decoded into a MyConfig.
	const cfgJSON = `{"Server": "foo.com", "Port": 80}`

	// Create a temporary file to hold our config.
	f, err := ioutil.TempFile("", "")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := f.Write([]byte(cfgJSON)); err != nil {
		log.Fatal(err)
	}

	// Create a decoder for decoding JSON strings into MyConfig.
	decoder := runtimevar.NewDecoder(MyConfig{}, runtimevar.JSONDecode)

	// Construct a runtimevar.Variable pointing at f.
	v, err := filevar.New(f.Name(), decoder, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()

	// Verify the variable value; it will be of type MyConfig.
	snapshot, err := v.Watch(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Value: %#v\n", snapshot.Value.(MyConfig))

	// Output:
	// Value: filevar_test.MyConfig{Server:"foo.com", Port:80}
}
