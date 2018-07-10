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

// Package paramstore reads parameters to the AWS Systems Manager Parameter Store.
package paramstore

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"
	"github.com/google/go-cloud/runtimevar/internal"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/ssm"
)

// defaultWait is the default amount of time for a watcher to make a new AWS
// API call.
// Change the docstring for NewVariable if this time is modified.
const defaultWait = 30 * time.Second

// Client stores long-lived variables for connecting to Parameter Store.
type Client struct {
	sess client.ConfigProvider
}

// NewClient returns a constructed Client with the required values.
func NewClient(ctx context.Context, p client.ConfigProvider) *Client {
	return &Client{sess: p}
}

// NewVariable constructs a runtimevar.Variable object with this package as the driver
// implementation.
// If WaitTime is not set the polling time is set to 30 seconds.
func (c *Client) NewVariable(ctx context.Context, name string, decoder *runtimevar.Decoder, opts *WatchOptions) (*runtimevar.Variable, error) {
	if opts == nil {
		opts = &WatchOptions{}
	}
	waitTime := opts.WaitTime
	switch {
	case waitTime == 0:
		waitTime = defaultWait
	case waitTime < 0:
		return nil, fmt.Errorf("cannot have negative WaitTime option value: %v", waitTime)
	}

	return runtimevar.New(&watcher{
		sess:        c.sess,
		name:        name,
		waitTime:    waitTime,
		decoder:     decoder,
		lastVersion: -1,
	}), nil
}

// WatchOptions provide optional configurations to the Watcher.
type WatchOptions struct {
	// WaitTime controls the frequency of making an HTTP call and checking for
	// updates by the Watch method. The smaller the value, the higher the frequency
	// of making calls, which also means a faster rate of hitting the API quota.
	// If this option is not set or set to 0, it uses a default value.
	WaitTime time.Duration
}

type watcher struct {
	// sess is the AWS session to use to talk to AWS.
	sess client.ConfigProvider
	// name is the parameter to retrieve.
	name string
	// waitTime is the amount of time to wait between querying AWS.
	waitTime time.Duration
	// decoder is the decoder that unmarshals the value in the param.
	decoder     *runtimevar.Decoder
	lastVersion int64
}

// WatchVariable begins watching the given parameter and waiting for it to change.
// The function will block until either the parameter changes, or the context
// is canceled.
func (w *watcher) WatchVariable(ctx context.Context) (driver.Variable, error) {
	return internal.Pinger(ctx, w.ping, w.waitTime)
}

// Close is a no-op. Cancel the context passed to Watch if watching should end.
func (w *watcher) Close() error {
	return nil
}

// ping hits AWS to read the latest parameter. If nothing had changed,
// the Variable and error will be nil.
func (w *watcher) ping(_ context.Context) (*driver.Variable, error) {
	p, err := readParam(w.sess, w.name, w.lastVersion)
	if err != nil {
		return nil, err
	}
	// version is set to 0 by readParam if the version hasn't changed.
	if p.version != 0 && w.lastVersion != p.version {
		dec := w.decoder
		val, err := dec.Decode([]byte(p.value))
		if err != nil {
			return nil, err
		}
		w.lastVersion = p.version
		return &driver.Variable{Value: val, UpdateTime: p.updateTime}, nil
	}
	return nil, nil
}

type param struct {
	name       string
	value      string
	version    int64
	updateTime time.Time
}

// readParam returns the named parameter. An error is returned if AWS is unreachable
// or the named parameter isn't found. If the parameter hasn't changed from the
// passed version, returns an empty param {} and nil error. This saves a
// redundant API call.
func readParam(p client.ConfigProvider, name string, version int64) (param, error) {
	zeroParam := param{}
	svc := ssm.New(p)

	getResp, err := svc.GetParameter(&ssm.GetParameterInput{Name: aws.String(name)})
	if err != nil || getResp.Parameter == nil {
		return zeroParam, fmt.Errorf("unable to get %q parameter: %v", name, err)
	}
	getP := getResp.Parameter
	if *getP.Version == version {
		return zeroParam, nil
	}

	descResp, err := svc.DescribeParameters(&ssm.DescribeParametersInput{
		Filters: []*ssm.ParametersFilter{
			{Key: aws.String("Name"), Values: []*string{&name}},
		},
	})
	if err != nil {
		return zeroParam, fmt.Errorf("unable to get metadata for %q: %v", name, err)
	}
	if len(descResp.Parameters) != 1 || *descResp.Parameters[0].Name != name {
		return zeroParam, fmt.Errorf("unable to get single %q parameter", name)
	}
	descP := descResp.Parameters[0]

	return param{
		name:       *getP.Name,
		value:      *getP.Value,
		version:    *getP.Version,
		updateTime: *descP.LastModifiedDate,
	}, nil
}
