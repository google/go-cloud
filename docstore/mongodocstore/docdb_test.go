package mongodocstore

import (
	"context"
	"fmt"
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gocloud.dev/docstore/drivertest"
	"gocloud.dev/internal/testing/setup"
)

const connectionStringTemplate = "mongodb://%s:%s@%s/?connect=direct&connectTimeoutMS=3000"

// To run the conformance tests against Amazon DocumentDB:
//
//  1. Run `terraform apply` in awsdocdb directory to provision a docdb cluster and an EC2 instance.
//  2. Run the command provided by the terraform output string to setup port-forwarding.
//  3. Set the following environment variables and run this test with `-record` flag.

var (
	username = os.Getenv("AWSDOCDB_USERNAME")
	password = os.Getenv("AWSDOCDB_PASSWORD")
	endpoint = os.Getenv("AWSDOCDB_ENDPOINT") // optional, default to localhost:27019
)

func TestConformanceDocDB(t *testing.T) {
	if !*setup.Record {
		t.Skip("replaying is not yet supported for Amazon DocumentDB")
	}
	if username == "" || password == "" {
		t.Fatal("environment not setup to run DocDB test")
	}

	client := newDocDBTestClient(t)
	defer client.Disconnect(context.Background())

	newHarness := func(context.Context, *testing.T) (drivertest.Harness, error) {
		return &harness{db: client.Database(dbName)}, nil
	}
	drivertest.RunConformanceTests(t, newHarness, codecTester{}, []drivertest.AsTest{verifyAs{}})
}

func newDocDBTestClient(t *testing.T) *mongo.Client {
	ctx := context.Background()
	if endpoint == "" {
		endpoint = "localhost:27019"
	}
	connectionURI := fmt.Sprintf(connectionStringTemplate, username, password, endpoint)

	o := options.Client().ApplyURI(connectionURI)
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	client, err := mongo.NewClient(o)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	err = client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to cluster: %v", err)
	}

	// Force a connection to verify our connection string
	err = client.Ping(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to ping cluster: %v", err)
	}
	return client
}
