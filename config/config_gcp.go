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

// +build gcp

package config

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"sync"

	"golang.org/x/oauth2/google"
	runtimeconfig "google.golang.org/api/runtimeconfig/v1beta1"

	cloud "github.com/vsekhar/go-cloud"
	"github.com/vsekhar/go-cloud/platforms/gcp"
)

const grcServiceName = "grc"

var client *http.Client
var clientOnce sync.Once

func init() {
	Register(grcServiceName, new(grcProvider))
}

type grcProvider struct {
}

func (p *grcProvider) GetConfig(ctx context.Context, name string) (Interface, chan struct{}, error) {
	clientOnce.Do(func() {
		var err error
		client, err = google.DefaultClient(
			ctx,
			"https://www.googleapis.com/auth/cloudruntimeconfig",
		)
		if err != nil {
			panic(err)
		}
	})
	service, configName, err := cloud.Service(name)
	if err != nil {
		return nil, nil, err
	}
	if service != grcServiceName {
		panic("bad provider routing: " + name)
	}
	svc, err := runtimeconfig.New(client)
	if err != nil {
		return nil, nil, err
	}
	varsvc := runtimeconfig.NewProjectsConfigsVariablesService(svc)
	r := &grcConfig{
		name:      configName,
		svc:       varsvc,
		cache:     make(map[string]string),
		project:   gcp.Project(),
		invalidCh: make(chan struct{}),
	}
	return r, r.invalidCh, nil
}

type grcConfig struct {
	name    string
	svc     *runtimeconfig.ProjectsConfigsVariablesService
	cache   map[string]string
	cacheMu sync.Mutex
	project string

	// TODO: implement invalidation check
	invalidCh chan struct{}
}

func (g *grcConfig) Get(name string) (string, error) {
	g.cacheMu.Lock()
	defer g.cacheMu.Unlock()
	if val, ok := g.cache[name]; ok {
		return val, nil
	}
	fullname := fmt.Sprintf("projects/%s/configs/%s/variables/%s", g.project, g.name, name)
	log.Printf("reading config: %s", fullname)
	call := g.svc.Get(fullname)
	r, err := call.Do()
	if err != nil {
		return "", err
	}
	log.Printf("Encoded value: %s", r.Value)
	v, err := base64.StdEncoding.DecodeString(r.Value)
	if err != nil {
		return "", err
	}
	g.cache[name] = string(v)
	return string(v), nil
}
