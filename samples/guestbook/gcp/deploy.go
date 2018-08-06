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
	flag.Parse()
	if err := deploy(*guestbookDir); err != nil {
		fmt.Fprintln(os.Stderr, "deploy:", err)
		os.Exit(1)
	}
}

func deploy(guestbookDir string) error {
	tfStatePath := filepath.Join(guestbookDir, "gcp", "terraform.tfstate")
	keys := []string{"project", "cluster_name", "cluster_zone", "bucket", "database_instance", "database_region", "motd_var_config", "motd_var_name"}
	tfState := make(map[string]string)
	for _, k := range keys {
		cmd := exec.Command("terraform", "output", "-state", tfStatePath, k)
		outb, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("running %v: %v: %s", cmd.Args, err, outb)
		}
		tfState[k] = string(outb)
	}

	gcp := gcloud{project: tfState["project"]}
	tempDir, err := ioutil.TempDir("", "guestbook-k8s-")
	if err != nil {
		return fmt.Errorf("making temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Fill in Kubernetes template parameters.
	proj := strings.Replace(tfState["project"], ":", "/", -1)
	imageName := fmt.Sprintf("gcr.io/%s/guestbook", proj)
	gbyin, err := ioutil.ReadFile(filepath.Join(guestbookDir, "gcp", "guestbook.yaml.in"))
	if err != nil {
		return fmt.Errorf("reading guestbook.yaml.in: %v", err)
	}
	gby := string(gbyin)
	replacements := map[string]string{
		"{{IMAGE}}":             imageName,
		"{{bucket}}":            tfState["bucket"],
		"{{database_instance}}": tfState["database_instance"],
		"{{database_region}}":   tfState["database_region"],
		"{{motd_var_config}}":   tfState["motd_var_config"],
		"{{motd_var_name}}":     tfState["motd_var_name"],
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
	build.Env = []string{"GOOS=linux", "GOARCH=amd64"}
	build.Dir = guestbookDir
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("building guestbook app by running %v: %v: %s", build.Args, err, out)
	}
	cmd := gcp.cmd("container", "builds", "submit", "-t", imageName, filepath.Join(guestbookDir, "gcp"))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("building container image with %v: %v: %s", cmd.Args, err, out)
	}

	// Run on Kubernetes.
	log.Printf("Deploying to %s...", tfState["cluster_name"])
	cmd = gcp.cmd("container", "clusters", "get-credentials", "--zone", tfState["cluster_zone"], tfState["cluster_name"])
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("getting credentials with %v: %v: %s", cmd.Args, err, out)
	}
	kubeCmds := [][]string{
		{"kubectl", "apply", "-f", filepath.Join(tempDir, "guestbook.yaml")},
		// Force pull the latest image.
		{"kubectl", "scale", "--replicas", "0", filepath.Join("deployment", guestbookDir)},
		{"kubectl", "scale", "--replicas", "1", filepath.Join("deployment", guestbookDir)},
	}
	for _, kcmd := range kubeCmds {
		cmd = exec.Command(kcmd[0], kcmd[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("running %v: %v: %s", cmd.Args, err, out)
		}
	}

	// Wait for endpoint then print it.
	log.Printf("Waiting for load balancer...")
	for {
		cmd := exec.Command("kubectl", "get", "service", "guestbook", "-o", "json")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("running %v: %v: %s", cmd.Args, err, out)
		}
		var t thing
		if err := json.Unmarshal(out, t); err != nil {
			dt := 5 * time.Second
			log.Printf("parsing service JSON: %v. Retrying after %v", err, dt)
			time.Sleep(dt)
			continue
		}
		endpoint := t.status.loadBalancer.ingress[0]
		log.Printf("Deployed at http://%s:8080", endpoint)
		break
	}
	return nil
}

type thing struct {
	status s
}

type s struct {
	loadBalancer lb
}

type lb struct {
	ingress []string
}

type gcloud struct {
	// project ID
	project string
}

func (gcp *gcloud) cmd(args ...string) *exec.Cmd {
	args = append([]string{"--quiet", "--project", gcp.project}, args...)
	return exec.Command("gcloud", args...)
}
