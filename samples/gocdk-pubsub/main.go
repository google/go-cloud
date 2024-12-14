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

// gocdk-pubsub demonstrates the use of the Go CDK pubsub package in a
// simple command-line application.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/google/subcommands"
	"gocloud.dev/pubsub"

	// Import the pubsub driver packages we want to be able to open.
	_ "gocloud.dev/pubsub/awssnssqs"
	_ "gocloud.dev/pubsub/azuresb"
	_ "gocloud.dev/pubsub/gcppubsub"
	_ "gocloud.dev/pubsub/kafkapubsub"
	_ "gocloud.dev/pubsub/natspubsub"
	_ "gocloud.dev/pubsub/rabbitpubsub"
)

const helpSuffix = `

  See https://gocloud.dev/concepts/urls/ for more background on
  Go CDK URLs, and sub-packages under gocloud.dev/pubsub
  (https://godoc.org/gocloud.dev/pubsub#pkg-subdirectories)
  for details on the topic/subscription URL format.
`

func main() {
	os.Exit(run())
}

func run() int {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(&pubCmd{}, "")
	subcommands.Register(&subCmd{}, "")
	log.SetFlags(0)
	log.SetPrefix("gocdk-pubsub: ")
	flag.Parse()
	return int(subcommands.Execute(context.Background()))
}

type pubCmd struct{}

func (*pubCmd) Name() string     { return "pub" }
func (*pubCmd) Synopsis() string { return "Publish a message to a topic" }
func (*pubCmd) Usage() string {
	return `pub <topic URL>

  Read messages from stdin, one per line and send them to <topic URL>.

  Example:
    gocdk-pubsub pub gcppubsub://myproject/mytopic` + helpSuffix
}

func (*pubCmd) SetFlags(_ *flag.FlagSet) {}

func (*pubCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	topicURL := f.Arg(0)

	// Open a *pubsub.Topic using the URL.
	topic, err := pubsub.OpenTopic(ctx, topicURL)
	if err != nil {
		log.Print(err)
		return subcommands.ExitFailure
	}
	defer topic.Shutdown(ctx)

	// Read lines from stdin and send them as messages to the topic.
	fmt.Fprintf(os.Stderr, "Enter messages, one per line, to be published to %q.\n", topicURL)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			log.Print("Skipping empty message.")
			continue
		}
		m := &pubsub.Message{Body: []byte(line)}
		if err := topic.Send(ctx, m); err != nil {
			log.Print(err)
			return subcommands.ExitFailure
		}
	}
	if err := scanner.Err(); err != nil {
		log.Print(err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

type subCmd struct {
	n int // number of messages to receive, or 0 for infinite
}

func (*subCmd) Name() string     { return "sub" }
func (*subCmd) Synopsis() string { return "Receive messages from a subscription" }
func (*subCmd) Usage() string {
	return `sub [-n N] <subscription URL>

  Receive messages from <subscription URL> and send them to stdout, one per line.

  Example:
    gocdk-pubsub sub gcppubsub://myproject/mytopic` + helpSuffix
}

func (cmd *subCmd) SetFlags(f *flag.FlagSet) {
	f.IntVar(&cmd.n, "n", 0, "number of messages to receive, or 0 for unlimited")
}

func (cmd *subCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	subURL := f.Arg(0)

	// Open a *pubsub.Subscription using the URL.
	sub, err := pubsub.OpenSubscription(ctx, subURL)
	if err != nil {
		log.Print(err)
		return subcommands.ExitFailure
	}
	defer sub.Shutdown(ctx)

	// Receive messages from the subscription and print them to stdout.
	fmt.Printf("Receiving messages from %q...\n", subURL)
	for i := 0; cmd.n == 0 || i < cmd.n; i++ {
		m, err := sub.Receive(ctx)
		if err != nil {
			log.Print(err)
			return subcommands.ExitFailure
		}
		fmt.Printf("%s\n", m.Body)
		m.Ack()
	}
	return subcommands.ExitSuccess
}
