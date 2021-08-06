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

package awskmsv2_test

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"gocloud.dev/secrets"
	"gocloud.dev/secrets/awskmsv2"
)

func ExampleOpenKeeper() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.

	// Load an AWS Config and use it to create a service client.
	// See https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/ for more info.
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}
	client := kms.NewFromConfig(cfg)

	// Construct a *secrets.Keeper.
	keeper := awskmsv2.OpenKeeper(client, "alias/test-secrets", nil)
	defer keeper.Close()
}

func Example_openFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/secrets/awskmsv2"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// Use one of the following:

	// 1. By ID.
	keeperByID, err := secrets.OpenKeeper(ctx,
		"awskmsv2://1234abcd-12ab-34cd-56ef-1234567890ab?region=us-east-1")
	if err != nil {
		log.Fatal(err)
	}
	defer keeperByID.Close()

	// 2. By alias.
	keeperByAlias, err := secrets.OpenKeeper(ctx,
		"awskmsv2://alias/ExampleAlias?region=us-east-1")
	if err != nil {
		log.Fatal(err)
	}
	defer keeperByAlias.Close()

	// 3. By ARN.
	const arn = "arn:aws:kms:us-east-1:111122223333:key/" +
		"1234abcd-12ab-34bc-56ef-1234567890ab"
	keeperByARN, err := secrets.OpenKeeper(ctx,
		"awskmsv2://"+arn+"?region=us-east-1")
	if err != nil {
		log.Fatal(err)
	}
	defer keeperByARN.Close()
}
