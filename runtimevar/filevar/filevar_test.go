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

package filevar

import (
	"context"
	"errors"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
	"gocloud.dev/secrets"
	_ "gocloud.dev/secrets/localsecrets"
)

type harness struct {
	dir    string
	closer func()
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	dir, err := ioutil.TempDir("", "filevar_test-")
	if err != nil {
		return nil, err
	}
	return &harness{
		dir:    dir,
		closer: func() { _ = os.RemoveAll(dir) },
	}, nil
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	// filevar uses a goroutine in the background that poll every WaitDuration if
	// the file is deleted. Make this fast for tests.
	return newWatcher(filepath.Join(h.dir, name), decoder, &Options{WaitDuration: 1 * time.Millisecond})
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	// Write to a temporary file and rename; otherwise,
	// Watch can read an empty file during the write.
	tmp, err := ioutil.TempFile(h.dir, "tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(val); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	return os.Rename(tmp.Name(), filepath.Join(h.dir, name))
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	return h.CreateVariable(ctx, name, val)
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	path := filepath.Join(h.dir, name)
	return os.Remove(path)
}

func (h *harness) Close() {
	h.closer()
}

func (h *harness) Mutable() bool { return true }

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (verifyAs) Name() string {
	return "verify As"
}

func (verifyAs) SnapshotCheck(s *runtimevar.Snapshot) error {
	var ss string
	if s.As(&ss) {
		return errors.New("Snapshot.As expected to fail")
	}
	return nil
}

func (verifyAs) ErrorCheck(v *runtimevar.Variable, err error) error {
	var ss string
	if v.ErrorAs(err, &ss) {
		return errors.New("runtimevar.ErrorAs expected to fail")
	}
	return nil
}

// Filevar-specific tests.

func TestOpenVariable(t *testing.T) {
	dir, err := ioutil.TempDir("", "filevar_test-")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		description string
		path        string
		decoder     *runtimevar.Decoder
		want        string
		wantErr     bool
	}{
		{
			description: "empty path results in error",
			decoder:     runtimevar.StringDecoder,
			wantErr:     true,
		},
		{
			description: "empty decoder results in error",
			path:        filepath.Join(dir, "foo.txt"),
			wantErr:     true,
		},
		{
			description: "basic path works",
			path:        filepath.Join(dir, "foo.txt"),
			decoder:     runtimevar.StringDecoder,
			want:        filepath.Join(dir, "foo.txt"),
		},
		{
			description: "path with extra relative dirs works and is cleaned up",
			path:        filepath.Join(dir, "bar/../foo.txt"),
			decoder:     runtimevar.StringDecoder,
			want:        filepath.Join(dir, "foo.txt"),
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			// Create driver impl.
			drv, err := newWatcher(test.path, test.decoder, nil)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
			if drv != nil {
				if drv.path != test.want {
					t.Errorf("got %q want %q", drv.path, test.want)
				}
				drv.Close()
			}

			// Create portable type.
			w, err := OpenVariable(test.path, test.decoder, nil)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
			if w != nil {
				w.Close()
			}
		})
	}
}

