// pub is a sample application that publishes messages from stdin to an
// existing topic.
package main

import (
	"bufio"
	"context"
	"flag"
	"log"
	"os"

	"github.com/google/subcommands"
	"gocloud.dev/internal/pubsub"
)

type newCmd struct{}

func (*newCmd) Name() string     { return "new" }
func (*newCmd) Synopsis() string { return "Create a new topic" }
func (*newCmd) Usage() string {
	return `new <topic>:
	Create a new topic.
`
}

func (*newCmd) SetFlags(f *flag.FlagSet) {
}

func (c *newCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	topicID := f.Arg(0)
	ctx := context.Background()
	var err error
	switch envFlag {
	case "gcp":
		err = makeGCPTopic(ctx, topicID)
	case "rabbit":
		err = makeRabbitTopic(topicID)
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
func (*delCmd) Synopsis() string { return "Delete a topic" }
func (*delCmd) Usage() string {
	return `del <topic>:
	Delete a topic.
`
}

func (*delCmd) SetFlags(f *flag.FlagSet) {
}

func (c *delCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	topicID := f.Arg(0)
	ctx := context.Background()
	var err error
	switch envFlag {
	case "gcp":
		err = deleteGCPTopic(ctx, topicID)
	case "rabbit":
		err = deleteRabbitTopic(topicID)
	default:
		log.Fatalf("unrecognized -env: %s", envFlag)
	}
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

type sendCmd struct{}

func (*sendCmd) Name() string     { return "send" }
func (*sendCmd) Synopsis() string { return "Send a message to a topic" }
func (*sendCmd) Usage() string {
	return `send <topic>
	Send a messages to a topic, one message per line from stdin.
`
}

func (*sendCmd) SetFlags(f *flag.FlagSet) {
}

func (c *sendCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	topicID := f.Arg(0)

	// Open a subscription in the specified environment.
	ctx := context.Background()
	var (
		topic   *pubsub.Topic
		cleanup func()
		err     error
	)
	switch envFlag {
	case "gcp":
		topic, cleanup, err = openGCPTopic(ctx, topicID)
	case "rabbit":
		topic, cleanup, err = openRabbitTopic(topicID)
	default:
		log.Fatalf("unrecognized -env: %s", envFlag)
	}
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}
	defer cleanup()

	// Get lines from stdin and send them as messages to the topic.
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		m := &pubsub.Message{Body: []byte(line)}
		if err := topic.Send(ctx, m); err != nil {
			log.Printf("send failed: %v", err)
			return subcommands.ExitFailure
		}
	}

	return subcommands.ExitSuccess
}

var envFlag string

func main() {
	log.SetFlags(0)
	log.SetPrefix("pub: ")
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(&newCmd{}, "")
	subcommands.Register(&delCmd{}, "")
	subcommands.Register(&sendCmd{}, "")
	flag.StringVar(&envFlag, "env", "", "environment [gcp|rabbit]")
	flag.Parse()
	if envFlag == "" {
		log.Fatal("missing -env flag\n")
	}
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))
}
