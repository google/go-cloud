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

package postgres

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Can't use Unix sockets on Windows")
	}
	postgresPath, err := exec.LookPath("postgres")
	if err != nil {
		t.Skip("Can't find postgres:", err)
	}
	initdbPath, err := exec.LookPath("initdb")
	if err != nil {
		t.Skip("Can't find initdb:", err)
	}

	// Create a temporary database data directory.
	currUser, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := ioutil.TempDir("", "gocloud_postgres_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Cleaning up: %v", err)
		}
	}()
	dataDir := filepath.Join(dir, "data")
	initdbCmd := exec.Command(initdbPath, "-U", currUser.Username, "-D", dataDir)
	initdbOutput := new(bytes.Buffer)
	initdbCmd.Stdout = initdbOutput
	initdbCmd.Stderr = initdbOutput
	err = initdbCmd.Run()
	if err != nil {
		t.Log(initdbOutput)
		t.Fatal(err)
	}

	// Configure the database server to listen on a Unix socket located in the temporary directory.
	socketDir, err := filepath.Abs(filepath.Join(dir, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(socketDir, 0777); err != nil {
		t.Fatal(err)
	}
	confData := new(bytes.Buffer)
	fmt.Fprintf(confData, "unix_socket_directories = '%s'\n", socketDir)
	err = ioutil.WriteFile(filepath.Join(dataDir, "postgresql.conf"), confData.Bytes(), 0666)
	if err != nil {
		t.Fatal(err)
	}

	// Start the database server (and arrange for it to be stopped at test end).
	server := exec.Command(postgresPath, "-D", dataDir)
	serverOutput := new(bytes.Buffer)
	server.Stdout = serverOutput
	server.Stderr = serverOutput
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	serverSignaled := false
	defer func() {
		if !serverSignaled {
			if err := server.Process.Kill(); err != nil {
				t.Error("Stopping server:", err)
			}
		}
		// Wait for server to exit, but ignore the expected failure error code.
		server.Wait()
		if t.Failed() {
			t.Log(serverOutput)
		}
	}()

	// Now the actual test: can we connect to the database via URL opener?
	ctx := context.Background()
	dbURL := &url.URL{
		Scheme: "blablabla", // Intentionally not "postgres" to ensure any scheme works.
		User:   url.User(currUser.Username),
		Path:   "/postgres",
		// Use the query parameter to avoid https://github.com/lib/pq/issues/796
		RawQuery: url.Values{"host": {socketDir}}.Encode(),
	}
	t.Logf("PostgreSQL URL: %s", dbURL)
	db, err := new(URLOpener).OpenPostgresURL(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	// Developing a realistic query would be hard, so instead we trust that the
	// PostgreSQL library reports healthy correctly. Since there's no way to
	// synchronize the server start and the ping, we might have to ping a few
	// times before it is healthy. (The overall test runner timeout will interrupt
	// if this takes too long.)
	for {
		err := db.Ping()
		if err == nil {
			break
		}
		t.Log("Ping:", err)
		time.Sleep(100 * time.Millisecond)
	}
	if err := db.Close(); err != nil {
		t.Error("Close:", err)
	}
	server.Process.Signal(os.Interrupt)
	serverSignaled = true
}
