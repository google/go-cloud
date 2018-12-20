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

// sub is a sample application that receives messages from an existing subscription.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"gocloud.dev/internal/pubsub"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("sub: ")
	envFlag := flag.String("env", "", "environment [gcp|rabbit]")
	nFlag := flag.Int("n", 0, "number of messages to receive, or 0 for unlimited")
	flag.Parse()
	if *envFlag == "" {
		log.Fatal("missing -env flag\n")
	}
	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: sub -env=<env> [flags] <subscription-name>\n")
	}
	if err := sub(flag.Arg(0), *envFlag, *nFlag); err != nil {
		log.Fatal(err)
	}
}

func sub(subID, env string, n int) error {
	// Open a subscription in the specified environment.
	ctx := context.Background()
	var (
		sub     *pubsub.Subscription
		cleanup func()
		err     error
	)
	switch env {
	case "gcp":
		sub, cleanup, err = openGCPSubscription(ctx, subID)
	case "rabbit":
		sub, cleanup, err = openRabbitSubscription(subID)
	default:
		log.Fatalf("unrecognized -env: %q", env)
	}
	if err != nil {
		return err
	}
	defer cleanup()

	// Receive messages and send them to stdout.
	for i := 0; n == 0 || i < n; i++ {
		m, err := sub.Receive(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", m.Body)
		m.Ack()
	}
	return nil
}
