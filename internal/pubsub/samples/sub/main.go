// sub is a sample application that publishes messages from stdin to an
// existing topic.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/google/subcommands"
	"gocloud.dev/internal/pubsub"
)

type newCmd struct{}

func (*newCmd) Name() string     { return "new" }
func (*newCmd) Synopsis() string { return "Create a new subscription" }
func (*newCmd) Usage() string {
	return `new <topic> <subscription>:
	Create a new subscription to an existing topic.
`
}

func (*newCmd) SetFlags(f *flag.FlagSet) {
}

func (c *newCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 2 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	topicID := f.Arg(0)
	subID := f.Arg(1)
	ctx := context.Background()
	var err error
	switch envFlag {
	case "gcp":
		err = makeGCPSubscription(ctx, topicID, subID)
	case "rabbit":
		err = makeRabbitSubscription(topicID, subID)
	default:
		log.Fatalf("unrecognized -env: %s", envFlag)
	}
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

type delCmd struct{}

func (*delCmd) Name() string     { return "del" }
func (*delCmd) Synopsis() string { return "Delete a subscription" }
func (*delCmd) Usage() string {
	return `del <subscription>:
	Delete a subscription.
`
}

func (*delCmd) SetFlags(f *flag.FlagSet) {
}

func (c *delCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	subID := f.Arg(0)
	ctx := context.Background()
	var err error
	switch envFlag {
	case "gcp":
		err = deleteGCPSubscription(ctx, subID)
	case "rabbit":
		err = deleteRabbitSubscription(subID)
	default:
		log.Fatalf("unrecognized -env: %s", envFlag)
	}
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

type recvCmd struct {
	n int
}

func (*recvCmd) Name() string     { return "recv" }
func (*recvCmd) Synopsis() string { return "Send a message to a topic" }
func (*recvCmd) Usage() string {
	return `recv <subscription>
	Send a messages to a topic, one message per line from stdin.
`
}

func (c *recvCmd) SetFlags(f *flag.FlagSet) {
	f.IntVar(&c.n, "n", 0, "number of messages to receive, or 0 for unlimited")
}

func (c *recvCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	// Get the subscription ID from the command line.
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	subID := f.Arg(0)

	// Open a subscription in the specified environment.
	ctx := context.Background()
	var (
		sub     *pubsub.Subscription
		cleanup func()
		err     error
	)
	switch envFlag {
	case "gcp":
		sub, cleanup, err = openGCPSubscription(ctx, subID)
	case "rabbit":
		sub, cleanup, err = openRabbitSubscription(subID)
	default:
		log.Fatalf("unrecognized -env: %s", envFlag)
	}
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}
	defer cleanup()

	// Receive messages and send them to stdout.
	for i := 0; c.n == 0 || i < c.n; i++ {
		m, err := sub.Receive(ctx)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", m.Body)
		m.Ack()
	}
	return subcommands.ExitSuccess
}

var envFlag string

func main() {
	log.SetFlags(0)
	log.SetPrefix("sub: ")
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(&newCmd{}, "")
	subcommands.Register(&delCmd{}, "")
	subcommands.Register(&recvCmd{}, "")
	flag.StringVar(&envFlag, "env", "", "environment [gcp|rabbit]")
	flag.Parse()
	if envFlag == "" {
		log.Fatal("missing -env flag\n")
	}
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))
}
