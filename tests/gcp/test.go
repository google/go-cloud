//!/bin/bash

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
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	testDir := flag.String("test_dir", ".", "tests/gcp directory")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: go run test.go PROJECT\n")
		os.Exit(64)
	}
	log.SetPrefix("tests/gcp/test: ")
	if err := test(*testDir, flag.Arg(0)); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func test(testDir, projectID string) error {
	gcp := &gcloud{projectID}

	tempdir, err := ioutil.TempDir("", "go-cloud-gcp-test")
	if err != nil {
		return fmt.Errorf("making temp dir: %v", err)
	}
	defer os.RemoveAll(tempdir)

	log.Printf("Provisioning GCP resources...")
	if _, err := run("terraform", "init", "-input=false", testDir); err != nil {
		return err
	}
	defer func() {
		log.Print("Tearing down GCP resources...")

		if _, err := run("terraform", "destroy", "-auto-approve", "-var", "project="+projectID, testDir); err != nil {
			panic(err)
		}
	}()
	if _, err := run("terraform", "apply", "-auto-approve", "-input=false", "-var", "project="+projectID, testDir); err != nil {
		return err
	}

	log.Print("Making sure vgo image is built...")
	vgoImage := fmt.Sprintf("gcr.io/%s/vgo", strings.Replace(projectID, ":", "/", -1))
	checkVgoArgs := gcp.cmd("container", "images", "describe", "--format=value(image_summary.digest)", vgoImage)
	checkVgo := exec.Command(checkVgoArgs[0], checkVgoArgs[1:]...)
	out, err := checkVgo.CombinedOutput()
	switch {
	case strings.Contains(string(out), "is not a valid name"):
		log.Print("Building vgo image...")
		if _, err := run(gcp.cmd("container", "builds", "submit", "-t", vgoImage, filepath.Join(testDir, "..", "..", "internal", "vgo_docker"))...); err != nil {
			return err
		}
	case err != nil:
		return fmt.Errorf("running %v: %v", checkVgo.Args, err)
	}

	log.Print("Building app binary with vgo...")
	appImage := fmt.Sprintf("gcr.io/%s/gcp-test", strings.Replace(projectID, ":", "/", -1))
	subs := "_IMAGE_NAME=" + appImage + ",_VGO_IMAGE_NAME=" + vgoImage
	buildID, err := run(gcp.cmd("container", "builds", "submit", "--async", "--format='value(id)'", "--config", testDir+"app/cloudbuild.yaml", "--substitutions", subs, filepath.Join(testDir, "..", ".."))...)
	if err != nil {
		return err
	}

	if _, err := run(gcp.cmd("container", "builds", "log", "--stream", buildID)...); err != nil {
		return err
	}

	log.Print("Deploying the app on the cluster...")
	// TODO(light): Some values might need escaping.
	YAMLIn, err := ioutil.ReadFile(filepath.Join(testDir, "app", "gcp-test.yaml.in"))
	if err != nil {
		return fmt.Errorf("reading YAML precursor file: %v", err)
	}
	YAMLOut := strings.Replace(string(YAMLIn), "{{IMAGE}}", appImage+":"+buildID, -1)
	if err := ioutil.WriteFile(filepath.Join(tempdir, "gcp-test.yaml"), []byte(YAMLOut), 0666); err != nil {
		return fmt.Errorf("writing YAML file: %v", err)
	}
	// TODO(ijt): use terraform output -json
	clusterName, err := run("terraform", "output", "-state", filepath.Join(testDir, "terraform.tfstate"), "cluster_name")
	if err != nil {
		return err
	}
	clusterZone, err := run("terraform", "output", "-state", filepath.Join(testDir, "terraform.tfstate"), "cluster_zone")
	if err != nil {
		return err
	}
	if _, err := run(gcp.cmd("container", "clusters", "get-credentials", "--zone", clusterZone, clusterName)...); err != nil {
		return err
	}

	if _, err := run("kubectl", "apply", "-f", filepath.Join(tempdir, "gcp-test.yaml")); err != nil {
		return err
	}

	log.Print("Waiting for load balancer...")
	var endpoint string
	for {
		serviceJSON, err := run("kubectl", "get", "service", "gcp-test", "-o", "json")
		if err != nil {
			return err
		}
		var t thing
		if err := json.Unmarshal([]byte(serviceJSON), &t); err != nil {
			return fmt.Errorf("parsing JSON: %v", err)
		}
		ings := t.status.loadBalancer.ingress
		if len(ings) == 1 {
			endpoint = ings[0].ip
			if endpoint != "" {
				break
			}
		}
		nap := 5 * time.Second
		log.Printf("Trying again in %v", nap)
		time.Sleep(nap)
	}

	log.Print("Running test:")
	if err := os.Chdir(filepath.Join(testDir, "app")); err != nil {
		return fmt.Errorf("changing directories: %v", err)
	}
	if _, err := run("vgo", "test", "-v", "-args", "--address", "http://"+endpoint, "--project", projectID); err != nil {
		return err
	}

	return nil
}

type thing struct{ status s }
type s struct{ loadBalancer lb }
type lb struct{ ingress []ing }
type ing struct{ ip string }

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

type gcloud struct {
	// project ID
	project string
}

func (gcp *gcloud) cmd(args ...string) []string {
	return append([]string{"gcloud", "--quiet", "--project", gcp.project}, args...)
}
