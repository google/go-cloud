// Copyright 2019 The Go Cloud Development Kit Authors
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

package launcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"

	"gocloud.dev/internal/cmd/gocdk/internal/docker"
	"gocloud.dev/internal/cmd/gocdk/internal/probe"
	"golang.org/x/xerrors"
)

// Input is the input to a launcher.
type Input struct {
	// DockerImage specifies the image name and tag of the local Docker image to
	// deploy. If the local image does not exist, then the launcher should return
	// an error.
	DockerImage string

	// env is the set of additional environment variables to set. It should not
	// include PORT nor should it contain multiple entries for the same variable
	// name.
	Env []string

	// specifier is the set of arguments passed from a biome's Terraform module.
	Specifier map[string]interface{}
}

// Local starts local Docker containers.
type Local struct {
	Logger       *log.Logger
	DockerClient *docker.Client
}

// Launch implements Launcher.Launch.
func (local *Local) Launch(ctx context.Context, input *Input) (*url.URL, error) {
	hostPort := specifierIntValue(input.Specifier, "host_port")
	if hostPort == 0 {
		hostPort = 8080
	} else if hostPort < 0 || hostPort > 65535 {
		return nil, xerrors.Errorf("local launch: host_port is out of range [0, 65535]")
	}
	// TODO(light): Maybe don't remove on exit?
	containerID, err := local.DockerClient.Start(ctx, input.DockerImage, &docker.RunOptions{
		Env:          append(append([]string(nil), input.Env...), "PORT=8080"),
		RemoveOnExit: true,
		Publish:      []string{fmt.Sprintf("%d:8080", hostPort)},
	})
	if err != nil {
		return nil, xerrors.Errorf("local launch: %w", err)
	}

	local.Logger.Printf("Docker container %s started, waiting for healthy...", containerID)
	serveURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", hostPort),
		Path:   "/",
	}
	healthCheckURL := &url.URL{
		Scheme: serveURL.Scheme,
		Host:   serveURL.Host,
		Path:   "/healthz/readiness",
	}
	if err := probe.WaitForHealthy(ctx, healthCheckURL); err != nil {
		// TODO(light): Run `docker stop`.
		return nil, xerrors.Errorf("local launch: %w", err)
	}
	local.Logger.Printf("Container healthy! To shut down, run: docker stop %s", containerID)
	return serveURL, nil
}

// specifierStringValue returns the specifier's value for a key if it is a string.
func specifierStringValue(spec map[string]interface{}, key string) string {
	v, _ := spec[key].(string)
	return v
}

// specifierIntValue returns the specifier's value for a key if it is an integer.
func specifierIntValue(spec map[string]interface{}, key string) int {
	switch v := spec[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	case string:
		i, _ := strconv.ParseInt(v, 10, 0)
		return int(i)
	default:
		return 0
	}
}

// specifierBoolValue returns the specifier's value for a key if it is a boolean.
func specifierBoolValue(spec map[string]interface{}, key string) bool {
	switch v := spec[key].(type) {
	case bool:
		return v
	case string:
		b, _ := strconv.ParseBool(v)
		return b
	default:
		return false
	}
}
