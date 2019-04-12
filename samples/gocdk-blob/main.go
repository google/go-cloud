// Copyright 2019 The Go Cloud Development Kit Authors
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

// gocdk-blob demonstrates the use of the Go CDK blob package in a
// simple command-line application.
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"

	"github.com/google/subcommands"
	"gocloud.dev/blob"

	// Import the blob driver packages we want to be able to open.
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
)

func main() {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(&downloadCmd{}, "")
	log.SetFlags(0)
	log.SetPrefix("gocdk-blob: ")
	flag.Parse()
	os.Exit(int(subcommands.Execute(context.Background())))
}

type downloadCmd struct{}

func (*downloadCmd) Name() string     { return "download" }
func (*downloadCmd) Synopsis() string { return "Download a blob to local disk" }
func (*downloadCmd) Usage() string {
	return `download <bucket URL> <key> <target file>

  Download the blob <key> from <bucket URL> and write it to <target file>.

  Example:
    gocdk-blob download gs://mybucket my/gcs/file /tmp/file

  See https://godoc.org/gocloud.dev#hdr-URLs for more background on
  Go CDK URLs, and sub-packages under gocloud.dev/blob
  (https://godoc.org/gocloud.dev/blob#pkg-subdirectories)
  for details on the blob.Bucket URL format.
`
}

func (s *downloadCmd) SetFlags(f *flag.FlagSet) {}

func (s *downloadCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 3 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	bucketURL := f.Arg(0)
	blobKey := f.Arg(1)
	targetFile := f.Arg(2)

	// Open a *blob.Bucket using the bucketURL.
	b, err := blob.OpenBucket(ctx, bucketURL)
	if err != nil {
		log.Printf("Failed to open bucket: %v\n", err)
		return subcommands.ExitFailure
	}
	defer b.Close()

	// Open a *blob.Reader for the blob at blobKey.
	reader, err := b.NewReader(ctx, blobKey, nil)
	if err != nil {
		log.Printf("Failed to read %q: %v\n", blobKey, err)
		return subcommands.ExitFailure
	}
	defer reader.Close()

	// Open the output file.
	outFile, err := os.Create(targetFile)
	if err != nil {
		log.Printf("Failed to create output file %q: %v\n", targetFile, err)
		return subcommands.ExitFailure
	}
	defer outFile.Close()

	// Copy the data.
	_, err = io.Copy(outFile, reader)
	if err != nil {
		log.Printf("Failed to copy data: %v\n", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}
