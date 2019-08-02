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

package runtimevar_test

import (
	"context"
	"fmt"
	"log"

	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/constantvar"
	"gocloud.dev/secrets"

	_ "gocloud.dev/runtimevar/gcpruntimeconfig"
	runtimeconfig "google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1"
	"google.golang.org/grpc/status"
)

func Example_jsonDecoder() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Config is the sample config struct we're going to parse our JSON into.
	type Config struct {
		Host string
		Port int
	}

	// A sample JSON config that will decode into Config.
	const jsonConfig = `{"Host": "gocloud.dev", "Port": 8080}`

	// Construct a Decoder that decodes raw bytes into our config.
	decoder := runtimevar.NewDecoder(Config{}, runtimevar.JSONDecode)

	// Next, a construct a *Variable using a constructor or URL opener.
	// This example uses constantvar.
	// If you're using a URL opener, you can't decode JSON into a struct, but
	// you can use the query parameter "decoder=jsonmap" to decode into a map.
	v := constantvar.NewBytes([]byte(jsonConfig), decoder)
	defer v.Close()
	// snapshot.Value will be of type Config.

	// PRAGMA: On gocloud.dev, hide the rest of the function.
	snapshot, err := v.Latest(ctx)
	if err != nil {
		log.Fatalf("Error in retrieving variable: %v", err)
	}
	fmt.Printf("Config: %+v\n", snapshot.Value.(Config))

	// Output:
	// Config: {Host:gocloud.dev Port:8080}
}

func Example_stringDecoder() {
	// Construct a *Variable using a constructor from one of the
	// runtimevar subpackages. This example uses constantvar.
	// The variable value is of type string, so we use StringDecoder.
	v := constantvar.NewBytes([]byte("hello world"), runtimevar.StringDecoder)
	defer v.Close()

	// Call Latest to retrieve the value.
	snapshot, err := v.Latest(context.Background())
	if err != nil {
		log.Fatalf("Error in retrieving variable: %v", err)
	}
	// snapshot.Value will be of type string.
	fmt.Printf("%q\n", snapshot.Value.(string))

	// Output:
	// "hello world"
}

func ExampleVariable_Latest() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	var v *runtimevar.Variable

	snapshot, err := v.Latest(context.Background())
	if err != nil {
		log.Fatalf("Error in retrieving variable: %v", err)
	}
	// PRAGMA: On gocloud.dev, hide the rest of the function.
	_ = snapshot
}

func ExampleSnapshot_As() {
	// This example is specific to the gcpruntimeconfig implementation; it
	// demonstrates access to the underlying
	// google.golang.org/genproto/googleapis/cloud/runtimeconfig.Variable type.
	// The types exposed for As by gcpruntimeconfig are documented in
	// https://godoc.org/gocloud.dev/runtimevar/gcpruntimeconfig#hdr-As
	ctx := context.Background()

	const url = "gcpruntimeconfig://proj/config/key"
	v, err := runtimevar.OpenVariable(ctx, url)
	if err != nil {
		log.Fatal(err)
	}

	s, err := v.Latest(ctx)
	if err != nil {
		log.Fatal(err)
	}

	var rcv *runtimeconfig.Variable
	if s.As(&rcv) {
		fmt.Println(rcv.UpdateTime)
	}
}

func ExampleVariable_ErrorAs() {
	// This example is specific to the gcpruntimeconfig implementation; it
	// demonstrates access to the underlying google.golang.org/grpc/status.Status
	// type.
	// The types exposed for As by gcpruntimeconfig are documented in
	// https://godoc.org/gocloud.dev/runtimevar/gcpruntimeconfig#hdr-As
	ctx := context.Background()

	const url = "gcpruntimeconfig://proj/wrongconfig/key"
	v, err := runtimevar.OpenVariable(ctx, url)
	if err != nil {
		log.Fatal(err)
	}

	_, err = v.Watch(ctx)
	if err != nil {
		var s *status.Status
		if v.ErrorAs(err, &s) {
			fmt.Println(s.Code())
		}
	}
}

func ExampleVariable_Watch() {
	// Construct a *Variable using a constructor from one of the
	// runtimevar subpackages. This example uses constantvar.
	// The variable value is of type string, so we use StringDecoder.
	v := constantvar.NewBytes([]byte("hello world"), runtimevar.StringDecoder)
	defer v.Close()

	// Call Watch in a loop from a background goroutine to see all changes,
	// including errors.
	//
	// You can use this for logging, or to trigger behaviors when the
	// config changes.
	//
	// Note that Latest always returns the latest "good" config, so seeing
	// an error from Watch doesn't mean that Latest will return one.
	go func() {
		for {
			snapshot, err := v.Watch(context.Background())
			if err == runtimevar.ErrClosed {
				// v has been closed; exit.
				return
			}
			if err == nil {
				// Casting to a string here because we used StringDecoder.
				log.Printf("New config: %v", snapshot.Value.(string))
			} else {
				log.Printf("Error loading config: %v", err)
				// Even though there's been an error loading the config,
				// v.Latest will continue to return the latest "good" value.
			}
		}
	}()
}

func ExampleDecryptDecode() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	var keeper *secrets.Keeper

	decodeFunc := runtimevar.DecryptDecode(keeper, runtimevar.StringDecode)
	decoder := runtimevar.NewDecoder("", decodeFunc)

	// PRAGMA: On gocloud.dev, hide the rest of the function.
	_ = decoder
}
