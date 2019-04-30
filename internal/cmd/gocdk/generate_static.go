// Copyright 2019 The Go Cloud Authors
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

// +build ignore

// This script is called by "go generate". It converts the files in static/
// into constants in a new .go file.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/shurcooL/vfsgen"
)

// By default vfsgen captures a ModTime for each file in the generated .go
// file; this can lead to spurious diffs. We wrap the http.Filesystem with
// zeroModTimeFS to replace all ModTimes with the zero time.
type zeroModTimeFS struct{ fs http.FileSystem }

func (fs zeroModTimeFS) Open(name string) (http.File, error) {
	f, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return zeroModTimeFile{f}, nil
}

type zeroModTimeFile struct{ http.File }

func (f zeroModTimeFile) Stat() (os.FileInfo, error) {
	fi, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	return zeroModTimeFileInfo{fi}, nil
}

type zeroModTimeFileInfo struct{ os.FileInfo }

func (zeroModTimeFileInfo) ModTime() time.Time {
	return time.Time{}
}

func main() {
	var fs http.FileSystem = zeroModTimeFS{http.Dir("./static")}
	err := vfsgen.Generate(fs, vfsgen.Options{
		Filename:     "static.go", // output file name
		VariableName: "static",    // output variable name
	})
	if err != nil {
		log.Fatalln(err)
	}
}
