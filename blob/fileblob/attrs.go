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

package fileblob

import (
	"encoding/json"
	"os"
)

const attrsExt = ".attrs"

type xattrs struct {
	MIMEType string `json:"user.mime_type"`
	Charset  string `json:"user.charset"`
}

func (w writer) setAttrs() error {
	f, err := os.Create(w.path + attrsExt)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(w.attrs); err != nil {
		return err
	}
	return f.Close()
}

func getAttrs(path string) (*xattrs, error) {
	f, err := os.Open(path + attrsExt)
	if err != nil {
		return nil, err
	}
	xa := new(xattrs)
	if err := json.NewDecoder(f).Decode(xa); err != nil {
		return nil, err
	}
	return xa, f.Close()
}
