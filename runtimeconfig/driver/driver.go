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

// Package driver provides the interface for providers of runtimeconfig.  This serves as a contract
// of how the runtimeconfig API uses a provider implementation.
package driver

import (
	"context"
	"time"
)

// Config contains a group of runtime configurations and additional metadata about this
// grouping.
type Config struct {
	Value      interface{}
	UpdateTime time.Time
}

// Watcher watches for updates on a configuration group and returns an updated Config object if
// there are changes.  A Watcher object is associated with a configuration group upon construction.
//
// An application can have more than one Watcher, one for each configuration group.  It is typical
// to only have one Watcher per configuration group.
//
// A Watcher provider can dictate the type of Config.Value if the backend service dictates
// a particular format and type.  If the backend service has the flexibility to store bytes and
// allow clients to dictate the format, it is better for a Watcher provider to allow users to
// dictate the type of Config.Value and a decoding function.  The Watcher provider can use the
// runtimeconfig.Decoder to facilitate the decoding logic.
type Watcher interface {
	// Watch blocks until the configuration group changes, the Context's Done channel closes or an
	// error occurs.
	//
	// If the configuration group has changed, then method should return a Config object with the
	// new value.
	//
	// It is recommended that an implementation should avoid returning the same error of the
	// previous Watch call if the implementation can detect such and treat as no changes had
	// incurred instead.  A sample case is when the configuration is deleted, Watch should return an
	// error upon initial detection.  A succeeding Watch call may decide to block until
	// the configuration source is restored.
	//
	// To stop this function from blocking, caller can passed in Context object constructed via
	// WithCancel and call the cancel function.
	Watch(context.Context) (Config, error)
	// Close cleans up any resources used by the Watcher object.
	Close() error
}
