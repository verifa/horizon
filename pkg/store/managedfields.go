package store

import (
	"encoding/json"
	"fmt"

	"github.com/verifa/horizon/pkg/hz"
)

func ManagedFieldsV1(data []byte) (hz.FieldsV1, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return hz.FieldsV1{}, fmt.Errorf("decoding request data: %w", err)
	}

	// Remove fields that should not be included in the managed fields.
	delete(raw, "kind")
	if metadata, ok := raw["metadata"].(map[string]interface{}); ok {
		delete(metadata, "name")
		delete(metadata, "account")
		// If there's nothing left in metadata, remove it.
		if len(metadata) == 0 {
			delete(raw, "metadata")
		}
	}
	return ManagedFieldsV1Object(nil, raw), nil
}

func ManagedFieldsV1Object(
	parent *hz.FieldsV1Step,
	raw map[string]interface{},
) hz.FieldsV1 {
	fields := hz.FieldsV1{
		Parent: parent,
		Fields: make(map[hz.FieldsV1Key]hz.FieldsV1),
	}
	for k, value := range raw {
		key := hz.FieldsV1Key{Key: k}
		step := hz.FieldsV1Step{
			Key:   key,
			Field: &fields,
		}
		switch value := value.(type) {
		case map[string]interface{}:
			fields.Fields[key] = ManagedFieldsV1Object(&step, value)
		case []interface{}:
			fields.Fields[key] = managedFieldsV1Array(&step, value)
		case []map[string]interface{}:
			// Convert to []interface{}.
			// When json unmarshalling an array of objects, it is converted to
			// []interface{} and this will not be needed.
			// When creating test objects, however, we need to do this.
			array := make([]interface{}, len(value))
			for i, v := range value {
				array[i] = v
			}
			fields.Fields[key] = managedFieldsV1Array(&step, array)
		default:
			fields.Fields[key] = hz.FieldsV1{
				Parent: &step,
			}
		}
	}

	return fields
}

func managedFieldsV1Array(
	parent *hz.FieldsV1Step,
	raw []interface{},
) hz.FieldsV1 {
	defaultFields := hz.FieldsV1{
		Parent: parent,
	}
	// If hz.the hz.list is empty, we *should* use the schema to know the
	// element
	// type. For now we can say that the manager owns the field entirely, which
	// is bad and wrong.
	if len(raw) == 0 {
		return defaultFields
	}
	fields := hz.FieldsV1{
		Parent:   parent,
		Elements: make(map[hz.FieldsV1Key]hz.FieldsV1),
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
			key := hz.FieldsV1Key{
				Type:  hz.FieldsV1KeyArray,
				Key:   "id",
				Value: idStr,
			}
			step := hz.FieldsV1Step{
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
