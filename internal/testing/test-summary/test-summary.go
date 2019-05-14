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
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

// From runing "go doc test2json".
type TestEvent struct {
	Time    time.Time // encodes as an RFC3339-format string
	Action  string
	Package string
	Test    string
	Elapsed float64 // seconds
	Output  string
}

func main() {
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
	for scanner.Scan() {
		var event TestEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return "", false, fmt.Errorf("%q: %v", scanner.Text(), err)
		}
		counts[event.Action]++
	}
	if err := scanner.Err(); err != nil {
		return "", false, err
	}
	p := counts["pass"]
	f := counts["fail"]
	s := counts["skip"]
	return fmt.Sprintf("ran %d; passed %d; failed %d; skipped %d", p+f+s, p, f, s), f > 0, nil
}
