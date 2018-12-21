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

var topicFlag = flag.String("topic", "test-topic", "topic to be used for testing pub and sub commands")
var subFlag = flag.String("sub", "test-subscription-1", "subscription to be used for testing pub and sub commands")
var envsFlag = flag.String("pubsub_envs", "gcp,rabbit", "environments in which to run the test")

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func TestPubAndSubCommands(t *testing.T) {
	if !commandExists("gcmsg") {
		t.Skip("gcmsg command not found")
	}
	envs := strings.Split(*envsFlag, ",")
	topic := *topicFlag
	sub := *subFlag
	for _, env := range envs {
		t.Run(env, func(t *testing.T) {
			msgs := []string{"alice", "bob"}
			for _, msg := range msgs {
				c := cmd{name: "gcmsg", args: []string{"-env", env, "pub", topic}}
				if _, err := c.runWithInput(msg); err != nil {
					t.Fatal(err)
				}
			}
			n := fmt.Sprintf("%d", len(msgs))
			c := cmd{name: "gcmsg", args: []string{"-env", env, "sub", "-n", n, sub}}
			recvOut, err := c.run()
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(recvOut), "\n")
			sort.Strings(lines)
			if diff := cmp.Diff(lines, msgs); diff != "" {
				t.Error(diff)
			}
		})
	}
}

type cmd struct {
	name string
	args []string
}

func (c *cmd) run() (string, error) {
	c2 := exec.Command(c.name, c.args...)
	out, err := c2.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("running %v: %s", c2.Args, out)
	}
	return string(out), nil
}

func (c *cmd) runWithInput(input string) (string, error) {
	c2 := exec.Command(c.name, c.args...)
	inPipe, err := c2.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("opening pipe to stdin for %v: %v", c2.Args, err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer inPipe.Close()
		inPipe.Write([]byte(input))
	}()
	out, err := c2.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("running %v: %s", c2.Args, out)
	}
	wg.Wait()
	return string(out), nil
}

func commandExists(name string) bool {
	c := exec.Command(name)
	err := c.Start()
	return err == nil
}
