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

// gocdk-secrets demonstrates the use of the Go CDK secrets package in a
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
	"gocloud.dev/secrets"

	// Import the secrets driver packages we want to be able to open.
	_ "gocloud.dev/secrets/awskms"
	_ "gocloud.dev/secrets/azurekeyvault"
	_ "gocloud.dev/secrets/gcpkms"
	_ "gocloud.dev/secrets/hashivault"
	_ "gocloud.dev/secrets/localsecrets"
)

const helpSuffix = `

  See https://gocloud.dev/concepts/urls/ for more background on
  Go CDK URLs, and sub-packages under gocloud.dev/secrets
  (https://godoc.org/gocloud.dev/secrets#pkg-subdirectories)
  for details on the secrets.Keeper URL format.
`

func main() {
	os.Exit(run())
}

func run() int {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(&decryptCmd{}, "")
	subcommands.Register(&encryptCmd{}, "")
	log.SetFlags(0)
	log.SetPrefix("gocdk-secrets: ")
	flag.Parse()
	return int(subcommands.Execute(context.Background()))
}

type decryptCmd struct {
	base64in  bool
	base64out bool
}

func (*decryptCmd) Name() string     { return "decrypt" }
func (*decryptCmd) Synopsis() string { return "Decrypt data" }
func (*decryptCmd) Usage() string {
	return `decrypt [-base64in] [-base64out] <keeper URL> <ciphertext>

  Decrypt the ciphertext using <keeper URL> and print the result to stdout.

  Example:
    gocdk-secrets decrypt stringkey://mykey nzam9AJHqH1sqeEr1ZLMbWOf4pp5NRHKYBx/h8loARL83+CBc0WPh8dYzHfccQYFUQ==` + helpSuffix
}

func (cmd *decryptCmd) SetFlags(f *flag.FlagSet) {
	f.BoolVar(&cmd.base64in, "base64in", true, "the ciphertext is base64 encoded")
	f.BoolVar(&cmd.base64out, "base64out", false, "the resulting plaintext should be base64 encoded before printing it out")
}

func (cmd *decryptCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if f.NArg() != 2 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	keeperURL := f.Arg(0)
	ciphertext := f.Arg(1)

	cipher := []byte(ciphertext)
	if cmd.base64in {
		var err error
		cipher, err = base64.StdEncoding.DecodeString(ciphertext)
		if err != nil {
			log.Printf("Failed to base64 decode ciphertext: %v\n", err)
			return subcommands.ExitFailure
		}
	}

	// Open a *secrets.Keeper using the keeperURL.
	keeper, err := secrets.OpenKeeper(ctx, keeperURL)
	if err != nil {
		log.Printf("Failed to open keeper: %v\n", err)
		return subcommands.ExitFailure
	}
	defer keeper.Close()

	plain, err := keeper.Decrypt(ctx, cipher)
	if err != nil {
		log.Printf("Failed to decrypt: %v\n", err)
		return subcommands.ExitFailure
	}

	plaintext := string(plain)
	if cmd.base64out {
		plaintext = base64.StdEncoding.EncodeToString(plain)
	}
	fmt.Println(plaintext)

	return subcommands.ExitSuccess
}

type encryptCmd struct {
	base64in  bool
	base64out bool
}

func (*encryptCmd) Name() string     { return "encrypt" }
func (*encryptCmd) Synopsis() string { return "Encrypt data" }
func (*encryptCmd) Usage() string {
	return `encrypt [-base64in] [-base64out] <keeper URL> <plaintext>

  Encrypt the plaintext using <keeper URL> and print the result to stdout.

  Example:
    gocdk-secrets encrypt --base64out stringkey://mykey my-plaintext` + helpSuffix
}

func (cmd *encryptCmd) SetFlags(f *flag.FlagSet) {
	f.BoolVar(&cmd.base64in, "base64in", false, "the plaintext is base64-encoded")
	f.BoolVar(&cmd.base64out, "base64out", true, "the resulting ciphertext should be base64-encoded before printing it out")
}

func (cmd *encryptCmd) Execute(ctx context.Context, f *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if f.NArg() != 2 {
		f.Usage()
		return subcommands.ExitUsageError
	}
	keeperURL := f.Arg(0)
	plaintext := f.Arg(1)

	plain := []byte(plaintext)
	if cmd.base64in {
		var err error
		plain, err = base64.StdEncoding.DecodeString(plaintext)
		if err != nil {
			log.Printf("Failed to base64 decode plaintext: %v\n", err)
			return subcommands.ExitFailure
		}
	}

	// Open a *secrets.Keeper using the keeperURL.
	keeper, err := secrets.OpenKeeper(ctx, keeperURL)
	if err != nil {
		log.Printf("Failed to open keeper: %v\n", err)
		return subcommands.ExitFailure
	}
	defer keeper.Close()

	cipher, err := keeper.Encrypt(ctx, plain)
	if err != nil {
		log.Printf("Failed to encrypt: %v\n", err)
		return subcommands.ExitFailure
	}

	ciphertext := string(cipher)
	if cmd.base64out {
		ciphertext = base64.StdEncoding.EncodeToString(cipher)
	}
	fmt.Println(ciphertext)

	return subcommands.ExitSuccess
}
