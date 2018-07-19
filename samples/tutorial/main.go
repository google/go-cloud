// Command upload saves files to blob storage on GCP and AWS.
package main

import (
	"context"
	"flag"
	"io/ioutil"
	"log"
)

const bucketName = "my-cool-bucket"

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
	b, err := setupGenericBucket(context.Background(), *cloud, bucketName)
	if err != nil {
		log.Fatalf("Failed to setup bucket: %s", err)
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
