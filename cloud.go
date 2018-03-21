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

package cloud

import (
	"fmt"
	"strings"
)

// Service splits a path of the form "service://resource/identifier"
// into ("service", "resource/identifier"). path must specify at least the
// service name and one path element.
func Service(path string) (service, rest string, err error) {
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 3 {
		return "", "", fmt.Errorf("path must contain at least a service and one path element: %v", parts)
	}
	if parts[0][len(parts[0])-1] != ':' {
		return "", "", fmt.Errorf("missing ':' separating service name from path")
	}
	service = parts[0][0 : len(parts[0])-1]
	if parts[1] != "" {
		return "", "", fmt.Errorf("missing '//' separating service name from path")
	}
	rest = parts[2]
	return
}

// PopPath returns the first path element and the rest of the path. It assumes
// the path is delimited by '/'. PopPath is useful since many identifiers have
// something 'special' as the first element, like a bucket name.
func PopPath(path string) (first, rest string) {
	parts := strings.SplitN(path, "/", 2)
	if len(parts) > 1 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}
