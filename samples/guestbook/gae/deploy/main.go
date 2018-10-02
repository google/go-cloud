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

// The deploy program deploys the Guestbook app to GAE.
// It is meant to be run from the go-cloud-samples/guestbook directory.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("gae/deploy: ")
	if err := deploy(); err != nil {
		log.Fatal(err)
	}
}

func deploy() error {
	// Extract some info from the tfstate file.
	type tfItem struct {
		Sensitive bool
		Type      string
		Value     string
	}
	type state struct {
		Project             tfItem `json:"project"`
		Bucket              tfItem `json:"bucket"`
		DBInstance          tfItem `json:"database_instance"`
		DBRegion            tfItem `json:"database_region"`
		DBGuestbookPassword tfItem `json:"database_guestbook_password"`
		MotdVarConfig       tfItem `json:"motd_var_config"`
		MotdVarName         tfItem `json:"motd_var_name"`
	}
	tfStateb, err := runb("terraform", "output", "-state", "gae/terraform.tfstate", "-json")
	if err != nil {
		return err
	}
	var s state
	if err := json.Unmarshal(tfStateb, &s); err != nil {
		return fmt.Errorf("parsing terraform state JSON: %v", err)
	}

	// Fill out the params for app.yaml.
	var p Params
	p.Instance = fmt.Sprintf("%s:%s:%s", s.Project.Value, s.DBRegion.Value, s.DBInstance.Value)
	p.Password = s.DBGuestbookPassword.Value
	p.Bucket = s.Bucket.Value

	// Write the app.yaml configuration file.
	t, err := template.New("appyaml").Parse(appYAML)
	if err != nil {
		return fmt.Errorf("parsing app.yaml template: %v", err)
	}
	f, err := os.Create("app.yaml")
	if err != nil {
		return err
	}
	defer f.Close()
	t.Execute(f, p)

	// Deploy the app to GAE.
	cmd := exec.Command("gcloud", "app", "deploy")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running gcloud app deploy: %v", err)
	}

	return nil
}

type Params struct {
	Instance string
	Password string
	Bucket   string
}

const appYAML = `runtime: go111
env_variables:
  DB_USER: guestbook
  DB_INSTANCE: {{.Instance}}
  DB_DATABASE: guestbook
  DB_PASSWORD: {{.Password}}
  GUESTBOOK_BUCKET: {{.Bucket}}

`

type gcloud struct {
	projectID string
}

func (gcp *gcloud) cmd(args ...string) *exec.Cmd {
	args = append([]string{"--quiet", "--project", gcp.projectID}, args...)
	cmd := exec.Command("gcloud", args...)
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Stderr = os.Stderr
	return cmd
}

func run(args ...string) (stdout string, err error) {
	stdoutb, err := runb(args...)
	return strings.TrimSpace(string(stdoutb)), err
}

func runb(args ...string) (stdout []byte, err error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	cmd.Env = append(cmd.Env, os.Environ()...)
	stdoutb, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running %v: %v", cmd.Args, err)
	}
	return stdoutb, nil
}
