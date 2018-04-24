// Copyright 2018 Google LLC
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

package gcpconfig_test

import (
	"context"
	"log"

	"github.com/google/go-cloud/runtimeconfig/gcpconfig"
)

// MyAppConfig is the unmarshaled type for configuration stored in key
// "projects/projectID/configs/configName/variables/appConfig".
type MyAppConfig struct {
	MsgOfTheDay string `json:"msg_of_the_day"`
}

func ExampleClient_NewConfig() {
	ctx := context.Background()
	client, err := gcpconfig.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	name := gcpconfig.ResourceName{
		ProjectID: "projectID",
		Config:    "configName",
		Variable:  "appConfig",
	}
	cfg, err := client.NewConfig(ctx, name, &MyAppConfig{}, nil)
	if err != nil {
		log.Fatalf("Error in constructing Config: %v", err)
	}
	defer cfg.Close()
}
