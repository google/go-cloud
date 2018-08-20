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

// The provision_db program connects to a Cloud SQL database and initializes it
// with SQL from a file. It's intended to be invoked from Terraform.
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
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("gcp/provision_db: ")
	project := flag.String("project", "", "GCP project ID")
	serviceAccount := flag.String("service_account", "", "name of service account in GCP project")
	instance := flag.String("instance", "", "database instance name")
	database := flag.String("database", "", "name of database to initialize")
	password := flag.String("password", "", "root password for the database")
	schema := flag.String("schema", "", "path to .sql file defining the database schema")
	flag.Parse()
	missing := false
	flag.VisitAll(func(f *flag.Flag) {
		if f.Value.String() == "" {
			log.Printf("Required flag -%s is not set.", f.Name)
			missing = true
		}
	})
	if missing {
		os.Exit(64)
	}
	if err := provisionDB(*project, *serviceAccount, *instance, *database, *password, *schema); err != nil {
		log.Fatal(err)
	}
}

type key struct {
	PrivateKeyID string `json:"private_key_id"`
}

func provisionDB(projectID, serviceAccount, dbInstance, dbName, dbPassword, schemaPath string) error {
	log.Printf("Downloading Docker images...")
	const mySQLImage = "mysql:5.6"
	cloudSQLProxyImage := "gcr.io/cloudsql-docker/gce-proxy:1.11"
	images := []string{mySQLImage, cloudSQLProxyImage}
	for _, img := range images {
		if _, err := run("docker", "pull", img); err != nil {
			return err
		}
	}

	log.Printf("Getting connection string from database metadata...")
	gcp := &gcloud{projectID}
	dbConnStr, err := run(gcp.cmd("sql", "instances", "describe", "--format", "value(connectionName)", dbInstance)...)
	if err != nil {
		return fmt.Errorf("getting connection string: %v", err)
	}

	// Create a temporary directory to hold the service account key.
	// We resolve all symlinks to avoid Docker on Mac issues, see
	// https://github.com/google/go-cloud/issues/110.
	serviceAccountVolDir, err := ioutil.TempDir("", "guestbook-service-acct")
	if err != nil {
		return fmt.Errorf("creating temp dir to hold service account key: %v", err)
	}
	serviceAccountVolDir, err = filepath.EvalSymlinks(serviceAccountVolDir)
	if err != nil {
		return fmt.Errorf("evaluating any symlinks: %v", err)
	}
	defer os.RemoveAll(serviceAccountVolDir)
	log.Printf("Created %v", serviceAccountVolDir)

	// Furnish a new service account key.
	if _, err := run(gcp.cmd("iam", "service-accounts", "keys", "create", "--iam-account="+serviceAccount, serviceAccountVolDir+"/key.json")...); err != nil {
		return fmt.Errorf("creating new service account key: %v", err)
	}
	keyJSONb, err := ioutil.ReadFile(filepath.Join(serviceAccountVolDir, "key.json"))
	if err != nil {
		return fmt.Errorf("reading key.json file: %v", err)
	}
	var k key
	if err := json.Unmarshal(keyJSONb, &k); err != nil {
		return fmt.Errorf("parsing key.json: %v", err)
	}
	serviceAccountKeyID := k.PrivateKeyID
	defer func() {
		if _, err := run(gcp.cmd("iam", "service-accounts", "keys", "delete", "--iam-account", serviceAccount, serviceAccountKeyID)...); err != nil {
			log.Printf("deleting service account key: %v", err)
		}
	}()
	log.Printf("Created service account key %s", serviceAccountKeyID)

	log.Printf("Starting Cloud SQL proxy...")
	proxyContainerID, err := run("docker", "run", "--detach", "--rm", "--volume", serviceAccountVolDir+":/creds", "--publish", "3306", cloudSQLProxyImage, "/cloud_sql_proxy", "-instances", dbConnStr+"=tcp:0.0.0.0:3306", "-credential_file=/creds/key.json")
	if err != nil {
		return err
	}
	defer func() {
		if _, err := run("docker", "kill", proxyContainerID); err != nil {
			log.Printf("failed to kill docker container for proxy: %v", err)
		}
	}()

	log.Print("Sending schema to database...")
	mySQLCmd := fmt.Sprintf(`mysql --wait -h"$PROXY_PORT_3306_TCP_ADDR" -P"$PROXY_PORT_3306_TCP_PORT" -uroot -p'%s' '%s'`, dbPassword, dbName)
	connect := exec.Command("docker", "run", "--rm", "--interactive", "--link", proxyContainerID+":proxy", mySQLImage, "sh", "-c", mySQLCmd)
	schema, err := os.Open(schemaPath)
	if err != nil {
		return err
	}
	defer schema.Close()
	connect.Stdin = schema
	connect.Stderr = os.Stderr
	if err := connect.Run(); err != nil {
		return fmt.Errorf("running %v: %v", connect.Args, err)
	}

	return nil
}

func run(args ...string) (stdout string, err error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
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
