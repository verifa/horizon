package managedfields

import (
	"encoding/json"
	"fmt"
)

func ExtractFieldsV1Object(
	src map[string]interface{},
	fields FieldsV1,
) (map[string]interface{}, error) {
	dst := map[string]interface{}{}
	for key, value := range src {
		subFields, ok := fields.Fields[FieldsV1Key{Key: key}]
		if !ok {
			continue
		}
		if subFields.IsLeaf() {
			dst[key] = value
			continue
		}
		switch value.(type) {
		case map[string]interface{}:
			subDst, err := ExtractFieldsV1Object(
				value.(map[string]interface{}),
				subFields,
			)
			if err != nil {
				return nil, err
			}
			dst[key] = subDst
		case []interface{}:
			subDst, err := extractFieldsV1Array(
				value.([]interface{}),
				subFields,
			)
			if err != nil {
				return nil, err
			}
			dst[key] = subDst
		default:
			// TODO: figure out how to get path/steps for field, so we get the
			// whole path in error.
			return nil, fmt.Errorf("non-leaf field %q is not a map or list", key)
		}
	}

	return dst, nil
}

// extractFieldsV1Array takes a src slice of objects and returns a dst slice
// with the elements that are owned according tot he FieldsV1.
// key of the FieldsV1Step matches the object.
// An array (slice) can only be owned on an element level if it is an array of
// objects. Otherwise the field containing the array is considered a leaf and
// owned entirely by the field manager.
func extractFieldsV1Array(
	src []interface{},
	fields FieldsV1,
) ([]interface{}, error) {
	// If fields is a leaf, we own the entire array.
	if fields.IsLeaf() {
		return src, nil
	}
	dst := make([]interface{}, 0)
	for key, elem := range fields.Elements {
		index := FindIndexArrayByKey(src, key)
		if index == -1 {
			continue
		}
		subDst, err := ExtractFieldsV1Object(
			src[index].(map[string]interface{}),
			elem,
		)
		if err != nil {
			return nil, err
		}

		dst = append(dst, subDst)
	}
	return dst, nil
}

// FindIndexArrayByKey takes an array and a FieldsV1Step.
// It iterates over the array, and if the elements are objects, if checks if the
// key of the FieldsV1Step matches the object.
// If it finds the key-object pairing, it returns the index.
// If it doesn't find the key-object pairing, it returns -1.
func FindIndexArrayByKey(obj []interface{}, key FieldsV1Key) int {
	for i, e := range obj {
		if v, ok := e.(map[string]interface{}); ok {
			if value, ok := v[key.Key]; ok && value == key.Value {
				return i
			}
		}
	}
	return -1
}

func mapToObject[T any](m map[string]interface{}) (T, error) {
	var t T
	bDst, err := json.Marshal(m)
	if err != nil {
		return t, fmt.Errorf("marshalling object: %w", err)
	}
	if err := json.Unmarshal(bDst, &t); err != nil {
		return t, fmt.Errorf("unmarshalling object: %w", err)
	}
	return t, nil
}
