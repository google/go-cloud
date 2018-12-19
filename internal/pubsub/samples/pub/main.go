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
		fmt.Fprintf(os.Stderr, "usage: pub [flags] topic\n")
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

	return nil
}
