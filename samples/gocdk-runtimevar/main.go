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

// gocdk-runtimevar demonstrates the use of the Go CDK runtimevar package in a
// simple command-line application.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/subcommands"
	"gocloud.dev/runtimevar"

	// Import the runtimevar driver packages we want to be able to open.
	_ "gocloud.dev/runtimevar/awsparamstore"
	_ "gocloud.dev/runtimevar/blobvar"
	_ "gocloud.dev/runtimevar/constantvar"
	_ "gocloud.dev/runtimevar/filevar"
	_ "gocloud.dev/runtimevar/gcpruntimeconfig"
	_ "gocloud.dev/runtimevar/httpvar"
)

const helpSuffix = `

  See https://gocloud.dev/concepts/urls/ for more background on
  Go CDK URLs, and sub-packages under gocloud.dev/runtimevar
  (https://godoc.org/gocloud.dev/runtimevar#pkg-subdirectories)
  for details on the runtimevar.Variable URL format.
`

func main() {
	os.Exit(run(context.Background()))
}

func run(ctx context.Context) int {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(&catCmd{}, "")
	subcommands.Register(&watchCmd{}, "")
	log.SetFlags(0)
	log.SetPrefix("gocdk-runtimevar: ")
	flag.Parse()
	return int(subcommands.Execute(ctx))
}

type catCmd struct{}

func (*catCmd) Name() string     { return "cat" }
func (*catCmd) Synopsis() string { return "Print a variable's value to stdout" }
func (*catCmd) Usage() string {
	return `cat <variable URL>

  Read the current value of the variable from <variable URL> and print it to stdout.

  Example:
    gocdk-runtimevar cat "constant://?val=foo&decoder=string"` + helpSuffix
}

func (*catCmd) SetFlags(_ *flag.FlagSet) {}

func (*catCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	variableURL := f.Arg(0)

	// Open a *runtimevar.Variable using the variableURL.
	variable, err := runtimevar.OpenVariable(ctx, variableURL)
	if err != nil {
		log.Printf("Failed to open variable: %v\n", err)
		return subcommands.ExitFailure
	}
	defer variable.Close()

	snapshot, err := variable.Latest(ctx)
	if err != nil {
		log.Printf("Failed to read variable value: %v\n", err)
		return subcommands.ExitFailure
	}
	fmt.Printf("(%T) %v\n", snapshot.Value, snapshot.Value)
	return subcommands.ExitSuccess
}

type watchCmd struct{}

func (*watchCmd) Name() string     { return "watch" }
func (*watchCmd) Synopsis() string { return "Watch a variable's value and print changes to stdout" }
func (*watchCmd) Usage() string {
	return `watch <variable URL>

  Read the value of the variable from <variable URL> and print changes to stdout.

  Example:
    gocdk-runtimevar watch "constant://?val=foo&decoder=string"` + helpSuffix
}

func (*watchCmd) SetFlags(_ *flag.FlagSet) {}

func (*watchCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if f.NArg() != 1 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	variableURL := f.Arg(0)

	// Open a *runtimevar.Variable using the variableURL.
	variable, err := runtimevar.OpenVariable(ctx, variableURL)
	if err != nil {
		log.Printf("Failed to open variable: %v\n", err)
		return subcommands.ExitFailure
	}
	defer variable.Close()

	fmt.Fprintf(os.Stderr, "Watching %s for changes...\n\n", variableURL)
	time.Sleep(250 * time.Millisecond) // to ensure deterministic combined output for test
	for {
		snapshot, err := variable.Watch(ctx)
		if err != nil {
			if err == context.Canceled {
				return subcommands.ExitSuccess
			}
			fmt.Printf("(error) %v\n", err)
			continue
		}
		fmt.Printf("(%T) %[1]v\n", snapshot.Value)
	}
	return subcommands.ExitSuccess
}
