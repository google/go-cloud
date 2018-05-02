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
