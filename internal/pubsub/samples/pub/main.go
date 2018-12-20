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

// pub is a sample application that publishes messages from stdin to an
// existing topic.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"gocloud.dev/internal/pubsub"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("pub: ")
	envFlag := flag.String("env", "", "environment [gcp|rabbit]")
	flag.Parse()
	if *envFlag == "" {
		log.Fatal("missing -env flag\n")
	}
	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: pub -env=<env> [flags] <topic-name>\n")
	}
	if err := pub(flag.Arg(0), *envFlag); err != nil {
		log.Fatal(err)
	}
}

func pub(topicID, env string) error {
	// Open a subscription in the specified environment.
	ctx := context.Background()
	var (
		topic   *pubsub.Topic
		cleanup func()
		err     error
	)
	switch env {
	case "gcp":
		topic, cleanup, err = openGCPTopic(ctx, topicID)
	case "rabbit":
		topic, cleanup, err = openRabbitTopic(topicID)
	default:
		return fmt.Errorf("unrecognized -env: %s", env)
	}
	if err != nil {
		return err
	}
	defer cleanup()

	// Get lines from stdin and send them as messages to the topic.
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		m := &pubsub.Message{Body: []byte(line)}
		if err := topic.Send(ctx, m); err != nil {
			return err
		}
	}
	return scanner.Err()
}
