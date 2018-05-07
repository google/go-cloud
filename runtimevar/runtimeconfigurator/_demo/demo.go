// Copyright 2018 Google LLC
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

// This binary demonstrates watching over a Runtime Configurator variable using the runtimevar
// package with the gcpconfig package as the driver implementation.  To cancel the Config.Watch
// call, enter 'x' and '<enter>' keys on the terminal.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"reflect"
	"time"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/gcpconfig"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintf(os.Stderr,
			"Usage: %s <project-id> <config-name> <var-name>\n\n",
			path.Base(os.Args[0]))
		os.Exit(1)
	}

	name := gcpconfig.ResourceName{
		ProjectID: os.Args[1],
		Config:    os.Args[2],
		Variable:  os.Args[3],
	}

	ctx := context.Background()
	client, err := gcpconfig.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cfg, err := client.NewConfig(ctx, name, "", &gcpconfig.WatchOptions{
		WaitTime: 10 * time.Second,
		Decode:   stringDecode,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer cfg.Close()

	go func() {
		key := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(key)
			if err != nil {
				log.Printf("stdin error: %v\n", err)
			}
			if n == 1 && key[0] == 'x' {
				log.Println("That's all folks!")
				cancel()
				time.Sleep(1 * time.Second)
				os.Exit(0)
			}
		}
	}()

	snap, err := cfg.Watch(ctx)
	if err != nil {
		log.Fatalf("Failed at fetching initial config: %v", err)
	}
	log.Printf("Watching config %v", snapString(&snap))

	isWatching := true
	for isWatching {
		select {
		case <-ctx.Done():
			isWatching = false
		default:
			snap, err := cfg.Watch(ctx)
			if err == nil {
				log.Printf("Updated: %s", snapString(&snap))
			} else {
				log.Printf("Error: %v", err)
			}
		}
	}
}

func snapString(cfg *runtimevar.Snapshot) string {
	return fmt.Sprintf("<value: %q, updateTime: %v>", cfg.Value.(string), cfg.UpdateTime)
}

func stringDecode(b []byte, obj interface{}) error {
	// obj is a pointer to a string.
	v := reflect.ValueOf(obj).Elem()
	v.SetString(string(b))
	return nil
}
