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

// The provision_db program connects to an RDS database and initializes it with
// SQL from stdin. It is intended to be invoked from Terraform.
package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("aws/provision_db: ")
	host := flag.String("host", "", "hostname of database")
	securityGroup := flag.String("security_group", "", "database security group")
	database := flag.String("database", "", "name of database to provision")
	password := flag.String("password", "", "root password on database")
	schema := flag.String("schema", "", "path to .sql file defining the database schema")
	flag.Parse()
	missing := false
	flag.VisitAll(func(f *flag.Flag) {
		if f.Value.String() == "" {
			log.Printf("Required flag -%s not set.", f.Name)
			missing = true
		}
	})
	if missing {
		os.Exit(64)
	}
	if err := provisionDb(*host, *securityGroup, *database, *password, *schema); err != nil {
		log.Fatal(err)
	}
}

func provisionDb(dbHost, securityGroupID, dbName, dbPassword, schemaPath string) error {
	const mySQLImage = "mysql:5.6"

	// Pull the necessary Docker images.
	log.Print("Downloading Docker images...")
	if _, err := run("docker", "pull", mySQLImage); err != nil {
		return err
	}

	// Create a temporary directory to hold the certificates.
	// We resolve all symlinks to avoid Docker on Mac issues, see
	// https://github.com/google/go-cloud/issues/110.
	tempdir, err := ioutil.TempDir("", "guestbook-ca")
	if err != nil {
		return fmt.Errorf("creating temp dir for certs: %v", err)
	}
	defer os.RemoveAll(tempdir)
	tempdir, err = filepath.EvalSymlinks(tempdir)
	if err != nil {
		return fmt.Errorf("evaluating any symlinks: %v", err)
	}
	resp, err := http.Get("https://s3.amazonaws.com/rds-downloads/rds-ca-2015-root.pem")
	if err != nil {
		return fmt.Errorf("fetching pem file: %v", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("response status code is %d, want 200", resp.StatusCode)
	}
	defer resp.Body.Close()
	caPath := filepath.Join(tempdir, "rds-ca.pem")
	caFile, err := os.Create(caPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(caFile, resp.Body); err != nil {
		return fmt.Errorf("copying response to file: %v", err)
	}

	log.Print("Adding a temporary ingress rule")
	if _, err := run("aws", "ec2", "authorize-security-group-ingress", "--group-id", securityGroupID, "--protocol=tcp", "--port=3306", "--cidr=0.0.0.0/0"); err != nil {
		return err
	}
	defer func() {
		log.Print("Removing ingress rule...")
		if _, err := run("aws", "ec2", "revoke-security-group-ingress", "--group-id", securityGroupID, "--protocol=tcp", "--port=3306", "--cidr=0.0.0.0/0"); err != nil {
			log.Print(err)
		}
	}()
	log.Printf("Added ingress rule to %s for port 3306", securityGroupID)

	// Send schema.
	log.Print("Sending schema to database...")
	schema, err := os.Open(schemaPath)
	if err != nil {
		return err
	}
	defer schema.Close()
	mySQLCmd := fmt.Sprintf(`mysql -h'%s' -uroot -p'%s' --ssl-ca=/ca/rds-ca.pem '%s'`, dbHost, dbPassword, dbName)
	connect := exec.Command("docker", "run", "--rm", "--interactive", "--volume", tempdir+":/ca", mySQLImage, "sh", "-c", mySQLCmd)
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
