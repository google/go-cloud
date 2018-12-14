package pubsub_test

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
)

func TestPubAndSubCommands(t *testing.T) {
	if !commandExists("pub") {
		t.Skip("pub command not found")
	}
	if !commandExists("sub") {
		t.Skip("sub command not found")
	}
	for _, env := range []string{"gcp", "rabbit"} {
		t.Run(env, func(t *testing.T) {
			topic := "pub-and-sub-test-topic"
			if _, err := run("pub", "-env", env, "new", topic); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := run("pub", "-env", env, "del", topic); err != nil {
					t.Fatal(err)
				}
			}()
			sub := "pub-and-sub-test-subscription"
			if _, err := run("sub", "-env", env, "new", topic, sub); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := run("sub", "-env", env, "del", sub); err != nil {
					t.Fatal(err)
				}
			}()
			msgs := []string{"alice", "bob"}
			for _, msg := range msgs {
				if _, err := runWithInput(msg, "pub", "-env", env, "send", topic); err != nil {
					t.Fatal(err)
				}
			}
			n := fmt.Sprintf("%d", len(msgs))
			recvOut, err := run("sub", "-env", env, "recv", "-n", n, sub)
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
	var g errgroup.Group
	g.Go(func() error {
		defer inPipe.Close()
		_, err = inPipe.Write([]byte(input))
		return err
	})
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("running %v: %s", c.Args, out)
	}
	if err = g.Wait(); err != nil {
		return "", err
	}
	return string(out), nil
}

func commandExists(name string) bool {
	c := exec.Command(name)
	err := c.Start()
	return err == nil
}
