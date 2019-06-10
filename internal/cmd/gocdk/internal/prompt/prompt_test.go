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

package prompt

import (
	"errors"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/cmd/gocdk/internal/static"
)

func TestString(t *testing.T) {
	tests := []struct {
		Description string
		In          string
		Default     string

		Want    string
		WantErr bool
	}{
		{
			Description: "cancel",
			In:          "cancel\n",
			WantErr:     true,
		},
		{
			Description: "EOF",
			In:          "\n",
			WantErr:     true,
		},
		{
			Description: "valid",
			In:          "foo\n",
			Want:        "foo",
		},
		{
			Description: "hit CR a few times before getting a good answer",
			In:          "\n\n\n\nfoo\n",
			Want:        "foo",
		},
		{
			Description: "using default",
			Default:     "foo",
			In:          "\n",
			Want:        "foo",
		},
	}
	for _, test := range tests {
		t.Run(test.Description, func(t *testing.T) {
			in := strings.NewReader(test.In)
			got, err := String(in, ioutil.Discard, "unused message", test.Default)
			if (err != nil) != test.WantErr {
				t.Fatalf("got err %v, want err? %v", err, test.WantErr)
			}
			if diff := cmp.Diff(got, test.Want); diff != "" {
				t.Errorf("diff: %s", diff)
			}
		})
	}
}

func TestAskIfNeeded(t *testing.T) {
	const varName = "myvariable_name"
	tests := []struct {
		Description     string
		PromptReturn    string
		PromptReturnErr error
		Existing        string

		Want    string
		WantErr bool
	}{
		{
			Description:     "cancel",
			PromptReturnErr: errors.New("cancelled"),
			WantErr:         true,
		},
		{
			Description:  "no existing value",
			PromptReturn: "foo",
			Want:         "foo",
		},
		{
			Description: "existing value",
			Existing:    "foo",
			Want:        "foo",
		},
	}
	for _, test := range tests {
		t.Run(test.Description, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "static-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)

			// Create a default main.tf file.
			if err := static.Do(dir, nil, static.CopyFile("/biome/main.tf", "main.tf")); err != nil {
				t.Fatal(err)
			}
			// If the test wants an existing value for the variable, add it.
			if test.Existing != "" {
				if err := static.Do(dir, nil, static.AddLocal(varName, test.Existing)); err != nil {
					t.Fatal(err)
				}
			}

			promptFn := func() (string, error) { return test.PromptReturn, test.PromptReturnErr }
			got, err := askIfNeeded(dir, varName, promptFn)
			if (err != nil) != test.WantErr {
				t.Fatalf("got err %v, want err? %v", err, test.WantErr)
			}
			// Make sure we got the right value.
			if diff := cmp.Diff(got, test.Want); diff != "" {
				t.Errorf("diff: %s", diff)
			}
			// We should also be able to get the same value using ReadLocal.
			gotFromRead, _ := static.ReadLocal(dir, varName)
			if diff := cmp.Diff(gotFromRead, test.Want); diff != "" {
				t.Errorf("diff from ReadLocal: %s", diff)
			}
		})
	}
}
