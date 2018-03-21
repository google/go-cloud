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

package config

import (
	"context"
	"errors"
)

var NotFound = errors.New("configuration not found")

// Interface is any object that can provides access to key-value pairs from a
// configuration.
type Interface interface {
	// Get returns a value for the provided key, or an error.
	Get(key string) (value string, err error)
}

// A provider is an object that provides a configuration. It is only relevant
// to those implementing a new configuration provider.
type Provider interface {
	// GetConfig returns a configuration, a channel that is closed if that
	// configuration becomes invalid, and an error.
	GetConfig(ctx context.Context, name string) (Interface, chan struct{}, error)
}
