package main

import (
	"context"
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/google/go-x-cloud/blob"
	"github.com/google/go-x-cloud/blob/gcsblob"
	"github.com/google/go-x-cloud/blob/s3blob"
	"github.com/google/go-x-cloud/gcp"
)

func main() {
	cloud := flag.String("cloud", "", "Cloud storage to use")
	file := flag.String("file", "", "Path to the file to upload")
	flag.Parse()

	var (
		b   *blob.Bucket
		err error
	)
	switch *cloud {
	case "gcp":
		// gcp.DefaultCredentials assumes GOOGLE_APPLICATION_CREDENTIALS is present
		// in tne environment and points to a service-account.json file.
		creds, err := gcp.DefaultCredentials(context.Background())
		if err != nil {
			log.Fatalf("Failed to create default credentials for GCP: %s", err)
		}
		c, err := gcp.NewHTTPClient(gcp.DefaultTransport(), gcp.CredentialsTokenSource(creds))
		if err != nil {
			log.Fatalf("Failed to create HTTP client: %s", err)
		}
		b, err = gcsblob.NewBucket(context.Background(), "enocom-tutorial-bucket", c)
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
		b, err = s3blob.NewBucket(context.Background(), s, "enocom-tutorial-bucket")
		if err != nil {
			log.Fatalf("Failed to connect to S3 bucket: %s", err)
		}
	default:
		log.Fatalf("Failed to recognize cloud. Want gcp or aws, got: %s", *cloud)
	}

	f, err := os.Open(*file)
	if err != nil {
		log.Fatalf("Failed to open file: %s", err)
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		log.Fatalf("Failed to read file: %s", err)
	}
	if err = f.Close(); err != nil {
		log.Fatalf("Failed to close file: %s", err)
	}

	w, err := b.NewWriter(context.Background(), "gopher", &blob.WriterOptions{
		// png is hard coded here for brevity to avoid dealing with mime type
		// parsing.
		ContentType: "image/png",
	})
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
