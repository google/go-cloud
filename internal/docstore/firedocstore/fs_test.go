// Copyright 2019 The Go Cloud Authors
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

package firedocstore

import (
	"context"
	"testing"

	vkit "cloud.google.com/go/firestore/apiv1"
	"gocloud.dev/internal/docstore/driver"
	"gocloud.dev/internal/docstore/drivertest"
	"gocloud.dev/internal/testing/setup"
	"google.golang.org/api/option"
)

const (
	// projectID is the project ID that was used during the last test run using --record.
	projectID      = "go-cloud-test-216917"
	collectionName = "docstore-test"
	keyName        = "_id"
	endPoint       = "firestore.googleapis.com:443"
)

type harness struct {
	client *vkit.Client
	done   func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	conn, done := setup.NewGCPgRPCConn(ctx, t, endPoint, "docstore")
	client, err := vkit.NewClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		done()
		return nil, err
	}
	return &harness{client, done}, nil
}

func (h *harness) MakeCollection(context.Context) (driver.Collection, error) {
	return newCollection(h.client, projectID, collectionName, keyName), nil
}

func (h *harness) Close() {
	_ = h.client.Close()
	h.done()
}

func TestConformance(t *testing.T) {
	drivertest.MakeUniqueStringDeterministicForTesting(1)
	// TODO(jba): implement a CodecTester. This isn't easy, because the Firestore client
	// (cloud.google.com/go/firestore) doesn't export its codec. We'll have to write a
	// fake gRPC server to capture the protos.
	drivertest.RunConformanceTests(t, newHarness, nil)
}
