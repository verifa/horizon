package managedfields

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type ManagedFields []FieldManager

func (m ManagedFields) FieldManager(manager string) (FieldManager, bool) {
	for _, fm := range m {
		if fm.Manager == manager {
			return fm, true
		}
	}
	return FieldManager{}, false
}

type FieldManager struct {
	Manager string `json:"manager" cue:"=~\"^[a-zA-Z0-9-_]+$\""`
	// Operation  Operation `json:"operation" cue:"=~\"^[a-zA-Z0-9-_]+$\""`
	// Time       time.Time `json:"time" cue:",opt"`
	FieldsType string   `json:"fieldsType" cue:"=~\"^[a-zA-Z0-9-_]+$\""`
	FieldsV1   FieldsV1 `json:"fieldsV1"`
}

type FieldsV1 struct {
	Parent *FieldsV1Step `json:"-"`
	// Fields = Object
	Fields map[FieldsV1Key]FieldsV1 `json:"-"`
	// Elements = Array
	Elements map[FieldsV1Key]FieldsV1 `json:"-"`
}

func (f FieldsV1) IsLeaf() bool {
	return len(f.Fields) == 0 && len(f.Elements) == 0
}

func (f FieldsV1) Path() FieldsV1Path {
	if f.Parent == nil {
		return FieldsV1Path{}
	}
	return append(f.Parent.Field.Path(), *f.Parent)
}

type FieldsV1Path []FieldsV1Step

func (p FieldsV1Path) String() string {
	steps := []string{}
	for _, step := range p {
		steps = append(steps, step.Key.Key)
	}
	return strings.Join(steps, ".")
}

type FieldsV1Step struct {
	Key   FieldsV1Key `json:"-"`
	Field *FieldsV1   `json:"-"`
}

func (s FieldsV1Step) String() string {
	if s.Field.Parent == nil {
		return s.Key.String()
	}
	steps := []string{s.Key.String()}
	for step := s.Field.Parent; step != nil; step = step.Field.Parent {
		steps = append([]string{step.Key.String()}, steps...)
	}
	return strings.Join(steps, ".")
}

type FieldsV1Key struct {
	Type  FieldsV1KeyType `json:"-"`
	Key   string          `json:"-"`
	Value string          `json:"-"`
}

type FieldsV1KeyType int

const (
	FieldsV1KeyObject FieldsV1KeyType = iota
	FieldsV1KeyArray
)

func (k FieldsV1Key) String() string {
	if k.Type == FieldsV1KeyObject {
		return k.Key
	}
	return fmt.Sprintf("{%s:%s}", k.Key, k.Value)
}

func (f FieldsV1) MarshalJSON() ([]byte, error) {
	if len(f.Fields) > 0 && len(f.Elements) > 0 {
		return nil, errors.New("cannot have both object and array")
	}
	if len(f.Fields) > 0 {
		buf := bytes.Buffer{}
		buf.WriteString("{")
		index := 0
		for key, subField := range f.Fields {
			bSub, err := json.Marshal(subField)
			if err != nil {
				return nil, err
			}
			if index > 0 {
				buf.WriteString(",")
			}
			buf.WriteString(fmt.Sprintf("\"f:%s\":", key.Key))
			buf.Write(bSub)
			index++
		}
		buf.WriteString("}")
		return buf.Bytes(), nil
	}
	if len(f.Elements) > 0 {
		buf := bytes.Buffer{}
		buf.WriteString("{")
		index := 0
		for key, subField := range f.Elements {
			bSub, err := json.Marshal(subField)
			if err != nil {
				return nil, err
			}
			if index > 0 {
				buf.WriteString(",")
			}
			// Could json unmarshal key otherwise, but this seems easier
			// somehow.
			strKey := strconv.Quote(
				fmt.Sprintf("k:{\"%s\":\"%s\"}", key.Key, key.Value),
			)
			buf.WriteString(fmt.Sprintf("%s:%s", strKey, bSub))
			index++
		}
		buf.WriteString("}")

		return buf.Bytes(), nil
	}
	return []byte("{}"), nil
}

func (f *FieldsV1) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	for key, value := range raw {
		switch key[0:2] {
		case "f:":
			if len(f.Fields) == 0 {
				f.Fields = make(map[FieldsV1Key]FieldsV1, len(raw))
			}
			var subField FieldsV1
			if err := json.Unmarshal(value, &subField); err != nil {
				return err
			}
			subKey := FieldsV1Key{Key: key[2:]}

			subField.Parent = &FieldsV1Step{
				Key:   subKey,
				Field: f,
			}
			f.Fields[subKey] = subField
		case "k:":
			if len(f.Elements) == 0 {
				f.Elements = make(map[FieldsV1Key]FieldsV1, len(raw))
			}
			var subKey FieldsV1Key
			if err := json.Unmarshal([]byte(key[2:]), &subKey); err != nil {
				return err
			}
			var subField FieldsV1
			if err := json.Unmarshal(value, &subField); err != nil {
				return err
			}
			subField.Parent = &FieldsV1Step{
				Key:   subKey,
				Field: f,
			}
			f.Elements[subKey] = subField
		default:
			return fmt.Errorf("invalid key: %s", key)
		}
	}
	return nil
}

func (f FieldsV1Key) MarshalJSON() ([]byte, error) {
	if f.Type == FieldsV1KeyObject {
		return nil, errors.New(
			"cannot marshal key of type object (must be array)",
		)
	}
	return json.Marshal(map[string]string{
		f.Key: f.Value,
	})
}

func (f *FieldsV1Key) UnmarshalJSON(data []byte) error {
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key, value := range raw {
		f.Type = FieldsV1KeyArray
		f.Key = key
		f.Value = value
		return nil
	}
	return errors.New("empty fields key \"k:\"")
}

type Operation string

const (
	OperationCreate Operation = "Create"
	OperationUpdate Operation = "Update"
	OperationApply  Operation = "Apply"
)
