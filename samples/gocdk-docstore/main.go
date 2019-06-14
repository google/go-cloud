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
	"gocloud.dev/docstore"

	// Import the docstore driver packages we want to be able to open.
	_ "gocloud.dev/docstore/dynamodocstore"
	_ "gocloud.dev/docstore/firedocstore"
	_ "gocloud.dev/docstore/memdocstore"
	_ "gocloud.dev/docstore/mongodocstore"
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
	subcommands.Register(&updateCmd{}, "")
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

  List the documents in <collection URL>.

  Example:
    gocdk-docstore ls -d "2006-01-02" "firestore://projects/myproject/databases/(default)/documents/mycollection?name_field=myID"` + helpSuffix
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
	defer collection.Close()

	q := collection.Query()
	if cmd.date != "" {
		q = q.Where("Date", "=", cmd.date)
	}
	iter := q.Get(ctx)
	defer iter.Stop()
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
    gocdk-docstore put "firestore://projects/myproject/databases/(default)/documents/mycollection?name_field=myID" "hello docstore` + helpSuffix
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
		log.Printf("Failed to open collection: %v", err)
		return subcommands.ExitFailure
	}
	defer collection.Close()

	t := time.Now()
	if err := collection.Put(ctx, &Message{
		Date:      t.Format("2006-01-02"),
		Timestamp: t.Format(time.RFC3339),
		Content:   content,
	}); err != nil {
		log.Printf("Failed to put message: %v", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

type updateCmd struct{}

func (*updateCmd) Name() string     { return "update" }
func (*updateCmd) Synopsis() string { return "Update an item in a collection" }
func (*updateCmd) Usage() string {
	return `update <timestamp> <collection URL> <updated message>

  Update the document at <timestamp> in <collection URL>.

  Example:
    gocdk-docstore update "2019-06-13T15:57:59.186915-07:00" "firestore://projects/myproject/databases/(default)/documents/mycollection?name_field=myID" "hello again"` + helpSuffix
}

func (*updateCmd) SetFlags(_ *flag.FlagSet) {}

func (cmd *updateCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 3 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	timestamp := f.Arg(0)
	collectionURL := f.Arg(1)
	updated := f.Arg(2)

	// Open a *docstore.Collection using the collectionURL.
	collection, err := docstore.OpenCollection(ctx, collectionURL)
	if err != nil {
		log.Printf("Failed to open collection: %v\n", err)
		return subcommands.ExitFailure
	}
	defer collection.Close()

	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		log.Fatal("wrong timestamp passed in, use RFC3339 formact, e.g.", time.Now().Format(time.RFC3339))
	}

	msg := &Message{
		Date:      t.Format("2006-01-02"),
		Timestamp: timestamp,
	}
	if errs := collection.Actions().Update(msg, docstore.Mods{
		"Content": updated,
	}).Get(msg).Do(ctx); errs != nil {
		log.Printf("Failed to update message: %v", errs)
		return subcommands.ExitFailure
	}
	log.Println("updated:", msg.Timestamp, msg.Content)
	return subcommands.ExitSuccess
}

type deleteCmd struct {
	date string
}

func (*deleteCmd) Name() string     { return "delete" }
func (*deleteCmd) Synopsis() string { return "Delete items in a collection" }
func (*deleteCmd) Usage() string {
	return `delete [-d <date>] <collection URL>

  Delete the documents in <collection URL>.

  Example:
    gocdk-docstore delete -d "2006-01-02" "firestore://projects/myproject/databases/(default)/documents/mycollection?name_field=myID"` + helpSuffix
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
	defer collection.Close()

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
