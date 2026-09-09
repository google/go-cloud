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
//
//	go test  -json ./... | test-summary
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	verbose = flag.Bool("verbose", false, "display all test output")
)

// TestEvent is copied from "go doc test2json".
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

	// Collects tests that failed.
	var failedTests []string

	// Stores output produced by each test.
	testOutputs := map[string][]string{}

	start := time.Now()
	for scanner.Scan() {
		// When the build fails, go test -json doesn't emit a valid JSON value, only
		// a line of output starting with FAIL. Report a more reasonable error in
		// this case.
		if strings.HasPrefix(scanner.Text(), "FAIL") {
			fmt.Println(scanner.Text())
			continue
		}

		var event TestEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return "", false, fmt.Errorf("%q: %w", scanner.Text(), err)
		}

		// The Test field, if non-empty, specifies the test, example, or benchmark
		// function that caused the event. Events for the overall package test do
		// not set Test.
		testpath := filepath.Join(event.Package, event.Test)
		testName := event.Test
		isPackageLevel := testName == ""
		counts[event.Action]++
		actionMsg := fmt.Sprintf("%s %s (%.2fs)", event.Action, testpath, event.Elapsed)
		switch event.Action {
		case "fail":
			fmt.Println(actionMsg)
			// For failed tests, print all the output we collected for them before
			// the "fail" event.
			if !isPackageLevel && !*verbose {
				fmt.Println("\nSTART of failed test output")
				fmt.Println("  " + strings.Join(testOutputs[testpath], "  "))
				fmt.Println("END of failed test output")
				fmt.Println()
			}
			failedTests = append(failedTests, testpath)
		case "pass":
			// Only print package-level pass actions.
			if isPackageLevel {
				fmt.Println(actionMsg)
			}
		case "output":
			if *verbose {
				fmt.Print(event.Output)
			}
			testOutputs[testpath] = append(testOutputs[testpath], event.Output)
		}

	}
	if err := scanner.Err(); err != nil {
		return "", false, err
	}
	p := counts["pass"]
	f := counts["fail"]
	s := counts["skip"]

	summary := fmt.Sprintf("ran %d; passed %d; failed %d; skipped %d (in %.1f sec)", p+f+s, p, f, s, time.Since(start).Seconds())
	if len(failedTests) > 0 {
		var sb strings.Builder
		sb.WriteString("\nFailures (reporting up to 10):\n")
		for i := 0; i < len(failedTests) && i < 10; i++ {
			fmt.Fprintf(&sb, "  %s\n", failedTests[i])
		}
		if len(failedTests) > 10 {
			sb.WriteString("  ...\n")
		}
		sb.WriteString(summary)
		summary = sb.String()
	}

	return summary, f > 0, nil
}
