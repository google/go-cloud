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
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os/exec"

	"golang.org/x/xerrors"
)

// Exec runs an arbitrary subprocess to launch.
type Exec struct {
	// Stderr is where the subprocess's stderr will be written to.
	Stderr io.Writer

	// Dir is the directory the subprocess should be run from. For now, this
	// should always be the module root.
	Dir string
}

// Launch implements Launcher.Launch.
func (e *Exec) Launch(ctx context.Context, input *Input) (*url.URL, error) {
	argv := specifierStringArrayValue(input.Specifier, "argv")
	if len(argv) == 0 {
		return nil, xerrors.Errorf("exec launch: specifier argv must be an array of strings with at least one element")
	}
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	c.Stderr = e.Stderr
	c.Dir = e.Dir
	stdin := new(bytes.Buffer)
	if err := json.NewEncoder(stdin).Encode(input); err != nil {
		return nil, xerrors.Errorf("exec launch: %w", err)
	}
	c.Stdin = stdin
	out, err := c.Output()
	if err != nil {
		return nil, xerrors.Errorf("exec launch: %w", err)
	}
	out = bytes.TrimSuffix(out, []byte("\n"))
	u, err := url.Parse(string(out))
	if err != nil {
		return nil, xerrors.Errorf("exec launch: parse output: %w", err)
	}
	return u, nil
}
