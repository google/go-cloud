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

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// -topics and -subs are parallel arrays of URLs* that specify topics and corresponding subscriptions.
//
// *in an extended sense, bringing URLs closer to being really universal.
var topicsFlag = flag.String("topics", "gcppubsub://projects/go-cloud-test-216917/topics/test-topic,rabbitpubsub://guest:guest@localhost:5672/topics/test-topic", "comma-separated URLs for topics")
var subsFlag = flag.String("subs", "gcppubsub://projects/go-cloud-test-216917/subscriptions/test-subscription-1,rabbitpubsub://guest:guest@localhost:5672/subscriptions/test-subscription-1", "comma-separated URLs for subscriptions")

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func TestPubAndSubCommands(t *testing.T) {
	// TODO: Handle possible unavailability of GCP pubsub or rabbit on Travis.
	// Maybe use record/replay.
	topics := strings.Split(*topicsFlag, ",")
	subs := strings.Split(*subsFlag, ",")
	if len(subs) != len(topics) {
		t.Fatalf("got %d items in -subs flag and %d items in -topics flag, want them to be the same", len(subs), len(topics))
	}
	if len(topics) == 0 {
		t.Fatalf("empty string specified for -topics flag")
	}
	for i := range topics {
		topic := topics[i]
		subURL := subs[i]
		testName := fmt.Sprintf("%s+%s", topic, subURL)
		t.Run(testName, func(t *testing.T) {
			msgs := []string{"alice", "bob"}
			for _, msg := range msgs {
				pc := pubCmd{}
				r := strings.NewReader(msg)
				ctx := context.Background()
				if err := pc.pub(ctx, topic, r); err != nil {
					t.Fatal(err)
				}
			}
			sc := subCmd{n: len(msgs)}
			var buf bytes.Buffer
			ctx := context.Background()
			if err := sc.sub(ctx, subURL, &buf); err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
			sort.Strings(lines)
			if diff := cmp.Diff(lines, msgs); diff != "" {
				t.Error(diff)
			}
		})
	}
}
