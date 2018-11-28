// Copyright 2018 The Go Cloud Authors
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

// Package paramstore provides a runtimevar.Driver implementation
// that reads variables from AWS Systems Manager Parameter Store.
//
// Construct a Client, then use NewVariable to construct any number of
// runtimevar.Variable objects.
package paramstore

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/ssm"
)

// Client stores long-lived variables for connecting to Parameter Store.
type Client struct {
	sess client.ConfigProvider
}

// NewClient returns a constructed Client with the required values.
func NewClient(p client.ConfigProvider) *Client {
	return &Client{sess: p}
}

// NewVariable constructs a runtimevar.Variable object with this package as the driver
// implementation.
func (c *Client) NewVariable(name string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	w, err := c.newWatcher(name, decoder, opts)
	if err != nil {
		return nil, err
	}
	return runtimevar.New(w), nil
}

func (c *Client) newWatcher(name string, decoder *runtimevar.Decoder, opts *Options) (*watcher, error) {
	if opts == nil {
		opts = &Options{}
	}
	return &watcher{
		sess:    c.sess,
		name:    name,
		wait:    driver.WaitDuration(opts.WaitDuration),
		decoder: decoder,
	}, nil
}

// Options sets options.
type Options struct {
	// WaitDuration controls how quickly Watch polls. Defaults to 30 seconds.
	WaitDuration time.Duration
}

// state implements driver.State.
type state struct {
	val        interface{}
	updateTime time.Time
	version    int64
	err        error
}

func (s *state) Value() (interface{}, error) {
	return s.val, s.err
}

func (s *state) UpdateTime() time.Time {
	return s.updateTime
}

// errorState returns a new State with err, unless prevS also represents
// the same error, in which case it returns nil.
func errorState(err error, prevS driver.State) driver.State {
	s := &state{err: err}
	if prevS == nil {
		return s
	}
	prev := prevS.(*state)
	if prev.err == nil {
		// New error.
		return s
	}
	if err == prev.err || err.Error() == prev.err.Error() {
		// Same error, return nil to indicate no change.
		return nil
	}
	var code, prevCode string
	if awsErr, ok := err.(awserr.Error); ok {
		code = awsErr.Code()
	}
	if awsErr, ok := prev.err.(awserr.Error); ok {
		prevCode = awsErr.Code()
	}
	if code != "" && code == prevCode {
		return nil
	}
	return s
}

type watcher struct {
	// sess is the AWS session to use to talk to AWS.
	sess client.ConfigProvider
	// name is the parameter to retrieve.
	name string
	// wait is the amount of time to wait between querying AWS.
	wait time.Duration
	// decoder is the decoder that unmarshals the value in the param.
	decoder *runtimevar.Decoder
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	lastVersion := int64(-1)
	if prev != nil {
		lastVersion = prev.(*state).version
	}
	// Read the variable from the backend.
	p, err := readParam(w.sess, w.name, lastVersion)
	if err != nil {
		return errorState(err, prev), w.wait
	}
	if p == nil {
		// Version hasn't changed.
		return nil, w.wait
	}

	// New value! Decode it.
	val, err := w.decoder.Decode([]byte(p.value))
	if err != nil {
		return errorState(err, prev), w.wait
	}
	return &state{val: val, updateTime: p.updateTime, version: p.version}, w.wait
}

// Close implements driver.Close.
func (w *watcher) Close() error {
	return nil
}

type param struct {
	name       string
	value      string
	version    int64
	updateTime time.Time
}

// readParam returns the named parameter. An error is returned if AWS is unreachable
// or the named parameter isn't found. If the parameter hasn't changed from the
// passed version, returns a nil param and nil error. This saves a
// redundant API call.
func readParam(p client.ConfigProvider, name string, version int64) (*param, error) {
	svc := ssm.New(p)

	getResp, err := svc.GetParameter(&ssm.GetParameterInput{Name: aws.String(name)})
	if err != nil {
		return nil, err
	}
	if getResp.Parameter == nil {
		return nil, fmt.Errorf("unable to get %q parameter", name)
	}
	getP := getResp.Parameter
	if *getP.Version == version {
		return nil, nil
	}

	descResp, err := svc.DescribeParameters(&ssm.DescribeParametersInput{
		Filters: []*ssm.ParametersFilter{
			{Key: aws.String("Name"), Values: []*string{&name}},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(descResp.Parameters) != 1 || *descResp.Parameters[0].Name != name {
		return nil, fmt.Errorf("unable to get single %q parameter", name)
	}
	descP := descResp.Parameters[0]

	return &param{
		name:       *getP.Name,
		value:      *getP.Value,
		version:    *getP.Version,
		updateTime: *descP.LastModifiedDate,
	}, nil
}
