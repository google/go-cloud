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

// The test-driver command makes various requests to the deployed test app and
// tests features initialized by the AWS server packages.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"cloud.google.com/go/logging/logadmin"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/xray"
	"github.com/google/go-cloud/tests"
	"golang.org/x/crypto/ssh"
)

var (
	hostIP         string
	serverAddr     string
	awsRegion      string
	gcpProjectID   string
	sshUser        string
	sshKeyPath     string
	logadminClient *logadmin.Client
	xrayClient     *xray.XRay
)

func init() {
	flag.StringVar(&hostIP, "host-ip", "0.0.0.0", "host IP address of the instance")
	flag.StringVar(&awsRegion, "aws-region", "us-west-1", "the region used to run the sample app")
	flag.StringVar(&gcpProjectID, "gcp-project", "", "GCP project used to collect request logs")
	flag.StringVar(&sshUser, "ssh-user", "admin", "user name used to ssh into the EC2 instance")
	flag.StringVar(&sshKeyPath, "key-path", "", "path to the key file")
}

func main() {
	flag.Parse()

	ctx := context.Background()
	if err := startApp(); err != nil {
		log.Fatal("failed to start app:", err)
	}
	time.Sleep(3 * time.Second)

	serverAddr = "http://" + hostIP + ":8080"
	if _, err := http.Get(serverAddr); err != nil {
		log.Fatal("ping server:", err)
	}

	var err error
	logadminClient, err = logadmin.NewClient(ctx, gcpProjectID)
	if err != nil {
		log.Fatalf("error creating logadmin client: %v\n", err)
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion),
	})
	if err != nil {
		log.Fatalf("error creating an AWS session: %v\n", err)
	}
	xrayClient = xray.New(sess)
	testCases := []tests.Test{
		testRequestlog{
			url: "/requestlog/",
		},
		testTrace{
			url: "/trace/",
		},
	}
	var wg sync.WaitGroup
	wg.Add(len(testCases))

	for _, tc := range testCases {
		go func(t tests.Test) {
			defer wg.Done()
			log.Printf("Test %s running\n", t)
			if err := t.Run(); err != nil {
				log.Printf("Test %s failed: %v\n", t, err)
			} else {
				log.Printf("Test %s passed\n", t)
			}
		}(tc)
	}

	wg.Wait()
}

func startApp() error {
	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			publicKeyFile(sshKeyPath),
		},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		Timeout: 3 * time.Second,
	}
	c, err := ssh.Dial("tcp", hostIP+":22", config)
	if err != nil {
		return fmt.Errorf("failed to dial %s:22: %v", hostIP, err)
	}
	defer c.Close()
	session, err := c.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()
	return session.Start("nohup /home/admin/app 2>&1 | logger &")
}

func publicKeyFile(path string) ssh.AuthMethod {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil
	}
	key, err := ssh.ParsePrivateKey(b)
	if err != nil {
		return nil
	}
	return ssh.PublicKeys(key)
}
