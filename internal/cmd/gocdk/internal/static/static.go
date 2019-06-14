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

package static

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"text/template"

	"golang.org/x/xerrors"
)

// TODO(rvangent): Consider panicking instead of returning an error in more
// cases where the static data itself returns an error. E.g., ListFiles should
// never fail.

// ListFiles returns a listing of files from the static data under dir,
// recursively. The relative path to each file from dir is returned.
func ListFiles(dir string) ([]string, error) {
	f, err := assets.Open(dir)
	if err != nil {
		return nil, err
	}
	infos, err := f.Readdir(-1)
	if err != nil {
		return nil, err
	}
	var retval []string
	for _, info := range infos {
		fullpath := path.Join(dir, info.Name())
		if info.IsDir() {
			nestedFiles, err := ListFiles(fullpath)
			if err != nil {
				return nil, err
			}
			for _, f := range nestedFiles {
				retval = append(retval, path.Join(info.Name(), f))
			}
			continue
		}
		retval = append(retval, info.Name())
	}
	return retval, nil
}

// Action represents an action relative to static content that will be performed
// in Do.
// TODO(rvangent): Consider unexporting most of the fields; only TemplateData
// is currently used externally, and the helpers in this package are used to
// construct Actions.
type Action struct {
	// SourceContent specifies the source content to be instantiated.
	// Exactly one of SourceContent/SourcePath should be specified.
	SourceContent []byte

	// SourcePath is a full path to a file in the static data to be used as
	// the source content.
	// Exactly one of SourceContent/SourcePath should be specified.
	SourcePath string

	// DestRelPath is the destination path, relative to the destRoot
	// provided to Do.
	DestRelPath string

	// TemplateData set to a non-nil value causes the source content to
	// be parsed and executed as a Go template
	// (see https://golang.org/pkg/text/template/). TemplateData will be
	// passed to Execute. The result of executing the template will then
	// be used as the source content.
	// NOTE: We can't just treat every file as a template; some content
	// files include template directives that should not be interpolated at
	// copy time. For example, the .go files for demos include constants
	// for the HTML they generate which include template directives. We
	// chose to default to treating source files as raw bytes rather than
	// as template because most files are copied as is, and it would be
	// surprising if including a template directive accidentally (e.g.,
	// a stray "{{") caused things to break.
	TemplateData interface{}

	// DestExists must be set to true if the content is being added to an
	// existing file. If the destination file does not exist, Do will return
	// an error.
	// See InsertMarker for where the content will be added, and
	// SkipMarker to identify if the content has already been added.
	DestExists bool

	// SkipMarker is used when DestExists is true, to determine whether
	// the addition has already occurred, and therefore this action should
	// be skipped. It defaults to the source content.
	SkipMarker string

	// InsertMarker is used when DestExists is true, to determine where in
	// the destinatation file the source content is inserted. If empty, the
	// source content is appended to the end of the file. Otherwise, it is
	// inserted into the file at the next line after the marker. If the
	// marker is not found, an error is returned.
	InsertMarker string

	// Description is used when DestExists is true, to describe the content
	// that was added in log messages via Options.Logger.
	// Log messages are of the form "added <Description> to <filename>".
	Description string
}

// AddProvider returns an Action that will add a Terraform provider to main.tf.
//
// It pulls content from the static data under /providers/<provider>.tf and
// appends to main.tf.
func AddProvider(provider string) *Action {
	return &Action{
		SourcePath:  fmt.Sprintf("/providers/%s.tf", provider),
		DestRelPath: "main.tf",
		DestExists:  true,
		SkipMarker:  fmt.Sprintf("provider %q", provider),
		Description: fmt.Sprintf("a Terraform provider %q", provider),
	}
}

// AddLocal returns an Action that will add a Terraform variable to the "locals"
// section in main.tf.
func AddLocal(key, val string) *Action {
	return &Action{
		SourceContent: []byte(fmt.Sprintf("  %s = %q\n", key, val)),
		DestRelPath:   "main.tf",
		DestExists:    true,
		InsertMarker:  "# DO NOT REMOVE THIS COMMENT; GO CDK LOCALS WILL BE INSERTED BELOW HERE",
		Description:   fmt.Sprintf("a local variable %q", key),
	}
}

// AddOutputVar returns an Action that will add a Terraform output variable to
// outputs.tf.
func AddOutputVar(key, val string) *Action {
	return &Action{
		SourceContent: []byte(fmt.Sprintf("    %s = %q\n", key, val)),
		DestRelPath:   "outputs.tf",
		DestExists:    true,
		InsertMarker:  "# DO NOT REMOVE THIS COMMENT; GO CDK DEMO URLs WILL BE INSERTED BELOW HERE",
		Description:   fmt.Sprintf("an output variable %q", key),
	}
}

// CopyFile returns an Action that copies a single file from the static data
// at srcPath to destRelPath. destRelPath is relative to the destRoot passed to
// Do.
func CopyFile(srcPath, destRelPath string) *Action {
	return &Action{
		SourcePath:  srcPath,
		DestRelPath: destRelPath,
	}
}

// CopyDir returns a slice of Action that recursively copy all of the files
// under srcRoot in the static data to the same paths relative to the destRoot
// passed to Do.
func CopyDir(srcRoot string) ([]*Action, error) {
	files, err := ListFiles(srcRoot)
	if err != nil {
		return nil, err
	}
	var retval []*Action
	for _, f := range files {
		retval = append(retval, &Action{
			SourcePath:  path.Join(srcRoot, f),
			DestRelPath: f,
		})
	}
	return retval, nil
}

