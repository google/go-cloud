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

package main_test

import (
	context "context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/logging/logadmin"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/xray"
	"github.com/google/go-cloud/tests/internal/testutil"
	"golang.org/x/crypto/ssh"
	"google.golang.org/api/iterator"
)

const (
	requestlogURL = "/requestlog/"
	traceURL      = "/trace/"
)

var (
	hostIP       string
	awsRegion    string
	gcpProjectID string
	sshUser      string
	sshKeyPath   string
)

func init() {
	flag.StringVar(&hostIP, "host-ip", "", "host IP address of the instance")
	flag.StringVar(&awsRegion, "aws-region", "us-west-1", "the region used to run the sample app")
	flag.StringVar(&gcpProjectID, "gcp-project", "", "GCP project used to collect request logs")
	flag.StringVar(&sshUser, "ssh-user", "admin", "user name used to ssh into the EC2 instance")
	flag.StringVar(&sshKeyPath, "key-path", "", "path to the key file")
}

func TestMain(m *testing.M) {
	flag.Parse()
	if hostIP == "" || gcpProjectID == "" {
		log.Println("Test environments need to be setup to run server tests.")
		flag.PrintDefaults()
		return
	}
	if err := startApp(); err != nil {
		log.Fatal("startApp: ", err)
	}
	os.Exit(m.Run())
}

func startApp() error {
	signer, err := signerFromFile(sshKeyPath)
	if err != nil {
		return fmt.Errorf("cannot get key from file %s: %v", sshKeyPath, err)
	}
	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
		Timeout: 30 * time.Second,
	}
	c, err := ssh.Dial("tcp", hostIP+":22", config)
	if err != nil {
		return fmt.Errorf("failed to dial %s:22: %v", hostIP, err)
	}
	session, err := c.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}

	// This close the session channel and the TCP connection, but doesn't terminate
	// the running app. session.Signal() does not terminate it either, see
	// https://github.com/golang/go/issues/4115. This is OK for our use case since
	// we tear down the instance after the test finishes.
	defer func() {
		session.Close()
		c.Close()
	}()
	return session.Start("/home/admin/app 2>&1 | logger")
}

func signerFromFile(path string) (ssh.Signer, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(b)
}

func TestRequestLog(t *testing.T) {
	t.Parallel()
	suf, err := testutil.URLSuffix()
	if err != nil {
		t.Fatal("cannot generate URL suffix:", err)
	}
	p := path.Clean(fmt.Sprintf("/%s/%s", requestlogURL, suf))
	if err := testutil.Retry(t, testutil.Get("http://"+hostIP+":8080"+p)); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	c, err := logadmin.NewClient(ctx, gcpProjectID)
	if err != nil {
		t.Fatalf("error creating logadmin client: %v", err)
	}
	defer c.Close()

	if err := testutil.Retry(t, func() error {
		return readLogEntries(ctx, c, p)
	}); err != nil {
		t.Error(err)
	}
}

func readLogEntries(ctx context.Context, c *logadmin.Client, u string) error {
	iter := c.Entries(context.Background(),
		logadmin.Filter(strconv.Quote(u)),
	)
	_, err := iter.Next()
	if err == iterator.Done {
		return fmt.Errorf("no entry found for request log that matches %q", u)
	}
	if err != nil {
		return err
	}
	return nil
}

func TestTrace(t *testing.T) {
	t.Parallel()
	suf, err := testutil.URLSuffix()
	if err != nil {
		t.Fatal("cannot generate URL suffix:", err)
	}
	p := path.Clean(fmt.Sprintf("/%s/%s", traceURL, suf))
	startTime := time.Now()
	if err := testutil.Retry(t, testutil.Get("http://"+hostIP+":8080"+p)); err != nil {
		t.Fatal(err)
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion),
	})
	if err != nil {
		log.Fatalf("error creating an AWS session: %v\n", err)
	}
	c := xray.New(sess)
	if err := testutil.Retry(t, func() error {
		return readTrace(c, p, startTime)
	}); err != nil {
		t.Error(err)
	}
}

func readTrace(c *xray.XRay, u string, startTime time.Time) error {
	summaries, err := c.GetTraceSummaries(&xray.GetTraceSummariesInput{
		StartTime: aws.Time(startTime),
		EndTime:   aws.Time(time.Now()),
	})
	if err != nil {
		return err
	}
	if len(summaries.TraceSummaries) == 0 {
		return fmt.Errorf("no trace found for %s", u)
	}

	// The above call to get trace summaries cannot find the trace by filtering
	// with the url, so we have to use the id to get individual traces and look
	// into their segments (spans).
	ids := make([]*string, len(summaries.TraceSummaries))
	for i, ts := range summaries.TraceSummaries {
		ids[i] = ts.Id
	}
	traces, err := c.BatchGetTraces(&xray.BatchGetTracesInput{
		TraceIds: ids,
	})
	if err != nil {
		return err
	}
	for _, tr := range traces.Traces {
		for _, seg := range tr.Segments {
			if seg.Document != nil && strings.Contains(*seg.Document, u) {
				return nil
			}
		}
	}
	return fmt.Errorf("no trace found for %s", u)
}
