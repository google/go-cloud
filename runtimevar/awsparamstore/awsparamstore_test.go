// Copyright 2018 The Go Cloud Development Kit Authors
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

package awsparamstore

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
)

// This constant records the region used for the last --record.
// If you want to use --record mode,
// 1. Update this constant to your AWS region.
// TODO(issue #300): Use Terraform to get this.
const region = "us-east-2"

type harness struct {
	client *ssm.Client
	closer func()
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	t.Helper()

	cfg, _, done, _ := setup.NewAWSv2Config(context.Background(), t, region)
	return &harness{client: ssm.NewFromConfig(cfg), closer: done}, nil
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	return newWatcher(h.client, name, decoder, nil), nil
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	_, err := h.client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(name),
		Type:      "String",
		Value:     aws.String(string(val)),
		Overwrite: aws.Bool(true),
	})
	return err
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	return h.CreateVariable(ctx, name, val)
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	_, err := h.client.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: aws.String(name)})
	return err
}

func (h *harness) Close() {
	h.closer()
}

func (h *harness) Mutable() bool { return true }

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct {
}

func (verifyAs) Name() string {
	return "verify As"
}

func (v verifyAs) SnapshotCheck(s *runtimevar.Snapshot) error {
	var getParam *ssm.GetParameterOutput
	if !s.As(&getParam) {
		return errors.New("Snapshot.As failed for GetParameterOutput")
	}
	return nil
}

func (va verifyAs) ErrorCheck(v *runtimevar.Variable, err error) error {
	var e smithy.APIError
	if !v.ErrorAs(err, &e) {
		return errors.New("Keeper.ErrorAs failed")
	}
	return nil
}

// Paramstore-specific tests.

func TestOpenVariable(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"awsparamstore://myvar", false},
		// OK, setting region.
		{"awsparamstore://myvar?region=us-west-1", false},
		// OK, setting decoder.
		{"awsparamstore://myvar?decoder=string", false},
		// Invalid decoder.
		{"awsparamstore://myvar?decoder=notadecoder", true},
		// OK, setting wait.
		{"awsparamstore://myvar?wait=2m", false},
		// Invalid wait.
		{"awsparamstore://myvar?wait=x", true},
		// Invalid parameter.
		{"awsparamstore://myvar?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		v, err := runtimevar.OpenVariable(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if err == nil {
			v.Close()
		}
	}
}
