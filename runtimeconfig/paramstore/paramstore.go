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

// Package paramstore reads and writes parameters to the AWS Systems Manager Parameter Store.
package paramstore

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/ssm"
)

// ReadParam returns the parameters stored at the given path.
// An error is returned if AWS is unreachable.
func ReadParam(p client.ConfigProvider, path string) ([]*ssm.Parameter, error) {
	svc := ssm.New(p)
	resp, err := svc.GetParametersByPath(&ssm.GetParametersByPathInput{Path: aws.String(path)})
	if err != nil {
		return nil, err
	}

	return resp.Parameters, nil
}
