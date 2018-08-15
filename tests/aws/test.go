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
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	testDir := flag.String("test_dir", ".", "directory where the test will run")
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s gcp_project_id ssh_key_path [region]\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}
	log.SetPrefix("tests/aws/test: ")

	var region string
	switch {
	case flag.NArg() >= 3:
		region = flag.Arg(2)
	default:
		region = "us-west-1"
		log.Printf("region not provided, default to %s", region)
	}

	if err := test(flag.Arg(0), flag.Arg(1), region, *testDir); err != nil {
		log.Fatal(err)
	}
}

func test(GCPProjectID, SSHKeyPath, region, testDir string) error {
	if err := os.Chdir(testDir); err != nil {
		return err
	}

	tempdir, err := ioutil.TempDir("", "go-cloud-aws-test")
	if err != nil {
		return fmt.Errorf("making temp dir: %v", err)
	}
	defer os.RemoveAll(tempdir)

	log.Print("Building test app...")
	build := exec.Command("vgo", "build", "-o", filepath.Join(tempdir, "app"))
	build.Dir = filepath.Join(testDir, "app")
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("running %v: %v", build.Args, err)
	}

	log.Print("Provisioning AWS resources...")
	if _, err := run("terraform", "init", "-input=false", testDir); err != nil {
		return err
	}
	var1 := "app_binary=" + filepath.Join(tempdir, "app")
	var2 := "gcp_project=" + GCPProjectID
	defer func() {
		log.Print("Tearing down AWS resources...")
		if _, err := run("terraform", "destroy", "-auto-approve", "-input=false", "-var", var1, "-var", var2, testDir); err != nil {
			panic(err)
		}
	}()
	if _, err := run("terraform", "apply", "-auto-approve", "-input=false", "-var", var1, "-var", var2, testDir); err != nil {
		return err
	}

	log.Print("Running test...")
	hostIP, err := run("terraform", "output", "-state="+filepath.Join(testDir, "terraform.tfstate"), "host_ip")
	if err != nil {
		return err
	}
	vgoTest := exec.Command("vgo", "test", "-v", "-args", "--gcp-project", GCPProjectID, "--aws-region", region, "--host-ip", hostIP, "--key-path", SSHKeyPath)
	vgoTest.Dir = filepath.Join(testDir, "app")
	vgoTest.Stdout = os.Stdout
	vgoTest.Stderr = os.Stderr
	if err := vgoTest.Run(); err != nil {
		return fmt.Errorf("running %v: %v", vgoTest.Args, err)
	}

	return nil
}

func run(args ...string) (stdout string, err error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	cmd.Env = append(cmd.Env, "TF_IN_AUTOMATION=1")
	cmd.Env = append(cmd.Env, os.Environ()...)
	stdoutb, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("running %v: %v", cmd.Args, err)
	}
	return strings.TrimSpace(string(stdoutb)), nil
}
