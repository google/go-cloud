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

// Package azurecloud contains Wire providers for Azure services.
package azurecloud // import "gocloud.dev/azure/azurecloud"

import (
	"github.com/google/wire"
	"gocloud.dev/blob/azureblob"
	"gocloud.dev/secrets/azurekeyvault"
)

// Azure is a Wire provider set that includes the default wiring for all
// Microsoft Azure services in this repository, but does not include
// credentials. Individual services may require additional configuration.
var Azure = wire.NewSet(
	azurekeyvault.Set,
	azureblob.Set,
)
