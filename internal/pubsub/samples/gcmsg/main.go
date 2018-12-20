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

// gcmsg is a sample application that publishes messages from stdin to an
// existing topic or receives messages from an existing subscription and
// sends them to stdout. The name gcmsg is short for Go Cloud Messages.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/google/subcommands"
	"gocloud.dev/internal/pubsub"
)

type pubCmd struct {
	cf *cliFlags
}

func (*pubCmd) Name() string     { return "pub" }
func (*pubCmd) Synopsis() string { return "Publish a message to a topic" }
func (*pubCmd) Usage() string {
	return `pub <topic>:
  Read messages from stdin, one per line and send them to <topic>.
`
}

func (p *pubCmd) SetFlags(f *flag.FlagSet) {
}

func (p *pubCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	topicID := f.Arg(0)
	if err := p.pub(ctx, topicID); err != nil {
		log.Print(err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

func (p *pubCmd) pub(ctx context.Context, topicID string) error {
	// Open a subscription in the specified environment.
	var (
		topic   *pubsub.Topic
		cleanup func()
		err     error
	)
	switch p.cf.env {
	case "gcp":
		topic, cleanup, err = openGCPTopic(ctx, topicID)
	case "rabbit":
		topic, cleanup, err = openRabbitTopic(topicID, p.cf.rabbitURL)
	default:
		return fmt.Errorf("unrecognized -env: %s", p.cf.env)
	}
	if err != nil {
		return err
	}
	defer cleanup()

	// Get lines from stdin and send them as messages to the topic.
	fmt.Fprintf(os.Stderr, "Enter messages, one per line to be published to \"%s\".\n", topicID)
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

type subCmd struct {
	n  int
	cf *cliFlags
}

func (*subCmd) Name() string     { return "sub" }
func (*subCmd) Synopsis() string { return "Receive messages from a subscription" }
func (*subCmd) Usage() string {
	return `sub [-n N] <subscription>:
  Receive messages from <subscription> and send them to stdout, one per line.
`
}

func (s *subCmd) SetFlags(f *flag.FlagSet) {
	flag.IntVar(&s.n, "n", 0, "number of messages to receive, or 0 for unlimited")
}

func (s *subCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	subscriptionID := f.Arg(0)
	if err := s.sub(ctx, subscriptionID); err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

func (s *subCmd) sub(ctx context.Context, subID string) error {
	// Open a subscription in the specified environment.
	var (
		sub     *pubsub.Subscription
		cleanup func()
		err     error
	)
	switch s.cf.env {
	case "gcp":
		sub, cleanup, err = openGCPSubscription(ctx, subID)
	case "rabbit":
		sub, cleanup, err = openRabbitSubscription(subID, s.cf.rabbitURL)
	default:
		log.Fatalf("unrecognized -env: %q", s.cf.env)
	}
	if err != nil {
		return err
	}
	defer cleanup()

	// Receive messages and send them to stdout.
	for i := 0; s.n == 0 || i < s.n; i++ {
		m, err := sub.Receive(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", m.Body)
		m.Ack()
	}
	return nil
}

type cliFlags struct {
	env       string
	rabbitURL string
}

func main() {
	var cf cliFlags
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(&pubCmd{&cf}, "")
	subcommands.Register(&subCmd{0, &cf}, "")
	log.SetFlags(0)
	log.SetPrefix("gcmsg: ")
	flag.StringVar(&cf.env, "env", "", "environment [gcp|rabbit]")
	flag.StringVar(&cf.rabbitURL, "rabbiturl", "amqp://guest:guest@localhost:5672/", "URL of server if using RabbitMQ")
	flag.Parse()
	if cf.env == "" {
		log.Fatal("missing -env flag\n")
	}
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))
}
