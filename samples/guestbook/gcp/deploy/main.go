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

// The deploy program builds the Guestbook server locally and deploys it to
// GKE.
package main

import (
	"bytes"
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
	guestbookDir := flag.String("guestbook_dir", "..", "directory containing the guestbook example")
	tfStatePath := flag.String("tfstate", "terraform.tfstate", "path to terraform state file")
	flag.Parse()
	log.SetPrefix("gcp/deploy: ")
	if err := deploy(*guestbookDir, *tfStatePath); err != nil {
		fmt.Fprintln(os.Stderr, "deploy:", err)
		os.Exit(1)
	}
}

func deploy(guestbookDir, tfStatePath string) error {
	type tfItem struct {
		Sensitive bool
		Type      string
		Value     string
	}
	type state struct {
		Project          tfItem
		ClusterName      tfItem `json:"cluster_name"`
		ClusterZone      tfItem `json:"cluster_zone"`
		Bucket           tfItem
		DatabaseInstance tfItem `json:"database_instance"`
		DatabaseRegion   tfItem `json:"database_region"`
		MotdVarConfig    tfItem `json:"motd_var_config"`
		MotdVarName      tfItem `json:"motd_var_name"`
	}
	getTfState := exec.Command("terraform", "output", "-state", tfStatePath, "-json")
	getTfState.Stderr = os.Stderr
	out, err := getTfState.Output()
	if err != nil {
		return fmt.Errorf("getting terraform state with %v: %v: %s", getTfState.Args, err, out)
	}
	var tfState state
	if err := json.Unmarshal(out, &tfState); err != nil {
		return fmt.Errorf("parsing terraform state JSON (%s): %v", out, err)
	}
	gcp := gcloud{project: tfState.Project.Value}
	tempDir, err := ioutil.TempDir("", "guestbook-k8s-")
	if err != nil {
		return fmt.Errorf("making temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Fill in Kubernetes template parameters.
	proj := strings.Replace(tfState.Project.Value, ":", "/", -1)
	imageName := fmt.Sprintf("gcr.io/%s/guestbook", proj)
	gbyin, err := ioutil.ReadFile(filepath.Join(guestbookDir, "gcp", "guestbook.yaml.in"))
	if err != nil {
		return fmt.Errorf("reading guestbook.yaml.in: %v", err)
	}
	gby := string(gbyin)
	replacements := map[string]string{
		"{{IMAGE}}":             imageName,
		"{{bucket}}":            tfState.Bucket.Value,
		"{{database_instance}}": tfState.DatabaseInstance.Value,
		"{{database_region}}":   tfState.DatabaseRegion.Value,
		"{{motd_var_config}}":   tfState.MotdVarConfig.Value,
		"{{motd_var_name}}":     tfState.MotdVarName.Value,
	}
	for old, new := range replacements {
		gby = strings.Replace(gby, old, new, -1)
	}
	if err := ioutil.WriteFile(filepath.Join(tempDir, "guestbook.yaml"), []byte(gby), 0666); err != nil {
		return fmt.Errorf("writing guestbook.yaml: %v", err)
	}

	// Build Guestbook Docker image.
	log.Printf("Building %s...", imageName)
	build := exec.Command("vgo", "build", "-o", "gcp/guestbook")
	env := append(build.Env, "GOOS=linux", "GOARCH=amd64")
	env = append(env, os.Environ()...)
	build.Env = env
	absDir, err := filepath.Abs(guestbookDir)
	if err != nil {
		return fmt.Errorf("getting abs path to guestbook dir (%s): %v", guestbookDir, err)
	}
	build.Dir = absDir
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("building guestbook app by running %v: %v", build.Args, err)
	}
	cbs := gcp.cmd("container", "builds", "submit", "-t", imageName, filepath.Join(guestbookDir, "gcp"))
	if out, err := cbs.CombinedOutput(); err != nil {
		return fmt.Errorf("building container image with %v: %v: %s", cbs.Args, err, out)
	}

	// Run on Kubernetes.
	log.Printf("Deploying to %s...", tfState.ClusterName.Value)
	getCreds := gcp.cmd("container", "clusters", "get-credentials", "--zone", tfState.ClusterZone.Value, tfState.ClusterName.Value)
	getCreds.Stderr = os.Stderr
	if err := getCreds.Run(); err != nil {
		return fmt.Errorf("getting credentials with %v: %v", getCreds.Args, err)
	}
	kubeCmds := [][]string{
		{"kubectl", "apply", "-f", filepath.Join(tempDir, "guestbook.yaml")},
		// Force pull the latest image.
		{"kubectl", "scale", "--replicas", "0", "deployment/guestbook"},
		{"kubectl", "scale", "--replicas", "1", "deployment/guestbook"},
	}
	for _, kcmd := range kubeCmds {
		cmd := exec.Command(kcmd[0], kcmd[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("running %v: %v: %s", cmd.Args, err, out)
		}
	}

	// Wait for endpoint then print it.
	log.Printf("Waiting for load balancer...")
	for {
		getService := exec.Command("kubectl", "get", "service", "guestbook", "-o", "json")
		var errBuf bytes.Buffer
		getService.Stderr = &errBuf
		out, err := getService.Output()
		if err != nil {
			return fmt.Errorf("getting service info with %v: %v: %s", getService.Args, err, errBuf)
			continue
		}
		var t thing
		if err := json.Unmarshal(out, &t); err != nil {
			return fmt.Errorf("parsing JSON output of %v: %v", getService.Args, err)
		}
		i := t.Status.LoadBalancer.Ingress
		if len(i) == 0 || i[0].IP == "" {
			dt := time.Second
			log.Printf("No ingress returned in %s. Trying again in %v", out, dt)
			time.Sleep(dt)
			continue
		}
		endpoint := i[0].IP
		log.Printf("Deployed at http://%s:8080", endpoint)
		break
	}
	return nil
}

type thing struct{ Status *s }
type s struct{ LoadBalancer lb }
type lb struct{ Ingress []ing }
type ing struct{ IP string }

type gcloud struct {
	// project ID
	project string
}

func (gcp *gcloud) cmd(args ...string) *exec.Cmd {
	args = append([]string{"--quiet", "--project", gcp.project}, args...)
	return exec.Command("gcloud", args...)
}
