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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"text/template"

	"golang.org/x/xerrors"
)

// List returns a listing of files and directories from the static data under
// dir. If recurse is true, it recurses into directories.
func List(dir string, recurse bool) ([]os.FileInfo, error) {
	f, err := assets.Open(dir)
	if err != nil {
		return nil, err
	}
	infos, err := f.Readdir(-1)
	if err != nil {
		return nil, err
	}
	var retval []os.FileInfo
	for _, info := range infos {
		retval = append(retval, info)
		if recurse && info.IsDir() {
			r, err := List(path.Join(dir, info.Name()), true)
			if err != nil {
				return nil, err
			}
			retval = append(retval, r...)
		}
	}
	return retval, nil
}

// MaterializeOptions holds options for Materialize.
type MaterializeOptions struct {
	// Force allows Materialize to clobber existing files when copying.
	Force bool
	// Logger is used to log information about changes.
	Logger *log.Logger
	// TemplateData is passed when executing templates marked with
	// fileOptions.IsTemplate.
	TemplateData interface{}
}

// TODO(rvangent): There are several cases where multiple places under
// static/assets_ are repeating the same data:
// 1. For places that we ask the user questions; currently, we are hardcoding
//    their "answer" (e.g., in provision/blob/gcsblob/mainlocal_gcpproject.tf).
//    But each place that wants to ask the question would need to repeat the
//    .tf file and the .json config for it. Instead, we should have something
//    like a "prompt" package that handles asking questions and (if needed)
//    saving them. Each "provision" provider should just specify which
//    questions it needs answered.
// 2. Similarly, there are shared resources like the "random" provider that are
//    currently repeated in multiple places, which the .json config file using
//    AddToSkipMarker to ensure we don't add it more than once. It would be more
//    robust to have this specified once centrally, and referenced.

// fileOptions holds options for materializing a specific static file.
type fileOptions struct {
	// Path is the relative path to the file being configured, from srcRoot.
	Path string
	// IsTemplate is true means that the source file will be parsed and executed
	// as a Go template (see https://golang.org/pkg/text/template/). The template
	// will be passed templateData. The result of executing the template will be
	// used as the content instead of the raw source.
	// NOTE: We can't just treat every file as a template; some content files
	// include template directives that should not be interpolated at copy time.
	// For example, the .go files for demos include constants for the HTML they
	// generate which include template directives. We chose to default to treating
	// source files as raw bytes rather than as template because most files are
	// copied as is, and it would be surprising if including a template directive
	// accidentally (e.g., a stray "{{") caused things to break.
	// TODO(rvangent): Consider specifying a list of template keys that the
	// template requires, and enforcing that they are provided in TemplateData.
	// TemplateData would probably have to always be a map to make that possible.
	// Alternatively, consider at least verifying that if TemplateData was
	// provided, it was used, and if IsTemplate=true, that TemplateData is
	// not nil.
	IsTemplate bool

	// AddTo sets a destination file that the content from this file will be
	// added to. The destination file must already exist. See AddToAfterMarker
	// for where the content will be added, and AddToSkipMarker to identify if
	// the content has already been added.
	AddTo string

	// AddToSkipMarker is used to determine whether the addition to the
	// destination file configured by AddTo has already occurred, and thus this
	// source file should be skipped. It defaults to the content.
	AddToSkipMarker string

	// AddToMarker determines where in the destinatation file configured by AddTo
	// the content is placed. If AddToMarker is the empty string, the content is
	// appended. Otherwise, it is inserted into the file at the next line after
	// the marker. If the marker is not found, an error is returned.
	AddToAfterMarker string

	// AddToDescription describes the content that was added; used when logging
	// changes via MaterializeOptions.Logger. Log messages are of the form
	// "added <AddToDescription> to <filename>". Defaults to "content".
	AddToDescription string
}

// Materialize recursively materializes files from the static data at srcRoot to
// dstRoot.
//
// By default, it copies files. If a file already exists at the destination,
// Materialize will return an error unless force is true.
//
// Additional behavior can be configured by adding a JSON file to the static
// data, at srcRoot + ".json". The JSON file should consist of a sequence of
// JSON objects that will be deserialized into fileOptions structs; see
// the struct docstring for more info.
func Materialize(dstRoot, srcRoot string, opts *MaterializeOptions) error {
	if opts == nil {
		opts = new(MaterializeOptions)
	}
	allFileOpts := map[string]fileOptions{}

	// Read the config file. If it is not there, that's fine, we'll just treat
	// all files using the zero value fileOptions.
	jsonCfgFilePath := srcRoot + ".json"
	optsFile, err := assets.Open(jsonCfgFilePath)
	if err == nil {
		dec := json.NewDecoder(optsFile)
		for {
			var cfg fileOptions
			if err := dec.Decode(&cfg); err == io.EOF {
				break
			} else if err != nil {
				return xerrors.Errorf("failed to read JSON from static data configuration file %s: %w", jsonCfgFilePath, err)
			}
			allFileOpts[cfg.Path] = cfg
		}
	}
	return materialize(dstRoot, srcRoot, "", opts, allFileOpts)
}

