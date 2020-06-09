// Copyright 2020 The Go Cloud Development Kit Authors
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

package gcsblob

import (
	"context"
	"errors"
	"testing"

	gax "github.com/googleapis/gax-go/v2"
	credentialspb "google.golang.org/genproto/googleapis/iam/credentials/v1"
)

const (
	mockKey       = "key0000"
	mockSignature = "signature"
)

type mockIAMClient struct {
	requestErr error
}

func (m mockIAMClient) SignBlob(context.Context, *credentialspb.SignBlobRequest, ...gax.CallOption) (*credentialspb.SignBlobResponse, error) {
	if m.requestErr != nil {
		return nil, m.requestErr
	}
	return &credentialspb.SignBlobResponse{KeyId: mockKey, SignedBlob: []byte(mockSignature)}, nil
}

func TestIAMCredentialsClient(t *testing.T) {
	tests := []struct {
		name       string
		connectErr error
		mockClient interface {
			SignBlob(context.Context, *credentialspb.SignBlobRequest, ...gax.CallOption) (*credentialspb.SignBlobResponse, error)
		}

		// These are for the produced SignBytesFunc
		input      []byte
		wantOutput []byte
		requestErr error
	}{
		{"happy path: signing", nil,
			mockIAMClient{},
			[]byte("payload"), []byte(mockSignature), nil,
		},
		{"won't connect", errors.New("Missing role: serviceAccountTokenCreator"),
			mockIAMClient{},
			[]byte("payload"), nil, nil,
		},
		{"request fails", nil,
			mockIAMClient{requestErr: context.Canceled},
			[]byte("payload"), nil, context.Canceled,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := credentialsClient{err: test.connectErr, client: test.mockClient}
			makeSignBytesFn := c.CreateMakeSignBytesWith(nil, serviceAccountID)

			signBytesFn := makeSignBytesFn(nil) // Our mocks don't read any context.
			haveOutput, haveErr := signBytesFn(test.input)

			if len(test.wantOutput) > 0 && string(haveOutput) != string(test.wantOutput) {
				t.Errorf("Unexpected output:\n -- have: %v\n -- want: %v",
					string(haveOutput), string(test.wantOutput))
				return
			}

			if test.connectErr == nil && test.requestErr == nil {
				return
			}
			if test.connectErr != nil && haveErr != test.connectErr {
				t.Error("The connection error, a permanent error, has not been returned but should.")
			}
			if test.requestErr != nil && haveErr != test.requestErr {
				t.Error("The per-request error has not been returned but should.")
			}
		})
	}
}
