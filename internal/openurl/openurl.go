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

// Package openurl provides helpers for URLMux and URLOpeners in portable APIs.
package openurl // import "gocloud.dev/internal/openurl"

import (
	"fmt"
	"net/url"
)

// SchemeMap maps URL schemes to values. The zero value is an empty map, ready for use.
type SchemeMap struct {
	api string
	m   map[string]interface{}
}

// Register registers scheme for value; subsequent calls to FromString or
// FromURL with scheme will return value.
// api is the portable API name (e.g., "blob"); the same value should always
// be passed.
// typ is the portable type (e.g., "Bucket").
// Register panics if scheme has already been registered.
// TODO(rvangent): Remove typ from here and use a single URLOpener per API.
func (m *SchemeMap) Register(api, typ, scheme string, value interface{}) {
	if m.m == nil {
		m.m = map[string]interface{}{}
	}
	if m.api == "" {
		m.api = api
	} else if m.api != api {
		panic(fmt.Errorf("previously registered using api %q (now %q)", m.api, api))
	}
	if _, exists := m.m[scheme]; exists {
		panic(fmt.Errorf("scheme %q already registered for %s.%s", scheme, api, typ))
	}
	m.m[scheme] = value
}

// FromString parses urlstr as an URL and looks up the value for the URL's scheme.
func (m *SchemeMap) FromString(typ, urlstr string) (interface{}, *url.URL, error) {
	u, err := url.Parse(urlstr)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s.%s: %v", m.api, typ, err)
	}
	val, err := m.FromURL(typ, u)
	if err != nil {
		return nil, nil, err
	}
	return val, u, nil
}

// FromURL looks up the value for u's scheme.
func (m *SchemeMap) FromURL(typ string, u *url.URL) (interface{}, error) {
	if u.Scheme == "" {
		return nil, fmt.Errorf("open %s.%s: no scheme in URL %q", m.api, typ, u)
	}
	v, ok := m.m[u.Scheme]
	if !ok {
		return nil, fmt.Errorf("open %s.%s: no provider registered for %q for URL %q", m.api, typ, u.Scheme, u)
	}
	return v, nil
}