// Printer supports printing messages.
type Printer interface {
	Printf(format string, args ...interface{})
}

// Options holds options for Do.
type Options struct {
	// Force allows Do to clobber existing files when copying.
	Force bool
	// Logger is used to log information about changes.
	Logger Printer
}

// Do recursively materializes files from the static data at srcRoot to
// destRoot.
//
// By default, it copies files. If a file already exists at the destination,
// Do will return an error unless force is true.
//
// Additional behavior can be configured by adding a JSON file to the static
// data, at srcRoot + ".json". The JSON file should consist of a sequence of
// JSON objects that will be deserialized into fileOptions structs; see
// the struct docstring for more info.
func Do(destRoot string, opts *Options, actions ...*Action) error {
	if opts == nil {
		opts = new(Options)
	}
	for _, action := range actions {
		if (action.SourceContent == nil) == (action.SourcePath == "") {
			return xerrors.Errorf("exactly one of SourceContent, SourcePath must be specified")
		}
		// First, get the source content into srcBytes.
		var srcBytes []byte
		var srcDescription string
		if action.SourceContent == nil {
			// Load source content from a static file.
			srcDescription = "file " + action.SourcePath
			f, err := assets.Open(action.SourcePath)
			if err != nil {
				return xerrors.Errorf("failed to open static data %s: %w", srcDescription, err)
			}
			defer f.Close()
			srcBytes, err = ioutil.ReadAll(f)
			if err != nil {
				return xerrors.Errorf("failed to read static data %s: %w", srcDescription, err)
			}
		} else {
			srcBytes = action.SourceContent
			srcDescription = string(srcBytes)
		}

		// Treat the source as a template if needed.
		if action.TemplateData != nil {
			tmpl, err := template.New("").Parse(string(srcBytes))
			if err != nil {
				return xerrors.Errorf("failed to parse static data %s as a template: %w", srcDescription, err)
			}
			buf := new(bytes.Buffer)
			if err := tmpl.Execute(buf, action.TemplateData); err != nil {
				return xerrors.Errorf("failed to execute template from %s: %w", srcDescription, err)
			}
			srcBytes = buf.Bytes()
		}

		// Materialize the content.
		destFullPath := filepath.FromSlash(filepath.Join(destRoot, action.DestRelPath))
		if action.DestExists {
			// Insert/append to an existing file.
			modified, err := addToFile(destFullPath, action.InsertMarker, action.SkipMarker, srcBytes)
			if err != nil {
				return err
			}
			if modified && opts.Logger != nil {
				if action.Description == "" {
					opts.Logger.Printf("  added to %q", action.DestRelPath)
				} else {
					opts.Logger.Printf("  added %s to %q", action.Description, action.DestRelPath)
				}
			}
		} else {
			// Write a new file.
			destDir := filepath.Dir(destFullPath)
			if err := os.MkdirAll(destDir, 0777); err != nil {
				return xerrors.Errorf("failed to create output directory at %s: %w", destDir, err)
			}
			if !opts.Force {
				if _, err := os.Stat(destFullPath); err == nil {
					// TODO(rvangent): Should we make --force always available?
					//return fmt.Errorf("%q has already been added to your project. Use --force (if available) if you want to re-add it, overwriting previous file(s)", action.DestPath)
					return fmt.Errorf("%q has already been added to your project", action.DestRelPath)
				}
			}
			if err := ioutil.WriteFile(destFullPath, srcBytes, 0666); err != nil {
				return xerrors.Errorf("failed to write output file at %s: %w", action.DestRelPath, err)
			}
			if opts.Logger != nil {
				opts.Logger.Printf("  added a new file %q", action.DestRelPath)
			}
		}
	}
	return nil
}

// addToFile adds content into the file at path.
//
// addToFile no-ops if skipMarker is found in the existing content of
// the file at path. skipMarker defaults to content.
//
// If marker is the empty string, addToFile appends the content;
// otherwise, it inserts it on the next line after the marker.
//
// It returns true if changes were made.
func addToFile(path, marker, skipMarker string, content []byte) (bool, error) {
	existing, err := ioutil.ReadFile(path)
	if err != nil {
		return false, xerrors.Errorf("couldn't read existing file %q to modify: %w", path, err)
	}

	// See if we should skip.
	skipMarkerBytes := content
	if skipMarker != "" {
		skipMarkerBytes = []byte(skipMarker)
	}
	if idx := bytes.Index(existing, skipMarkerBytes); idx != -1 {
		return false, nil
	}

	// Figure out where to put the content.
	var idx int
	if marker == "" {
		// Append.
		idx = len(existing)
	} else {
		// Insert after the marker.
		markerBytes := []byte(marker)
		idx = bytes.Index(existing, markerBytes)
		if idx == -1 {
			return false, fmt.Errorf("couldn't find insertion marker %q in existing file %q", marker, path)
		}
		// Move past the marker itself, and to the next line.
		idx += len(markerBytes)
		for existing[idx] == '\n' || existing[idx] == '\r' {
			idx++
		}
	}
	newContent := append(existing[:idx], append(content, existing[idx:]...)...)

	// Note: ioutil.WriteFile ignores the permissions for an existing file.
	if err := ioutil.WriteFile(path, newContent, 0666); err != nil {
		return false, xerrors.Errorf("couldn't modify %q: %w", path, err)
	}
	return true, nil
}