func TestOpenVariableURL(t *testing.T) {
	dir, err := ioutil.TempDir("", "gcdk-filevar-example")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	jsonPath := filepath.Join(dir, "myvar.json")
	if err := ioutil.WriteFile(jsonPath, []byte(`{"Foo": "Bar"}`), 0666); err != nil {
		t.Fatal(err)
	}
	txtPath := filepath.Join(dir, "myvar.txt")
	if err := ioutil.WriteFile(txtPath, []byte("hello world!"), 0666); err != nil {
		t.Fatal(err)
	}
	nonexistentPath := filepath.Join(dir, "filenotfound")
	ctx := context.Background()
	secretsPath := filepath.Join(dir, "mysecret.txt")
	cleanup, err := setupTestSecrets(ctx, dir, secretsPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	// Convert paths to a URL path, adding a leading "/" if needed on Windows
	// (on Unix, dirpath already has a leading "/").
	jsonPath = filepath.ToSlash(jsonPath)
	txtPath = filepath.ToSlash(txtPath)
	nonexistentPath = filepath.ToSlash(nonexistentPath)
	secretsPath = filepath.ToSlash(secretsPath)
	if os.PathSeparator != '/' {
		if !strings.HasPrefix(jsonPath, "/") {
			jsonPath = "/" + jsonPath
		}
		if !strings.HasPrefix(txtPath, "/") {
			txtPath = "/" + txtPath
		}
		if !strings.HasPrefix(nonexistentPath, "/") {
			nonexistentPath = "/" + nonexistentPath
		}
		if !strings.HasPrefix(secretsPath, "/") {
			secretsPath = "/" + secretsPath
		}
	}

	tests := []struct {
		URL          string
		WantErr      bool
		WantWatchErr bool
		Want         interface{}
	}{
		// Variable construction succeeds, but the file does not exist.
		{"file://" + nonexistentPath, false, true, nil},
		// Variable construction fails due to invalid decoder arg.
		{"file://" + txtPath + "?decoder=notadecoder", true, false, nil},
		// Variable construction fails due to invalid arg.
		{"file://" + txtPath + "?param=value", true, false, nil},
		// Working example with default decoder.
		{"file://" + txtPath, false, false, []byte("hello world!")},
		// Working example with string decoder.
		{"file://" + txtPath + "?decoder=string", false, false, "hello world!"},
		// Working example with JSON decoder.
		{"file://" + jsonPath + "?decoder=jsonmap", false, false, &map[string]interface{}{"Foo": "Bar"}},
		// Working example with decrypt (default) decoder.
		{"file://" + secretsPath + "?decoder=decrypt", false, false, []byte(`{"Foo":"Bar"}`)},
		// Working example with decrypt+bytes decoder.
		{"file://" + secretsPath + "?decoder=decrypt+bytes", false, false, []byte(`{"Foo":"Bar"}`)},
		// Working example with decrypt+json decoder.
		{"file://" + secretsPath + "?decoder=decrypt+jsonmap", false, false, &map[string]interface{}{"Foo": "Bar"}},
		// Working example with escaped decrypt+json decoder
		{"file://" + secretsPath + "?decoder=" + url.QueryEscape("decrypt+jsonmap"), false, false, &map[string]interface{}{"Foo": "Bar"}},
	}

	for _, test := range tests {
		t.Run(test.URL, func(t *testing.T) {
			v, err := runtimevar.OpenVariable(ctx, test.URL)
			if (err != nil) != test.WantErr {
				t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
			}
			if err != nil {
				return
			}
			defer v.Close()
			snapshot, err := v.Watch(ctx)
			if (err != nil) != test.WantWatchErr {
				t.Errorf("%s: got Watch error %v, want error %v", test.URL, err, test.WantWatchErr)
			}
			if err != nil {
				return
			}
			if !cmp.Equal(snapshot.Value, test.Want) {
				t.Errorf("%s: got snapshot value\n%v\n  want\n%v", test.URL, snapshot.Value, test.Want)
			}
		})
	}
}

func setupTestSecrets(ctx context.Context, dir, secretsPath string) (func(), error) {
	const keeperEnv = "RUNTIMEVAR_KEEPER_URL"
	const keeperURL = "base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4="
	oldURL := os.Getenv(keeperEnv)
	os.Setenv(keeperEnv, keeperURL)
	cleanup := func() { os.Setenv(keeperEnv, oldURL) }

	k, err := secrets.OpenKeeper(ctx, keeperURL)
	if err != nil {
		return cleanup, err
	}
	sc, err := k.Encrypt(ctx, []byte(`{"Foo":"Bar"}`))
	if err != nil {
		return cleanup, err
	}
	if err := ioutil.WriteFile(secretsPath, sc, 0666); err != nil {
		return cleanup, err
	}
	return cleanup, nil
}
