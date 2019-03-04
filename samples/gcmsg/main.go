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

// gcmsg is a sample application that publishes messages from stdin to an
// existing topic or receives messages from an existing subscription and
// sends them to stdout. The name gcmsg is short for Go CDK Messages.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/google/subcommands"
	"gocloud.dev/pubsub"
)

type pubCmd struct{}

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
	topicURL := f.Arg(0)
	if err := p.pub(ctx, topicURL, os.Stdin); err != nil {
		log.Print(err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

func (p *pubCmd) pub(ctx context.Context, topicURL string, r io.Reader) error {
	u, err := parseTopicURL(topicURL)
	if err != nil {
		return fmt.Errorf("parsing topic URL: %v", err)
	}
	t, cleanup, err := openTopic(ctx, u)
	if err != nil {
		return fmt.Errorf("opening topic: %v", err)
	}
	defer cleanup()

	// Get lines from r and send them as messages to the topic.
	fmt.Fprintf(os.Stderr, "Enter messages, one per line to be published to \"%s\".\n", topicURL)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" && u.Provider == "gcp" {
			log.Printf("skipping empty message for %s", u.Provider)
			continue
		}
		m := &pubsub.Message{Body: []byte(line)}
		if err := t.Send(ctx, m); err != nil {
			return err
		}
	}
	return scanner.Err()
}

type subCmd struct {
	n int
}

func (*subCmd) Name() string     { return "sub" }
func (*subCmd) Synopsis() string { return "Receive messages from a subscription" }
func (*subCmd) Usage() string {
	return `sub [-n N] <subscription>:
  Receive messages from <subscription> and send them to stdout, one per line.
`
}

func (s *subCmd) SetFlags(f *flag.FlagSet) {
	f.IntVar(&s.n, "n", 0, "number of messages to receive, or 0 for unlimited")
}

func (s *subCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	subURL := f.Arg(0)
	if err := s.sub(ctx, subURL, os.Stdout); err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

func (s *subCmd) sub(ctx context.Context, subURL string, w io.Writer) error {
	u, err := parseSubscriptionURL(subURL)
	if err != nil {
		return fmt.Errorf("parsing subscription URL: %v", err)
	}
	sub, cleanup, err := openSubscription(ctx, u)
	if err != nil {
		return fmt.Errorf("opening subscription: %v", err)
	}
	defer cleanup()

	// Receive messages and send them to w.
	for i := 0; s.n == 0 || i < s.n; i++ {
		m, err := sub.Receive(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s\n", m.Body)
		m.Ack()
	}
	return nil
}

func main() {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(&pubCmd{}, "")
	subcommands.Register(&subCmd{}, "")
	log.SetFlags(0)
	log.SetPrefix("gcmsg: ")
	flag.Parse()
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))
}
