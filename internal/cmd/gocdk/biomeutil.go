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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/xerrors"
)

const biomeConfigFileName = "biome.json"

// biomeConfig is the parsed configuration from a biome.json file.
type biomeConfig struct {
	ServeEnabled *bool   `json:"serve_enabled,omitempty"`
	Launcher     *string `json:"launcher,omitempty"`
}

// biomesRootDir returns the path to the biomes directory.
func biomesRootDir(moduleRoot string) string {
	return filepath.Join(moduleRoot, "biomes")
}

// biomeDir returns the path to the named biome.
//
// It returns an error if the named biome does not exist, but still returns
// the path; the caller may ignore the error.
func biomeDir(moduleRoot, name string) (string, error) {
	dir := filepath.Join(biomesRootDir(moduleRoot), name)
	if _, err := os.Stat(filepath.Join(dir, biomeConfigFileName)); err != nil {
		// TODO(light): Wrap error for formatting chain but not unwrap chain.
		return dir, &biomeNotFoundError{moduleRoot: moduleRoot, biome: name, frame: xerrors.Caller(0), detail: err}
	}
	return dir, nil
}

// readBiomeConfig reads and parses the biome configuration from the filesystem.
// If the configuration file could not be found, readBiomeConfig returns an
// error for which xerrors.As(err, new(*biomeNotFoundError)) returns true.
func readBiomeConfig(moduleRoot, biome string) (*biomeConfig, error) {
	dir, err := biomeDir(moduleRoot, biome)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(filepath.Join(dir, biomeConfigFileName))
	if err != nil {
		return nil, xerrors.Errorf("read biome %s configuration: %w", err)
	}
	config := new(biomeConfig)
	if err := json.Unmarshal(data, config); err != nil {
		return nil, xerrors.Errorf("read biome %s configuration: %w", err)
	}
	return config, nil
}

// refreshBiome refreshes the Terraform state for the biome. This updates
// the outputs.
func refreshBiome(ctx context.Context, moduleRoot, biome string, env []string) error {
	c := exec.CommandContext(ctx, "terraform", "refresh", "-input=false")
	var err error
	c.Dir, err = biomeDir(moduleRoot, biome)
	if err != nil {
		return err
	}
	c.Env = overrideEnv(env, "TF_IN_AUTOMATION=1")
	out, err := c.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return xerrors.Errorf("refresh biome %s:\n%s", biome, out)
		}
		return xerrors.Errorf("refresh biome %s: %w", biome, err)
	}
	return nil
}

// launchEnv returns the list of environment variables to pass to the launched
// server based on a biome's "launch_environment" Terraform output. On success,
// The returned slice will never be nil.
func launchEnv(tfOutput map[string]*tfOutput) ([]string, error) {
	envMap := tfOutput["launch_environment"].mapValue()
	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		if k == "PORT" {
			return nil, xerrors.Errorf("read launch environment: cannot set PORT manually (set by launcher)")
		}
		s, ok := v.(string)
		if !ok {
			return nil, xerrors.Errorf("read launch environment: variable %s is not a string (found %T)", k, v)
		}
		env = append(env, k+"="+s)
	}
	sort.Slice(env, func(i, j int) bool {
		// Sort by key.
		ki := env[i][:strings.IndexByte(env[i], '=')]
		kj := env[j][:strings.IndexByte(env[j], '=')]
		return ki < kj
	})
	return env, nil
}

// tfReadOutput runs `terraform output` on the given directory and returns
// the parsed result.
func tfReadOutput(ctx context.Context, dir string, env []string) (map[string]*tfOutput, error) {
	c := exec.CommandContext(ctx, "terraform", "output", "-json")
	c.Dir = dir
	c.Env = overrideEnv(env, "TF_IN_AUTOMATION=1")
	data, err := c.Output()
	if err != nil {
		return nil, xerrors.Errorf("read terraform output: %w", err)
	}
	var parsed map[string]*tfOutput
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, xerrors.Errorf("read terraform output: %w", err)
	}
	return parsed, nil
}

// tfOutput describes a single output value.
type tfOutput struct {
	Value interface{} `json:"value"`
}

// stringValue returns the output's value if it is a string.
func (out *tfOutput) stringValue() string {
	if out == nil {
		return ""
	}
	v, _ := out.Value.(string)
	return v
}

// mapValue returns the output's value if it is a map.
func (out *tfOutput) mapValue() map[string]interface{} {
	if out == nil {
		return nil
	}
	v, _ := out.Value.(map[string]interface{})
	return v
}

// biomeNotFoundError is an error returned when a biome cannot be found.
type biomeNotFoundError struct {
	moduleRoot string
	biome      string
	frame      xerrors.Frame
	detail     error
}

func (e *biomeNotFoundError) Error() string {
	return fmt.Sprintf("biome %s not found", e.biome)
}

func (e *biomeNotFoundError) FormatError(p xerrors.Printer) error {
	p.Print(e.Error())
	if !p.Detail() {
		return nil
	}
	p.Printf("biome = %q", filepath.Join(biomesRootDir(e.moduleRoot), e.biome))
	e.frame.Format(p)
	return e.detail
}

func (e *biomeNotFoundError) Format(f fmt.State, c rune) {
	xerrors.FormatError(e, f, c)
}
