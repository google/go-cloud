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
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	guestbookDir := flag.String("guestbook_dir", ".", "directory containing guestbook sample source code")
	flag.Parse()
	if flag.NArg() > 1 {
		fmt.Fprintf(os.Stderr, "usage: go run localdb.go [flags] container_name\n")
		os.Exit(1)
	}
	if err := runLocalDb(flag.Arg(0), *guestbookDir); err != nil {
		fmt.Fprintf(os.Stderr, "localdb: %v\n", err)
		os.Exit(1)
	}
}

func runLocalDb(containerName, guestbookDir string) error {
	image := "mysql:5.6"
	netcatImage := "alpine:3.7"

	// Start container
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
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running %v: %v: %s", cmd.Args, err, out)
	}
	containerID := strings.TrimSpace(string(out))
	log.Printf("Started container %s, waiting for healthy", containerID)

	whileLoop := fmt.Sprintf(`while ! nc -z "$MYSQL_PORT_3306_TCP_ADDR:$MYSQL_PORT_3306_TCP_PORT"; do sleep 1; done`)
	waitForHealth := exec.Command("docker", "run", "--rm", "--link", fmt.Sprintf("%s:mysql", containerID), netcatImage, "sh", "-c", whileLoop)
	if out, err := waitForHealth.CombinedOutput(); err != nil {
		stop := exec.Command("docker", "stop", containerID)
		if stopOut, stopErr := stop.CombinedOutput(); err != nil {
			return fmt.Errorf("Failed to stop container while handling error of database port not open: %v: %s", stopErr, stopOut)
		}
		return fmt.Errorf("database port not open; stopped %; details: %s", containerID, out)
	}

	// Initialize database schema and users
	schema, err := ioutil.ReadFile(filepath.Join(guestbookDir, "schema.sql"))
	if err != nil {
		return fmt.Errorf("reading schema: %v", err)
	}
	roles, err := ioutil.ReadFile(filepath.Join(guestbookDir, "roles.sql"))
	if err != nil {
		return fmt.Errorf("reading roles: %v", err)
	}
	mySQL := `exec mysql -h"$MYSQL_PORT_3306_TCP_ADDR" -P"$MYSQL_PORT_3306_TCP_PORT" -uroot -ppassword guestbook`
	dockerMySQL := exec.Command("docker", "run", "--rm", "--interactive", "--link", containerID+":mysql", image, "sh", "-c", mySQL)
	inPipe, err := dockerMySQL.StdinPipe()
	if err != nil {
		return fmt.Errorf("making stdin pipe to docker mysql command: %v", err)
	}
	if _, err := inPipe.Write(schema); err != nil {
		return fmt.Errorf("sending schema to MySQL: %v", err)
	}
	if _, err := inPipe.Write(roles); err != nil {
		return fmt.Errorf("sending roles to MySQL: %v", err)
	}
	if err := dockerMySQL.Start(); err != nil {
		return fmt.Errorf("starting docker mysql command: %v", err)
	}
	if err := dockerMySQL.Wait(); err != nil {
		stop := exec.Command("docker", "stop", containerID)
		if out, err := stop.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to seed database and failed to stop db container: %v: %s", err, out)
		}
		return fmt.Errorf("failed to seed database (%s); stopped %s", containerID)
	}

	log.Printf("Database running at localhost:3306")
	if err := syscall.Exec("docker", []string{"docker", "attach", containerID}, os.Environ()); err != nil {
		return fmt.Errorf("failed to exec docker attach: %v", err)
	}

	return nil
}
