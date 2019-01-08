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

package s3blob_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws/session"
	"gocloud.dev/blob"
	"gocloud.dev/blob/s3blob"
)

func Example() {
	// Set the environment variables AWS uses to load the session.
	// https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
	os.Setenv("AWS_ACCESS_KEY", "myaccesskey")
	os.Setenv("AWS_SECRET_KEY", "mysecretkey")
	os.Setenv("AWS_REGION", "us-east-1")

	// Establish an AWS session.
	session, err := session.NewSession(nil)
	if err != nil {
		log.Fatal(err)
	}

	// Create a *blob.Bucket.
	ctx := context.Background()
	b, err := s3blob.OpenBucket(ctx, session, "my-bucket", nil)
	if err != nil {
		log.Fatal(err)
	}
	_, err = b.ReadAll(ctx, "my-key")
	if err != nil {
		// This is expected due to the fake credentials we used above.
		fmt.Println("ReadAll failed due to invalid credentials")
	}

	// Output:
	// ReadAll failed due to invalid credentials
}

func Example_open() {
	_, _ = blob.Open(context.Background(), "s3://my-bucket")

	// Output:
}
