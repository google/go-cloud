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

	"gocloud.dev/docstore/internal/fields"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
)

// A Document is a lightweight wrapper around either a map[string]interface{} or a
// struct pointer. It provides operations to get and set fields and field paths.
type Document struct {
	Origin interface{}            // the argument to NewDocument
	m      map[string]interface{} // nil if it's a *struct
	s      reflect.Value          // the struct reflected
	fields fields.List            // for structs
}

// NewDocument creates a new document from doc, which must be a non-nil
// map[string]interface{} or struct pointer.
func NewDocument(doc interface{}) (Document, error) {
	if doc == nil {
		return Document{}, gcerr.Newf(gcerr.InvalidArgument, nil, "document cannot be nil")
	}
	if m, ok := doc.(map[string]interface{}); ok {
		if m == nil {
			return Document{}, gcerr.Newf(gcerr.InvalidArgument, nil, "document map cannot be nil")
		}
		return Document{Origin: doc, m: m}, nil
	}
	v := reflect.ValueOf(doc)
	t := v.Type()
	if t.Kind() != reflect.Ptr || t.Elem().Kind() != reflect.Struct {
		return Document{}, gcerr.Newf(gcerr.InvalidArgument, nil, "expecting *struct or map[string]interface{}, got %s", t)
	}
	t = t.Elem()
	if v.IsNil() {
		return Document{}, gcerr.Newf(gcerr.InvalidArgument, nil, "document struct pointer cannot be nil")
	}
	fields, err := fieldCache.Fields(t)
	if err != nil {
		return Document{}, err
	}
	return Document{Origin: doc, s: v.Elem(), fields: fields}, nil
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
	// We do case-insensitive match here to cover the MongoDB's lowercaseFields
	// option.
	f := d.fields.MatchFold(name)
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

// FieldNames returns names of the top-level fields of d.
func (d Document) FieldNames() []string {
	var names []string
	if d.m != nil {
		for k := range d.m {
			names = append(names, k)
		}
	} else {
		for _, f := range d.fields {
			names = append(names, f.Name)
		}
	}
	return names
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
		return decodeMap(reflect.ValueOf(d.m), dec)
	}
	return decodeStruct(d.s, dec)
}

// HasField returns whether or not d has a certain field.
func (d Document) HasField(field string) bool {
	return d.hasField(field, true)
}

// HasFieldFold is like HasField but matches case-insensitively for struct
// field.
func (d Document) HasFieldFold(field string) bool {
	return d.hasField(field, false)
}

func (d Document) hasField(field string, exactMatch bool) bool {
	if d.m != nil {
		_, ok := d.m[field]
		return ok
	}
	if exactMatch {
		return d.fields.MatchExact(field) != nil
	}
	return d.fields.MatchFold(field) != nil
}
