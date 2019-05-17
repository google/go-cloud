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

// Summarizes the output of go test.
// Run like so:
//    go test  -json ./... | test-summary
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

var (
	progress = flag.Bool("progress", false, "display test progress")
	verbose  = flag.Bool("verbose", false, "display all test output")
)

// From running "go doc test2json".
type TestEvent struct {
	Time    time.Time // encodes as an RFC3339-format string
	Action  string
	Package string
	Test    string
	Elapsed float64 // seconds
	Output  string
}

func main() {
	flag.Parse()
	s, fails, err := run(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(s)
	if fails {
		os.Exit(1)
	}
}

func run(r io.Reader) (msg string, failures bool, err error) {
	counts := map[string]int{}
	scanner := bufio.NewScanner(bufio.NewReader(r))
	prevPkg := "" // In progress mode, the package we previously wrote, to avoid repeating it.
	for scanner.Scan() {
		var event TestEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return "", false, fmt.Errorf("%q: %v", scanner.Text(), err)
		}
		if *verbose && event.Action == "output" {
			fmt.Print(event.Output)
		}
		// Ignore pass or fail events that don't have a Test; they refer to the
		// package as a whole, and we would be over-counting if we included them.
		// However, skips of an entire package are not duplicated with individual
		// test skips.
		if event.Test == "" && (event.Action == "pass" || event.Action == "fail") {
			continue
		}
		counts[event.Action]++
		if *progress && (event.Action == "pass" || event.Action == "fail" || event.Action == "skip") {
			if event.Package == prevPkg {
				fmt.Printf("%s     %s (%.2fs)\n", event.Action, event.Test, event.Elapsed)
			} else {
				path := event.Package
				if event.Test != "" {
					path += "/" + event.Test
				}
				fmt.Printf("%s %s (%.2fs)\n", event.Action, path, event.Elapsed)
				prevPkg = event.Package
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", false, err
	}
	p := counts["pass"]
	f := counts["fail"]
	s := counts["skip"]
	return fmt.Sprintf("ran %d; passed %d; failed %d; skipped %d", p+f+s, p, f, s), f > 0, nil
}
