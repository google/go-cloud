package pubsub_test

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
	if !commandExists("pub") {
		t.Skip("pub command not found")
	}
	if !commandExists("sub") {
		t.Skip("sub command not found")
	}
	envs := strings.Split(*envsFlag, ",")
	topic := *topicFlag
	sub := *subFlag
	for _, env := range envs {
		t.Run(env, func(t *testing.T) {
			msgs := []string{"alice", "bob"}
			for _, msg := range msgs {
				if _, err := runWithInput(msg, "pub", "-env", env, topic); err != nil {
					t.Fatal(err)
				}
			}
			n := fmt.Sprintf("%d", len(msgs))
			recvOut, err := run("sub", "-env", env, "-n", n, sub)
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

func run(name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("running %v: %s", c.Args, out)
	}
	return string(out), nil
}

func runWithInput(input, name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	inPipe, err := c.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("opening pipe to stdin for %v: %v", c.Args, err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer inPipe.Close()
		inPipe.Write([]byte(input))
	}()
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("running %v: %s", c.Args, out)
	}
	wg.Wait()
	return string(out), nil
}

func commandExists(name string) bool {
	c := exec.Command(name)
	err := c.Start()
	return err == nil
}
