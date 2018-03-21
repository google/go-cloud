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

// +build !plan9

package config

import (
	"context"
	"encoding/json"
	"io/ioutil"

	"github.com/fsnotify/fsnotify"
	cloud "github.com/google/go-cloud"
	"github.com/google/go-cloud/io"
)

const serviceName = "local"

func init() {
	Register("local", new(localProvider))
}

type localConfig struct {
	config map[string]string
}

func (c *localConfig) Get(name string) (string, error) {
	val, ok := c.config[name]
	if !ok {
		return "", NotFound
	}
	return val, nil
}

type localProvider struct {
	path  string
	cache map[string]string
}

func (p *localProvider) GetConfig(ctx context.Context, name string) (Interface, chan struct{}, error) {
	service, path, err := cloud.Service(name)
	if err != nil {
		return nil, nil, err
	}
	if service != serviceName {
		panic("bad handler routing: " + serviceName)
	}

	b, err := io.NewBlob(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	// watch the file
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}
	if err = watcher.Add(path); err != nil {
		return nil, nil, err
	}
	mask := fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename
	doneCh := make(chan struct{})
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&mask > 0 {
					watcher.Close()
					close(doneCh)
					return
				}
			case <-watcher.Errors:
				watcher.Close()
				close(doneCh)
				return
			}
		}
	}()

	// Parse JSON
	r, err := b.NewReader(ctx)
	if err != nil {
		return nil, nil, err
	}
	data, err := ioutil.ReadAll(r)
	c := new(localConfig)
	c.config = make(map[string]string)
	err = json.Unmarshal(data, &c.config)
	if err != nil {
		return nil, nil, err
	}
	return c, doneCh, nil
}
