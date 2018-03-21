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

package log

import "context"

// StructuredLogEntry is a log entry containing typed key-value pairs.
//
// Methods of StructuredLogEntry add additional values by name. The order in
// which values are added is not retained.
//
// Once all values are added, Send() or Sync() must be called to submit the
// entry to the logging services.
type StructuredLogEntry interface {
	// Add a string value to this log entry. Returns the same StructuredLogEntry
	// object for chaining.
	String(name string, x string) StructuredLogEntry

	// Add an int32 value to this log entry. Returns the same StructuredLogEntry
	// for chaining.
	Int32(name string, x int32) StructuredLogEntry

	// Send this log entry to all active logging services.
	//
	// A StructureLogEntry sent using Send() may be cached and deferred by the
	// implementation. If the process crashes, a cached StructuredLogEntry may
	// be lost. Use Sync() instead to ensure a StructureLogEntry is sent.
	//
	// Adding additional values to a StructuredLogEntry after calling Send will
	// panic.
	Send()

	// Send this log entry and block until it is received by all active logging
	// services, or return the first error encountered.
	Sync() error
}

type Logger interface {
	// Log logs a string message to all active logging services.
	//
	// The message may be cached and deferred by the implementation. If the
	// process crashes, a cached message may be lost. Use LogSync() instead to
	// ensure s message is sent.
	Log(ctx context.Context, msg string)

	// LogSync logs a string message and blocks until it is received by all
	// active logging services.
	LogSync(ctx context.Context, msg string) error

	// LogStructured creates and returns a new StructuredLogEntry.
	LogStructured(ctx context.Context) StructuredLogEntry
}
