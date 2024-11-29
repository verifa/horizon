package openapiv3

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type Spec struct {
	Openapi    string     `json:"openapi"`
	Info       Info       `json:"info"` // Required.
	Components Components `json:"components,omitempty"`
}

type Info struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type Components struct {
	Schemas map[string]Schema `json:"schemas,omitempty"`
}

type Schema struct {
	Key        string      `json:"-"`
	Type       *SchemaType `json:"type,omitempty"`
	Required   []string    `json:"required,omitempty"`
	Properties Properties  `json:"properties,omitempty"`

	MultipleOf       *float64     `json:"multipleOf,omitempty"`
	Maximum          *float64     `json:"maximum,omitempty"`
	ExclusiveMaximum *bool        `json:"exclusiveMaximum,omitempty"`
	Minimum          *float64     `json:"minimum,omitempty"`
	ExclusiveMinimum *bool        `json:"exclusiveMinimum,omitempty"`
	MaxLength        *int64       `json:"maxLength,omitempty"`
	MinLength        *int64       `json:"minLength,omitempty"`
	Pattern          *string      `json:"pattern,omitempty"`
	MaxItems         *int64       `json:"maxItems,omitempty"`
	MinItems         *int64       `json:"minItems,omitempty"`
	UniqueItems      *bool        `json:"uniqueItems,omitempty"`
	MaxProperties    *int64       `json:"maxProperties,omitempty"`
	MinProperties    *int64       `json:"minProperties,omitempty"`
	Enum             []string     `json:"enum,omitempty"`
	Not              *Schema      `json:"not,omitempty"`
	AllOf            []Schema     `json:"allOf,omitempty"`
	OneOf            []Schema     `json:"oneOf,omitempty"`
	AnyOf            []Schema     `json:"anyOf,omitempty"`
	Items            *Schema      `json:"items,omitempty"`
	Description      *string      `json:"description,omitempty"`
	Format           *string      `json:"format,omitempty"`
	Default          *interface{} `json:"default,omitempty"`
	Nullable         *bool        `json:"nullable,omitempty"`
	ReadOnly         *bool        `json:"readOnly,omitempty"`
	WriteOnly        *bool        `json:"writeOnly,omitempty"`
	Example          *interface{} `json:"example,omitempty"`
	Deprecated       *bool        `json:"deprecated,omitempty"`
}

func (s Schema) Property(key string) (Schema, bool) {
	for _, p := range s.Properties {
		if p.Key == key {
			return p, true
		}
	}
	return Schema{}, false
}

type SchemaType string

// SchemaType values enumeration.
const (
	SchemaTypeArray   = SchemaType("array")
	SchemaTypeBoolean = SchemaType("boolean")
	SchemaTypeInteger = SchemaType("integer")
	SchemaTypeNumber  = SchemaType("number")
	SchemaTypeObject  = SchemaType("object")
	SchemaTypeString  = SchemaType("string")
)

type Properties []Schema

func (p Properties) Get(key string) (Schema, bool) {
	for _, s := range p {
		if s.Key == key {
			return s, true
		}
	}
	return Schema{}, false
}

func (p Properties) MarshalJSON() ([]byte, error) {
	buf := bytes.Buffer{}

	buf.WriteString("{")
	for i, kv := range p {
		if i != 0 {
			buf.WriteString(",")
		}
		key, err := json.Marshal(kv.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteString(":")
		val, err := json.Marshal(kv)
		if err != nil {
			return nil, err
		}
		buf.Write(val)
	}

	buf.WriteString("}")
	return buf.Bytes(), nil
}

func (p *Properties) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewBuffer(b))
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if t != json.Delim('{') {
		return fmt.Errorf("expected '{', got %T: %v", t, t)
	}
	*p = make(Properties, 0)
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return fmt.Errorf("key: %w", err)
		}
		var v Schema
		if err := dec.Decode(&v); err != nil {
			return fmt.Errorf("decode: %w", err)
		}
		var ok bool
		v.Key, ok = k.(string)
		if !ok {
			return fmt.Errorf("key %q is not a string", k)
		}
		*p = append(*p, v)
	}
	return nil
}
