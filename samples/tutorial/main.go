// Command upload saves files to blob storage on GCP and AWS.
package main

import (
	"context"
	"flag"
	"io/ioutil"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/gcsblob"
	"github.com/google/go-cloud/blob/s3blob"
	"github.com/google/go-cloud/gcp"
)

func main() {
	// Define our input.
	cloud := flag.String("cloud", "", "Cloud storage to use")
	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatal("Failed to provide file to upload")
	}
	file := flag.Arg(0)

	ctx := context.Background()
	// Open a connection to the bucket.
	var (
		b   *blob.Bucket
		err error
	)
	switch *cloud {
	case "gcp":
		// DefaultCredentials assumes a user has logged in with gcloud.
		// See here for more information:
		// https://cloud.google.com/docs/authentication/getting-started
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			log.Fatalf("Failed to create default credentials for GCP: %s", err)
		}
		c, err := gcp.NewHTTPClient(gcp.DefaultTransport(), gcp.CredentialsTokenSource(creds))
		if err != nil {
			log.Fatalf("Failed to create HTTP client: %s", err)
		}
		// The bucket name must be globally unique.
		b, err = gcsblob.NewBucket(ctx, "my-cool-bucket", c)
		if err != nil {
			log.Fatalf("Failed to connect to bucket: %s", err)
		}
	case "aws":
		c := &aws.Config{
			Region: aws.String("us-east-2"), // or wherever the bucket is
			// credentials.NewEnvCredentials assumes two environment variables are
			// present:
			// 1. AWS_ACCESS_KEY_ID, and
			// 2. AWS_SECRET_ACCESS_KEY.
			Credentials: credentials.NewEnvCredentials(),
		}
		s := session.Must(session.NewSession(c))
		b, err = s3blob.NewBucket(ctx, s, "my-cool-bucket")
		if err != nil {
			log.Fatalf("Failed to connect to S3 bucket: %s", err)
		}
	default:
		log.Fatalf("Failed to recognize cloud. Want gcp or aws, got: %s", *cloud)
	}

	// Prepare the file for upload.
	data, err := ioutil.ReadFile(file)
	if err != nil {
		log.Fatalf("Failed to read file: %s", err)
	}

	w, err := b.NewWriter(ctx, file, nil)
	if err != nil {
		log.Fatalf("Failed to obtain writer: %s", err)
	}
	_, err = w.Write(data)
	if err != nil {
		log.Fatalf("Failed to write to bucket: %s", err)
	}
	if err := w.Close(); err != nil {
		log.Fatalf("Failed to close: %s", err)
	}
}
