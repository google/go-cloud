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

// +build aws

package config

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"

	cloud "github.com/google/go-cloud"
	"github.com/google/go-cloud/log"
	cloudaws "github.com/google/go-cloud/platforms/aws"
)

const ssmServiceName = "ssm"

var ssmClient *ssm.SSM
var ssmClientOnce sync.Once

func init() {
	Register(ssmServiceName, new(ssmProvider))
}

type versionedName struct {
	name    string
	version int64
}

type versionedValue struct {
	value   string
	version int64
}

type ssmConfig struct {
	name      string
	cache     map[string]versionedValue
	cacheMu   sync.Mutex
	invalidCh chan struct{}
}

const configCheckInterval = time.Duration(30 * time.Second)
const maxFailures = 5

func configChecker(config *ssmConfig) {
	failures := 0
	for {
		if failures >= maxFailures {
			log.Printf(context.Background(), "configuration invalidation check failed %d times (max allowed: %d); terminating configuration invalidation check", failures, maxFailures)
		}
		time.Sleep(configCheckInterval)

		// TODO: pagination
		t := true
		i := int64(256)
		req := ssmClient.GetParametersByPathRequest(&ssm.GetParametersByPathInput{
			MaxResults:     &i,
			Path:           &config.name,
			Recursive:      &t,
			WithDecryption: &t,
		})
		res, err := req.Send()
		if err != nil {
			log.Printf(context.Background(), "configuration invalidation check failed: %+v", err)
			failures++
			continue
		}

		invalid := false
		func() {
			config.cacheMu.Lock()
			defer config.cacheMu.Unlock()
			m := make(map[string]versionedValue)
			for _, p := range res.Parameters {
				m[*p.Name] = versionedValue{value: *p.Value, version: *p.Version}
				v, ok := config.cache[*p.Name]
				// Check for updates
				if ok {
					if *p.Version > v.version {
						invalid = true
						return
					}
				}
			}
			// Check for deletions
			for name := range config.cache {
				if _, ok := m[name]; !ok {
					invalid = true
					return
				}
			}
		}()
		if invalid {
			close(config.invalidCh)
			return
		}
	} // for
}

func (c *ssmConfig) Get(name string) (string, error) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if val, ok := c.cache[name]; ok {
		return val.value, nil
	}
	t := true
	fullName := filepath.Join("/", c.name, name)
	req := ssmClient.GetParameterRequest(&ssm.GetParameterInput{
		Name:           &fullName,
		WithDecryption: &t,
	})
	res, err := req.Send()
	if err != nil {
		return "", err
	}
	c.cache[name] = versionedValue{
		value:   *res.Parameter.Value,
		version: *res.Parameter.Version,
	}
	return *res.Parameter.Value, nil
}

type ssmProvider struct{}

func (p *ssmProvider) GetConfig(ctx context.Context, name string) (Interface, chan struct{}, error) {
	ssmClientOnce.Do(func() {
		ssmClient = ssm.New(cloudaws.Config())
	})
	service, configName, err := cloud.Service(name)
	if err != nil {
		return nil, nil, err
	}
	if service != ssmServiceName {
		panic("bad provider routing: " + name)
	}
	r := &ssmConfig{
		name:      configName,
		cache:     make(map[string]versionedValue),
		invalidCh: make(chan struct{}),
	}
	go configChecker(r)
	return r, r.invalidCh, nil
}