// materialize is a helper for Materialize, operating on a subdirectory.
// dstRoot and srcRoot are the root directories that Materialize was called
// with; subdir holds the current subdirectory path under that that we've
// recursed to.
func materialize(dstRoot, srcRoot, subdir string, opts *MaterializeOptions, allFileOpts map[string]fileOptions) error {
	srcDirPath := path.Join(srcRoot, subdir)
	dir, err := assets.Open(srcDirPath)
	if err != nil {
		return xerrors.Errorf("failed to open static data directory at %s: %w", srcDirPath, err)
	}
	defer dir.Close()
	infos, err := dir.Readdir(-1)
	if err != nil {
		return xerrors.Errorf("failed to read static data directory at %s: %w", srcDirPath, err)
	}
	dstDirPath := path.Join(dstRoot, subdir)
	if err := os.MkdirAll(dstDirPath, 0777); err != nil {
		return xerrors.Errorf("failed to create output directory at %s: %w", dstDirPath, err)
	}
	for _, info := range infos {
		relPath := path.Join(subdir, info.Name())
		if info.IsDir() {
			if err := materialize(dstRoot, srcRoot, relPath, opts, allFileOpts); err != nil {
				return err
			}
			continue
		}
		srcPath := path.Join(srcRoot, relPath)
		f, err := assets.Open(srcPath)
		if err != nil {
			return xerrors.Errorf("failed to open static data file at %s: %w", srcPath, err)
		}
		defer f.Close()
		srcBytes, err := ioutil.ReadAll(f)
		if err != nil {
			return xerrors.Errorf("failed to read static data file at %s: %w", srcPath, err)
		}
		dstBytes := srcBytes
		fileOpts := allFileOpts[relPath]
		if fileOpts.IsTemplate {
			tmpl, err := template.New(relPath).Parse(string(srcBytes))
			if err != nil {
				return xerrors.Errorf("failed to parse static data file at %s as a template: %w", srcPath, err)
			}
			buf := new(bytes.Buffer)
			if err := tmpl.Execute(buf, opts.TemplateData); err != nil {
				return xerrors.Errorf("failed to execute static data template file %s: %w", srcPath, err)
			}
			dstBytes = buf.Bytes()
		}

		if fileOpts.AddTo == "" {
			// By default, write a new file.
			dstPath := filepath.Join(dstRoot, relPath)
			if !opts.Force {
				if _, err := os.Stat(dstPath); err == nil {
					// TODO(rvangent): Should we make --force always available?
					//return fmt.Errorf("%q has already been added to your project. Use --force (if available) if you want to re-add it, overwriting previous file(s)", dstPath)
					return fmt.Errorf("%q has already been added to your project", dstPath)
				}
			}
			if err := ioutil.WriteFile(dstPath, dstBytes, 0666); err != nil {
				return xerrors.Errorf("failed to write output file at %s: %w", dstPath, err)
				return err
			}
			if opts.Logger != nil {
				opts.Logger.Printf("  added a new file %q to your project", relPath)
			}
		} else {
			// Insert/append to an existing file.
			modified, err := addToFile(filepath.Join(dstRoot, fileOpts.AddTo), fileOpts.AddToAfterMarker, fileOpts.AddToSkipMarker, srcBytes)
			if err != nil {
				return err
			}
			if modified && opts.Logger != nil {
				description := fileOpts.AddToDescription
				if description == "" {
					description = "content"
				}
				opts.Logger.Printf("  added %s to %q", description, fileOpts.AddTo)
			}
		}
	}
	return nil
}

// addToFile adds content into the file at path.
//
// addToFile no-ops if skipMarker (or content if skipMarker is the empty string)
// is found in the existing content at path.
//
// If marker is the empty string, addToFile appends content to the file at path;
// otherwise, it inserts content at the next line after the marker.
//
// It returns true if changes were made.
func addToFile(path, marker, skipMarker string, content []byte) (bool, error) {
	existingContent, err := ioutil.ReadFile(path)
	if err != nil {
		return false, xerrors.Errorf("couldn't read existing file to modify at %s: %w", path, err)
	}

	// See if we should skip doing anything.
	skipMarkerBytes := content
	if skipMarker != "" {
		skipMarkerBytes = []byte(skipMarker)
	}
	if idx := bytes.Index(existingContent, skipMarkerBytes); idx != -1 {
		return false, nil
	}

	// Figure out where to put the content.
	var insertIdx int
	if marker == "" {
		// Append.
		insertIdx = len(existingContent)
	} else {
		// Insert after marker.
		markerBytes := []byte(marker)
		insertIdx = bytes.Index(existingContent, markerBytes)
		if insertIdx == -1 {
			return false, fmt.Errorf("couldn't find marker %q in existing file %s", marker, path)
		}
		// Move past the marker itself, and to the next line.
		insertIdx += len(markerBytes)
		for existingContent[insertIdx] == '\n' || existingContent[insertIdx] == '\r' {
			insertIdx++
		}
	}
	newContent := append(existingContent[:insertIdx], append(content, existingContent[insertIdx:]...)...)

	// TODO(rvangent): Copy os.FileMode from the current file?
	if err := ioutil.WriteFile(path, newContent, 0666); err != nil {
		return false, xerrors.Errorf("couldn't update %s: %w", path, err)
	}
	return true, nil
}
