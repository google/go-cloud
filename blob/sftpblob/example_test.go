// Copyright 2026 The Go Cloud Development Kit Authors
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

package sftpblob_test

import (
	"context"
	"fmt"
	"log"

	"github.com/pkg/sftp"
	"gocloud.dev/blob"
	"gocloud.dev/blob/sftpblob"
	"golang.org/x/crypto/ssh"
)

func ExampleOpenBucket() {
	// Provide your SSH configuration
	config := &ssh.ClientConfig{
		User: "username",
		Auth: []ssh.AuthMethod{
			ssh.Password("password123"),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Dial the SSH connection
	sshClient, err := ssh.Dial("tcp", "example.com:22", config)
	if err != nil {
		log.Fatal(err)
	}
	defer sshClient.Close()

	// Initialize the SFTP client
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		log.Fatal(err)
	}

	// Create an sftp-based bucket pointing to a remote directory
	bucket, err := sftpblob.OpenBucket(sftpClient, "/remote/path/to/bucket", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer bucket.Close()

	// Use bucket
	ctx := context.Background()
	err = bucket.WriteAll(ctx, "hello.txt", []byte("hello sftp"), nil)
	if err != nil {
		log.Fatal(err)
	}
}

func Example_openBucketFromURL() {
	ctx := context.Background()

	// URL format: sftp://username:password@hostname:port/path/to/directory
	// This uses the default SSH configuration and relies on ssh-agent if no password is provided.
	b, err := blob.OpenBucket(ctx, "sftp://user:pass@example.com/remote/path/")
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	// Now we can use b to read or write files to the remote container.
	err = b.WriteAll(ctx, "my-key", []byte("hello world"), nil)
	if err != nil {
		log.Fatal(err)
	}
	data, err := b.ReadAll(ctx, "my-key")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
}
