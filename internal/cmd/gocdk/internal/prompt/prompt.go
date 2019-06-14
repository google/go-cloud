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

package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"gocloud.dev/internal/cmd/gocdk/internal/static"
)

// String prompts the user to input a string. It outputs msg to out, then
// prompts the user to enter a value. If the user just hits return, the
// value defaults to dflt.
// The function only returns an error if the user enters "cancel" and on I/O
// errors.
// TODO(rvangent): Add support for validation?
// TODO(rvangent): Ctrl-C doesn't work; maybe replace cancel; need to take a ctx.
// TODO(rvangent): Revisit how prompts looks (multi-line, etc.).
func String(reader *bufio.Reader, out io.Writer, msg, dflt string) (string, error) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, msg)
	if dflt != "" {
		fmt.Fprintln(out, "  default:", dflt)
	}
	const prompt = `Enter a value, or "cancel": `
	for {
		fmt.Fprintf(out, prompt)
		s, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		s = strings.TrimSuffix(s, "\n")
		if s == "" {
			s = dflt
		}
		if s == "" {
			continue
		}
		if s == "cancel" {
			return "", errors.New("cancelled")
		}
		return s, nil
	}
}

// askIfNeeded checks to see if we've gotten a value for tfLocalName from the
// user before. If so, it returns the previously-entered value. If not, it
// prompts the user to enter a value using promptFn, and saves the result.
//
// Variable values are saved in the "main.tf" file under biomeDir.
func askIfNeeded(biomeDir, tfLocalName string, promptFn func() (string, error)) (string, error) {
	if cur, err := static.ReadLocal(biomeDir, tfLocalName); err != nil {
		return "", err
	} else if cur != "" {
		// We have a previous value!
		return cur, nil
	}
	// Prompt the user.
	val, err := promptFn()
	if err != nil {
		return "", err
	}
	// Save the answer.
	if err := static.Do(biomeDir, nil, static.AddLocal(tfLocalName, val)); err != nil {
		return "", err
	}
	return val, nil
}

// AWSRegionIfNeeded prompts the user for an AWS region if needed.
func AWSRegionIfNeeded(reader *bufio.Reader, out io.Writer, biomeDir string) (string, error) {
	prompt := func() (string, error) {
		return String(reader, out, "Please enter an AWS region.\n  See https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.RegionsAndAvailabilityZones.html for more information.", "us-west-1")
	}
	return askIfNeeded(biomeDir, "aws_region", prompt)
}

// AzureLocationIfNeeded prompts the user for an Azure location if needed.
func AzureLocationIfNeeded(reader *bufio.Reader, out io.Writer, biomeDir string) (string, error) {
	prompt := func() (string, error) {
		return String(reader, out, "Please enter an Azure location.\n  See https://azure.microsoft.com/en-us/global-infrastructure/locations for more information.", "westus")
	}
	return askIfNeeded(biomeDir, "azure_location", prompt)
}

// GCPProjectID prompts the user for a GCP project ID.
func GCPProjectID(reader *bufio.Reader, out io.Writer) (string, error) {
	return String(reader, out, `Please enter your Google Cloud project ID (e.g. "example-project-123456")`, "")
}

// GCPProjectIDIfNeeded prompts the user for a GCP project ID if needed.
func GCPProjectIDIfNeeded(reader *bufio.Reader, out io.Writer, biomeDir string) (string, error) {
	return askIfNeeded(biomeDir, "gcp_project", func() (string, error) { return GCPProjectID(reader, out) })
}

// GCPStorageLocationIfNeeded prompts the user for a GCP storage location if needed.
func GCPStorageLocationIfNeeded(reader *bufio.Reader, out io.Writer, biomeDir string) (string, error) {
	prompt := func() (string, error) {
		return String(reader, out, "Please enter a GCP storage location.\n  See https://cloud.google.com/storage/docs/locations for more information.", "US")
	}
	return askIfNeeded(biomeDir, "gcp_storage_location", prompt)
}
