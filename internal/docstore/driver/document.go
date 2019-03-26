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

package driver

import (
	"reflect"

	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/docstore/internal/fields"
	"gocloud.dev/internal/gcerr"
)

var fieldCache = fields.NewCache(nil, nil, nil)

// A Document is a lightweight wrapper around either a map[string]interface{} or a
// struct pointer. It provides operations to get and set fields and field paths.
type Document struct {
	m      map[string]interface{} // nil if it's a *struct
	s      reflect.Value          // the struct reflected
	fields fields.List            // for structs
}

// NewDocument creates a new document from doc, which must be a
// map[string]interface{} or a struct pointer.
func NewDocument(doc interface{}) (Document, error) {
	// TODO: handle a nil *struct?
	if m, ok := doc.(map[string]interface{}); ok {
		return Document{m: m}, nil
	}
	v := reflect.ValueOf(doc)
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		v = v.Elem()
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return Document{}, gcerr.Newf(gcerr.InvalidArgument, nil, "expecting struct, *struct or map[string]interface{}, got %s", t)
	}
	fields, err := fieldCache.Fields(t)
	if err != nil {
		return Document{}, err
	}
	return Document{s: v, fields: fields}, nil
}

// GetField returns the value of the named document field.
func (d Document) GetField(field string) (interface{}, error) {
	if d.m != nil {
		x, ok := d.m[field]
		if !ok {
			return nil, gcerr.Newf(gcerr.NotFound, nil, "field %q not found in map", field)
		}
		return x, nil
	}
	v, err := d.structField(field)
	if err != nil {
		return nil, err
	}
	return v.Interface(), nil
}

// getDocument gets the value of the given field path, which must be a document.
// If create is true, it creates intermediate documents as needed.
func (d Document) getDocument(fp []string, create bool) (Document, error) {
	if len(fp) == 0 {
		return d, nil
	}
	x, err := d.GetField(fp[0])
	if err != nil {
		if create && gcerrors.Code(err) == gcerrors.NotFound {
			// TODO(jba): create the right type for the struct field.
			x = map[string]interface{}{}
			if err := d.SetField(fp[0], x); err != nil {
				return Document{}, err
			}
		} else {
			return Document{}, err
		}
	}
	d2, err := NewDocument(x)
	if err != nil {
		return Document{}, err
	}
	return d2.getDocument(fp[1:], create)
}

// Get returns the value of the given field path in the document.
func (d Document) Get(fp []string) (interface{}, error) {
	d2, err := d.getDocument(fp[:len(fp)-1], false)
	if err != nil {
		return nil, err
	}
	return d2.GetField(fp[len(fp)-1])
}

func (d Document) structField(name string) (reflect.Value, error) {
	f := d.fields.Match(name)
	if f == nil {
		return reflect.Value{}, gcerr.Newf(gcerr.NotFound, nil, "field %q not found in struct type %s", name, d.s.Type())
	}
	fv, ok := fieldByIndex(d.s, f.Index)
	if !ok {
		return reflect.Value{}, gcerr.Newf(gcerr.InvalidArgument, nil, "nil embedded pointer; cannot get field %q from %s",
			name, d.s.Type())
	}
	return fv, nil
}

// Set sets the value of the field path in the document.
// This creates sub-maps as necessary, if possible.
func (d Document) Set(fp []string, val interface{}) error {
	d2, err := d.getDocument(fp[:len(fp)-1], true)
	if err != nil {
		return err
	}
	return d2.SetField(fp[len(fp)-1], val)
}

// SetField sets the field to value in the document.
func (d Document) SetField(field string, value interface{}) error {
	if d.m != nil {
		d.m[field] = value
		return nil
	}
	v, err := d.structField(field)
	if err != nil {
		return err
	}
	if !v.CanSet() {
		return gcerr.Newf(gcerr.InvalidArgument, nil, "cannot set field %s in struct of type %s: not addressable",
			field, d.s.Type())
	}
	v.Set(reflect.ValueOf(value))
	return nil
}

// Encode encodes the document using the given Encoder.
func (d Document) Encode(e Encoder) error {
	if d.m != nil {
		return encodeMap(reflect.ValueOf(d.m), e)
	}
	return encodeStructWithFields(d.s, d.fields, e)
}

// Decode decodes the document using the given Decoder.
func (d Document) Decode(dec Decoder) error {
	if d.m != nil {
		return Decode(reflect.ValueOf(d.m), dec)
	}
	return Decode(d.s, dec)
}
