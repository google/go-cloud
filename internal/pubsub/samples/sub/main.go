// sub is a sample application that publishes messages from stdin to an
// existing topic.
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
