package managedfields

import (
	"encoding/json"
	"fmt"
)

func ManagedFieldsV1(data []byte) (FieldsV1, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return FieldsV1{}, fmt.Errorf("decoding request data: %w", err)
	}

	// Remove fields that should not be included in the managed fields.
	delete(raw, "kind")
	delete(raw, "apiVersion")
	// TODO: remove this once group gets merged with apiVersion.
	delete(raw, "group")
	if metadata, ok := raw["metadata"].(map[string]interface{}); ok {
		delete(metadata, "name")
		delete(metadata, "namespace")
		// If there's nothing left in metadata, remove it.
		if len(metadata) == 0 {
			delete(raw, "metadata")
		}
	}
	return ManagedFieldsV1Object(nil, raw), nil
}

func ManagedFieldsV1Object(
	parent *FieldsV1Step,
	raw map[string]interface{},
) FieldsV1 {
	fields := FieldsV1{
		Parent: parent,
		Fields: make(map[FieldsV1Key]FieldsV1),
	}
	for k, value := range raw {
		key := FieldsV1Key{Key: k}
		step := FieldsV1Step{
			Key:   key,
			Field: &fields,
		}
		switch value := value.(type) {
		case map[string]interface{}:
			fields.Fields[key] = ManagedFieldsV1Object(&step, value)
		case []interface{}:
			fields.Fields[key] = managedFieldsV1Array(&step, value)
		default:
			fields.Fields[key] = FieldsV1{
				Parent: &step,
			}
		}
	}

	return fields
}

func managedFieldsV1Array(
	parent *FieldsV1Step,
	raw []interface{},
) FieldsV1 {
	defaultFields := FieldsV1{
		Parent: parent,
	}
	// If the list is empty, we *should* use the schema to know the element
	// type. For now we can say that the manager owns the field entirely, which
	// is bad and wrong.
	if len(raw) == 0 {
		return defaultFields
	}
	fields := FieldsV1{
		Parent:   parent,
		Elements: make(map[FieldsV1Key]FieldsV1),
	}
	for _, elem := range raw {
		switch elem := elem.(type) {
		case map[string]interface{}:
			// HACK: for now we hardcode that an object within an array must
			// have an
			// id field. This is the merge key.
			idv, ok := elem["id"]
			if !ok {
				// If the merge key does not exist, we must say that this
				// manager
				// owns this field. This is bad and wrong.
				return defaultFields
			}
			idStr, ok := idv.(string)
			if !ok {
				// If the merge key is not a string, we must say that this
				// manager
				// owns this field. This is bad and wrong.
				return defaultFields
			}
			key := FieldsV1Key{
				Type:  FieldsV1KeyArray,
				Key:   "id",
				Value: idStr,
			}
			step := FieldsV1Step{
				Key:   key,
				Field: &fields,
			}
			fields.Elements[key] = ManagedFieldsV1Object(&step, elem)
		default:
			// For any type other than object, this manager owns the entire
			// field.
			return defaultFields
		}
	}
	return fields
}
