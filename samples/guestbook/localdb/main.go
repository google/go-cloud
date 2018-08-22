// Copyright 2018 Google LLC
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
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	pipe "gopkg.in/pipe.v2"
)

func main() {
	guestbookDir := flag.String("guestbook_dir", ".", "directory containing guestbook sample source code")
	flag.Parse()
	if flag.NArg() > 1 {
		fmt.Fprintf(os.Stderr, "usage: localdb [flags] container_name\n")
		os.Exit(1)
	}
	log.SetPrefix("localdb: ")
	log.SetFlags(0)
	if err := runLocalDB(flag.Arg(0), *guestbookDir); err != nil {
		log.Fatal(err)
	}
}

func runLocalDB(containerName, guestbookDir string) error {
	image := "mysql:5.6"

	log.Printf("Starting container running MySQL")
	dockerArgs := []string{"run", "--rm"}
	if containerName != "" {
		dockerArgs = append(dockerArgs, "--name", containerName)
	}
	dockerArgs = append(dockerArgs,
		"--env", "MYSQL_DATABASE=guestbook",
		"--env", "MYSQL_ROOT_PASSWORD=password",
		"--detach",
		"--publish", "3306:3306",
		image)
	cmd := exec.Command("docker", dockerArgs...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("running %v: %v: %s", cmd.Args, err, out)
	}
	containerID := strings.TrimSpace(string(out))
	defer func() {
		log.Printf("killing %s", containerID)
		stop := exec.Command("docker", "kill", containerID)
		stop.Stderr = os.Stderr
		if err := stop.Run(); err != nil {
			log.Printf("failed to kill db container: %v", err)
		}
	}()

	// Stop the container on Ctrl-C.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		c := make(chan os.Signal, 1)
		// TODO(ijt): Handle SIGTERM.
		signal.Notify(c, os.Interrupt)
		<-c
		cancel()
	}()

	nap := 10 * time.Second
	log.Printf("Waiting %v for database to come up", nap)
	select {
	case <-time.After(nap):
		// ok
	case <-ctx.Done():
		return errors.New("interrupted while napping")
	}

	log.Printf("Initializing database schema and users")
	schema, err := ioutil.ReadFile(filepath.Join(guestbookDir, "schema.sql"))
	if err != nil {
		return fmt.Errorf("reading schema: %v", err)
	}
	roles, err := ioutil.ReadFile(filepath.Join(guestbookDir, "roles.sql"))
	if err != nil {
		return fmt.Errorf("reading roles: %v", err)
	}
	tooMany := 10
	var i int
	for i = 0; i < tooMany; i++ {
		mySQL := `mysql -h"${MYSQL_PORT_3306_TCP_ADDR?}" -P"${MYSQL_PORT_3306_TCP_PORT?}" -uroot -ppassword guestbook`
		p := pipe.Line(
			pipe.Read(strings.NewReader(string(schema)+string(roles))),
			pipe.Exec("docker", "run", "--rm", "--interactive", "--link", containerID+":mysql", image, "sh", "-c", mySQL),
		)
		if _, stderr, err := pipe.DividedOutput(p); err != nil {
			log.Printf("Failed to seed database: %q; retrying", stderr)
			select {
			case <-time.After(time.Second):
				continue
			case <-ctx.Done():
				return errors.New("interrupted while napping in between database seeding attempts")
			}
		}
		break
	}
	if i == tooMany {
		return fmt.Errorf("gave up after %d tries to seed database", i)
	}

	log.Printf("Database running at localhost:3306")
	attach := exec.CommandContext(ctx, "docker", "attach", containerID)
	attach.Stdout = os.Stdout
	attach.Stderr = os.Stderr
	if err := attach.Run(); err != nil {
		return fmt.Errorf("running %v: %q", attach.Args, err)
	}

	return nil
}
