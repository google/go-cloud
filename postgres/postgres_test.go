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

package postgres

import (
	"context"
	"testing"
)

func TestOpen(t *testing.T) {
	t.Skip("Test not hermetic yet, see https://github.com/google/go-cloud/issues/1429")

	ctx := context.Background()
	dbByUrl, err := Open(ctx, "postgres://postgres@localhost/postgres?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if err := dbByUrl.Ping(); err != nil {
		t.Error("Ping:", err)
	}
	if err := dbByUrl.Close(); err != nil {
		t.Error("Close:", err)
	}
}
