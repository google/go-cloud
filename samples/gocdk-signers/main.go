// Copyright 2021 The Go Cloud Development Kit Authors
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

// gocdk-signers demonstrates the use of the Go CDK signers package in a
// simple command-line application.
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/google/subcommands"

	"gocloud.dev/signers"

	// Import the signers driver packages we want to be able to open.
	_ "gocloud.dev/signers/awssig"
	_ "gocloud.dev/signers/azuresig"
	_ "gocloud.dev/signers/gcpsig"
	_ "gocloud.dev/signers/localsigners"
)

const helpSuffix = `

  See https://gocloud.dev/concepts/urls/ for more background on
  Go CDK URLs, and sub-packages under gocloud.dev/signers
  (https://godoc.org/gocloud.dev/signers#pkg-subdirectories)
  for details on the signers.Signer URL format.
`

func main() {
	os.Exit(run())
}

func run() int {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(&signCmd{}, "")
	subcommands.Register(&verifyCmd{}, "")
	log.SetFlags(0)
	log.SetPrefix("gocdk-signers: ")
	flag.Parse()
	return int(subcommands.Execute(context.Background()))
}

type signCmd struct {
	base64out bool
}

func (*signCmd) Name() string     { return "sign" }
func (*signCmd) Synopsis() string { return "Sign digest" }
func (*signCmd) Usage() string {
	return `sign [-base64out] <signer URL> <base64-digest>

  Sign the digest using <signer URL> and print the signature to stdout.

  Example:
    gocdk-signers sign --base64out stringkey://mykey 4mWOJs4+gt4O4Nc+brZ5FiDFBLKqL9GRZmdZ/KKUgnM=` + helpSuffix
}

func (cmd *signCmd) SetFlags(f *flag.FlagSet) {
	f.BoolVar(&cmd.base64out, "base64out", true, "the resulting signature should be base64-encoded before printing it out")
}

func (cmd *signCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 2 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	signerURL := f.Arg(0)
	digest, err := base64.StdEncoding.DecodeString(f.Arg(1))
	if err != nil {
		log.Printf("Failed to base64 decode digest: %v\n", err)
		return subcommands.ExitFailure
	}

	// Open a *signers.Signer using the signerURL.
	signer, err := signers.OpenSigner(ctx, signerURL)
	if err != nil {
		log.Printf("Failed to open signer: %v\n", err)
		return subcommands.ExitFailure
	}
	defer signer.Close()

	signature, err := signer.Sign(ctx, digest)
	if err != nil {
		log.Printf("Failed to sign: %v\n", err)
		return subcommands.ExitFailure
	}

	if cmd.base64out {
		fmt.Println(base64.StdEncoding.EncodeToString(signature))
	} else {
		fmt.Print(signature)
	}

	return subcommands.ExitSuccess
}

type verifyCmd struct{}

func (*verifyCmd) Name() string     { return "verify" }
func (*verifyCmd) Synopsis() string { return "Verify signature of a digest" }
func (*verifyCmd) Usage() string {
	return `verify <signer URL> <base64-digest> <base64-signature>

  Verify the signature using <signer URL>.

  Example:
    gocdk-signers verify stringkey://mykey 4mWOJs4+gt4O4Nc+brZ5FiDFBLKqL9GRZmdZ/KKUgnM= sLrEn4Lzg3JYyduuf3xFhg==` + helpSuffix
}

func (cmd *verifyCmd) SetFlags(f *flag.FlagSet) {}

func (cmd *verifyCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 3 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	signerURL := f.Arg(0)
	b64digest := f.Arg(1)
	b64signature := f.Arg(2)

	digest, err := base64.StdEncoding.DecodeString(b64digest)
	if err != nil {
		log.Printf("Failed to base64 decode digest: %v\n", err)
		return subcommands.ExitFailure
	}
	signature, err := base64.StdEncoding.DecodeString(b64signature)
	if err != nil {
		log.Printf("Failed to base64 decode signature: %v\n", err)
		return subcommands.ExitFailure
	}

	// Open a *signers.Signer using the signerURL.
	signer, err := signers.OpenSigner(ctx, signerURL)
	if err != nil {
		log.Printf("Failed to open signer: %v\n", err)
		return subcommands.ExitFailure
	}
	defer signer.Close()

	ok, err := signer.Verify(ctx, digest, signature)
	if err != nil {
		log.Printf("Failed to verify: %v\n", err)
		return subcommands.ExitFailure
	}
	if !ok {
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}
