// Copyright 2018 The Go Cloud Development Kit Authors
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

// Package terraform provides a function to read Terraform output.
package terraform // import "gocloud.dev/internal/testing/terraform"

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// ReadOutput runs `terraform output` on the given directory and returns
// the parsed result.
func ReadOutput(dir string) (map[string]Output, error) {
	c := exec.Command("terraform", "output", "-json")
	c.Dir = dir
	data, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("read terraform output: %v", err)
	}
	var parsed map[string]Output
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("read terraform output: %v", err)
	}
	return parsed, nil
}

// Output describes a single output value.
type Output struct {
	Type      string      `json:"type"` // one of "string", "list", or "map"
	Sensitive bool        `json:"sensitive"`
	Value     interface{} `json:"value"`
}
