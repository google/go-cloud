// Copyright 2019 The Go Cloud Development Kit Authors
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

// gocdk-docstore demonstrates the use of the Go CDK docstore package in a
// simple command-line application.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/google/subcommands"
	"gocloud.dev/internal/docstore"

	// Import the docstore driver packages we want to be able to open.
	_ "gocloud.dev/internal/docstore/dynamodocstore"
	_ "gocloud.dev/internal/docstore/firedocstore"
	_ "gocloud.dev/internal/docstore/memdocstore"
	_ "gocloud.dev/internal/docstore/mongodocstore"
)

const helpSuffix = `

  See https://gocloud.dev/concepts/urls/ for more background on
  Go CDK URLs, and sub-packages under gocloud.dev/internal/docstore
  (https://godoc.org/gocloud.dev/internal/docstore#pkg-subdirectories)
  for details on the docstore.Collection URL format.
`

func main() {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(&listCmd{}, "")
	subcommands.Register(&putCmd{}, "")
	subcommands.Register(&deleteCmd{}, "")
	log.SetFlags(0)
	log.SetPrefix("gocdk-docstore: ")
	flag.Parse()
	os.Exit(int(subcommands.Execute(context.Background())))
}

type Message struct {
	Date             string
	Timestamp        string // unique ID of each message
	Content          string
	DocstoreRevision interface{}
}

func (m Message) String() string {
	return fmt.Sprintf("%s: %s", m.Timestamp, m.Content)
}

type listCmd struct {
	date string
}

func (*listCmd) Name() string     { return "ls" }
func (*listCmd) Synopsis() string { return "List items in a collection" }
func (*listCmd) Usage() string {
	return `ls [-d <date>] <collection URL>

  List the docstores in <collection URL>.

  Example:
    gocdk-docstore ls -d "2006-01-02" firestore://mycollection` + helpSuffix
}

func (cmd *listCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&cmd.date, "d", "", "get the messages from this date, in the format YYYY-MM-DD")
}

func (cmd *listCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	collectionURL := f.Arg(0)

	// Open a *docstore.Collection using the collectionURL.
	collection, err := docstore.OpenCollection(ctx, collectionURL)
	if err != nil {
		log.Printf("Failed to open collection: %v\n", err)
		return subcommands.ExitFailure
	}

	q := collection.Query()
	if cmd.date != "" {
		q = q.Where("Date", "=", cmd.date)
	}
	iter := q.Get(ctx)
	for {
		var msg Message
		err := iter.Next(ctx, &msg)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Failed to list: %v", err)
			return subcommands.ExitFailure
		}
		fmt.Println(msg)
	}
	return subcommands.ExitSuccess
}

type putCmd struct{}

func (*putCmd) Name() string     { return "put" }
func (*putCmd) Synopsis() string { return "Put an item from stdin" }
func (*putCmd) Usage() string {
	return `put <collection URL> <message>

  Read from stdin and put an message with the current timestamp in <collection URL>.

  Example:
    gocdk-docstore put firestore://mycollection "hello docstore` + helpSuffix
}

func (*putCmd) SetFlags(_ *flag.FlagSet) {}

func (*putCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 2 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	collectionURL := f.Arg(0)
	content := f.Arg(1)

	// Open a *docstore.Collection using the collectionURL.
	collection, err := docstore.OpenCollection(ctx, collectionURL)
	if err != nil {
		log.Printf("Failed to open collection: %v\n", err)
		return subcommands.ExitFailure
	}

	t := time.Now()
	if err := collection.Put(ctx, &Message{
		Date:      t.Format("2006-01-02"),
		Timestamp: t.Format(time.RFC3339Nano),
		Content:   content,
	}); err != nil {
		log.Printf("Failed to put message: %v\n", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

type deleteCmd struct {
	date string
}

func (*deleteCmd) Name() string     { return "delete" }
func (*deleteCmd) Synopsis() string { return "Delete items in a collection" }
func (*deleteCmd) Usage() string {
	return `delete [-d <date>] <collection URL>

  Delete the docstores in <collection URL>.

  Example:
    gocdk-docstore delete -d "2006-01-02" firestore://mycollection` + helpSuffix
}

func (cmd *deleteCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&cmd.date, "d", "", "delete the messages from this date, in the format YYYY-MM-DD")
}

func (cmd *deleteCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	collectionURL := f.Arg(0)

	// Open a *docstore.Collection using the collectionURL.
	collection, err := docstore.OpenCollection(ctx, collectionURL)
	if err != nil {
		log.Printf("Failed to open collection: %v\n", err)
		return subcommands.ExitFailure
	}

	q := collection.Query()
	if cmd.date != "" {
		q = q.Where("Date", "=", cmd.date)
	}
	if err := q.Delete(ctx); err != nil {
		log.Printf("Failed to delete: %v", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}
