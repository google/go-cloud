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

import "net/http"

// Open opens a file from the static assets.
// TODO(rvangent): Provide a richer (but hopefully smaller) API,
// which should cover things like copying files from assets/,
// materializeTemplateDir, etc.
func Open(path string) (http.File, error) {
	return assets.Open(path)
}
