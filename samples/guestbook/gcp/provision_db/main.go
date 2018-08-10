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
// with SQL from stdin. It's intended to be invoked from Terraform.
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
)

func main() {
	if len(os.Args) != 1+5 {
		fmt.Fprintf(os.Stderr, "usage: provision_db PROJECT SERVICE_ACCOUNT INSTANCE DATABASE ROOT_PASSWORD\n")
		os.Exit(64)
	}

	log.SetPrefix("gcp/provision_db: ")

	if err := provisionDb(os.Args[1], os.Args[2], os.Args[3], os.Args[4], os.Args[5]); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

type key struct {
	PrivateKeyID string `json:"private_key_id"`
}

type gcloud struct {
	// project ID
	project string
}

func (gcp *gcloud) cmd(args ...string) *exec.Cmd {
	args = append([]string{"--quiet", "--project", gcp.project}, args...)
	return exec.Command("gcloud", args...)
}

func provisionDb(projectID, serviceAccount, dbInstance, dbName, dbPassword string) error {
	log.Printf("Downloading Docker images...")
	mySQLImage := "mysql:5.6"
	cloudSQLProxyImage := "gcr.io/cloudsql-docker/gce-proxy:1.11"
	images := []string{mySQLImage, cloudSQLProxyImage}
	for _, img := range images {
		pull := exec.Command("docker", "pull", img)
		pull.Stderr = os.Stderr
		if err := pull.Run(); err != nil {
			return fmt.Errorf("running %v: %v", pull.Args, err)
		}
	}

	log.Printf("Getting connection string from database metadata...")
	gcp := &gcloud{projectID}
	getConnString := gcp.cmd("sql", "instances", "describe", "--format", "value(connectionName)", dbInstance)
	getConnString.Stderr = os.Stderr
	connStrb, err := getConnString.Output()
	if err != nil {
		return fmt.Errorf("getting connection string: %v", err)
	}
	dbConnStr := strings.TrimSpace(string(connStrb))

	// Create a temporary directory to hold the service account key.
	// We resolve all symlinks to avoid Docker on Mac issues, see
	// https://github.com/google/go-cloud/issues/110.
	serviceAccountVoldir, err := ioutil.TempDir("", "guestbook-service-acct")
	if err != nil {
		return fmt.Errorf("creating temp dir to hold service account key: %v", err)
	}
	if err := os.Chdir(serviceAccountVoldir); err != nil {
		return fmt.Errorf("changing to temp dir: %v", err)
	}
	defer os.RemoveAll(serviceAccountVoldir)
	log.Printf("Created %v", serviceAccountVoldir)

	// Furnish a new service account key.
	createKey := gcp.cmd("iam", "service-accounts", "keys", "create", "--iam-account="+serviceAccount, serviceAccountVoldir+"/key.json")
	createKey.Stderr = os.Stderr
	if err := createKey.Run(); err != nil {
		return fmt.Errorf("creating new service account key: %v", err)
	}
	keyJSONb, err := ioutil.ReadFile(serviceAccountVoldir + "/key.json")
	if err != nil {
		return fmt.Errorf("reading key.json file: %v", err)
	}
	var k key
	if err := json.Unmarshal(keyJSONb, &k); err != nil {
		return fmt.Errorf("parsing key.json: %v", err)
	}
	serviceAccountKeyID := k.PrivateKeyID
	defer func() {
		delKey := gcp.cmd("iam", "service-accounts", "keys", "delete", "--iam-account", serviceAccount, serviceAccountKeyID)
		delKey.Stderr = os.Stderr
		if err := delKey.Run(); err != nil {
			panic(fmt.Sprintf("deleting service account key: %v", err))
		}
	}()
	log.Printf("Created service account key %s", serviceAccountKeyID)

	log.Printf("Starting Cloud SQL proxy...")
	startProxy := exec.Command("docker", "run", "--detach", "--rm", "--volume", serviceAccountVoldir+":/creds", "--publish", "3306", cloudSQLProxyImage, "/cloud_sql_proxy", "-instances", dbConnStr+"=tcp:0.0.0.0:3306", "-credential_file=/creds/key.json")
	startProxy.Stderr = os.Stderr
	proxyContainerIDb, err := startProxy.Output()
	if err != nil {
		return fmt.Errorf("running %v: %v", startProxy.Args, err)
	}
	proxyContainerID := strings.TrimSpace(string(proxyContainerIDb))
	defer func() {
		kill := exec.Command("docker", "kill", proxyContainerID)
		kill.Stderr = os.Stderr
		if err := kill.Run(); err != nil {
			panic(fmt.Sprintf("killing docker container for proxy: %v", err))
		}
	}()

	log.Print("Connecting to database, expecting schema on stdin...")
	mySQLCmd := fmt.Sprintf(`mysql --wait -h"$PROXY_PORT_3306_TCP_ADDR" -P"$PROXY_PORT_3306_TCP_PORT" -uroot -p'%s' '%s'`, dbPassword, dbName)
	connect := exec.Command("docker", "run", "--rm", "--interactive", "--link", proxyContainerID+":proxy", mySQLImage, "sh", "-c", mySQLCmd)
	connect.Stderr = os.Stderr
	connect.Stdin = os.Stdin
	if err := connect.Run(); err != nil {
		return fmt.Errorf("running %v: %v", connect.Args, err)
	}

	return nil
}
