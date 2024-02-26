package managedfields

import (
	"fmt"
	"strings"
)

var _ (error) = (*Conflict)(nil)

type Conflict struct {
	// Fields is a list of fields that are in conflict.
	Fields []FieldsV1
}

func (c *Conflict) Error() string {
	conflicts := make([]string, len(c.Fields))
	for i, f := range c.Fields {
		conflicts[i] = f.Path().String()
	}
	return fmt.Sprintf(
		"conflicting fields: [%s]",
		strings.Join(conflicts, ", "),
	)
}

func MergeManagedFields(
	managedFields []FieldManager,
	reqFM FieldManager,
) (MergeResult, error) {
	mr := MergeResult{}
	// Create slice of all field managers except the one we are merging.
	otherFields := []FieldsV1{}
	for _, mgrs := range managedFields {
		if mgrs.Manager == reqFM.Manager {
			continue
		}
		otherFields = append(otherFields, mgrs.FieldsV1)
	}
	conflicts := Conflict{}
	fieldsConflict(&conflicts, reqFM.FieldsV1, otherFields...)
	if len(conflicts.Fields) > 0 {
		return MergeResult{}, &conflicts
	}
	fieldsIndex := func() (int, bool) {
		for i, mgrs := range managedFields {
			if mgrs.Manager == reqFM.Manager {
				return i, true
			}
		}
		return -1, false
	}
	index, ok := fieldsIndex()
	if ok {
		fieldsDiff(&mr, managedFields[index].FieldsV1, reqFM.FieldsV1)
		// Overwrite the existing field manager.
		managedFields[index] = reqFM
	} else {
		managedFields = append(managedFields, reqFM)
	}
	mr.ManagedFields = managedFields
	return mr, nil
}

type MergeResult struct {
	// ManagedFields is the updated list of field managers after the merge.
	ManagedFields []FieldManager
	// Removed contains a list of fields that were previously owned by the field
	// manager and have been removed in this merge request.
	Removed []FieldsV1
}

func fieldsConflict(
	conflicts *Conflict,
	fd FieldsV1,
	existing ...FieldsV1,
) {
	// Object values.
	for key, value := range fd.Fields {
		subFields := []FieldsV1{}
		for _, fields := range existing {
			// If the key exists (conflicts) with another manager and this field
			// is a leaf (no children), then we have a conflict.
			// Otherwise add it to the sub fields to check the children of this
			// field.
			if subField, ok := fields.Fields[key]; ok {
				if value.IsLeaf() {
					conflicts.Fields = append(conflicts.Fields, value)
					break
				}
				subFields = append(subFields, subField)
			}
		}
		fieldsConflict(conflicts, value, subFields...)
	}
	// Array elements.
	for key, value := range fd.Elements {
		subFields := []FieldsV1{}
		for _, fields := range existing {
			if subField, ok := fields.Elements[key]; ok {
				if value.IsLeaf() {
					conflicts.Fields = append(conflicts.Fields, value)
					break
				}
				subFields = append(subFields, subField)
			}
		}
		fieldsConflict(conflicts, value, subFields...)
	}
}

func fieldsDiff(mr *MergeResult, old, new FieldsV1) {
	for oldKey, oldValue := range old.Fields {
		newValue, ok := new.Fields[oldKey]
		if !ok {
			mr.Removed = append(mr.Removed, oldValue)
			continue
		}
		fieldsDiff(mr, oldValue, newValue)
	}
	for oldKey, oldValue := range old.Elements {
		newValue, ok := new.Elements[oldKey]
		if !ok {
			mr.Removed = append(mr.Removed, oldValue)
			continue
		}
		fieldsDiff(mr, oldValue, newValue)
	}
}

func PurgeRemovedFields(
	obj map[string]interface{},
	removed []FieldsV1,
) error {
	for _, field := range removed {
		if err := blaObject(obj, field.Path()); err != nil {
			return err
		}
	}
	return nil
}

func blaObject(
	obj map[string]interface{},
	path []FieldsV1Step,
) error {
	fmt.Println("blaObject: ", path)
	step := path[0]
	if step.Key.Type != FieldsV1KeyObject {
		return fmt.Errorf(
			"expected array but got object at %s",
			step.String(),
		)
	}

	stepObj, ok := obj[step.Key.Key]
	if !ok {
		return fmt.Errorf(
			"key %q not found at %q",
			step.Key.Key,
			step.String(),
		)
	}
	if len(path) == 1 {
		delete(obj, step.Key.Key)
		return nil
	}
	nextStep := path[1]
	if nextStep.Key.Type == FieldsV1KeyObject {
		v, ok := stepObj.(map[string]interface{})
		if !ok {
			return fmt.Errorf(
				"expected map[string]interface{}, got %T at %q",
				stepObj,
				step.String(),
			)
		}
		if err := blaObject(v, path[1:]); err != nil {
			return err
		}
		return nil
	}

	if _, ok := stepObj.([]interface{}); !ok {
		return fmt.Errorf(
			"expected []interface{}, got %T at %q",
			stepObj,
			step.String(),
		)
	}

	arrayVal, err := blaArray(stepObj.([]interface{}), path[1:])
	if err != nil {
		return err
	}
	obj[step.Key.Key] = arrayVal

	return nil
}

func blaArray(
	obj []interface{},
	path []FieldsV1Step,
) ([]interface{}, error) {
	step := path[0]
	if step.Key.Type == FieldsV1KeyObject {
		return nil, fmt.Errorf(
			"expected array but got object at %q",
			step.String(),
		)
	}
	index := FindIndexArrayByKey(obj, step.Key)
	if index == -1 {
		return nil, fmt.Errorf(
			"key %q not found at %q",
			step.Key.String(),
			step.String(),
		)
	}
	if len(path) == 1 {
		// Remove the element from the array.
		return append(obj[:index], obj[index+1:]...), nil
	}
	// If there is still path left, we need to recurse.
	nextObj, ok := obj[index].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf(
			"expected map[string]interface{}, got %T at %q",
			obj[index],
			step.String(),
		)
	}
	if err := blaObject(nextObj, path[1:]); err != nil {
		return nil, err
	}
	return obj, nil
}

// func MergeObjectsWithFields(
// 	dst map[string]interface{},
// 	src map[string]interface{},
// 	fields FieldsV1,
// ) {
// 	for _, elem := range fields.Elements {
// 	}
// }

func MergeObjects(
	dst map[string]interface{},
	src map[string]interface{},
) {
	mergeObjects(dst, src)
}

func mergeObjects(dst map[string]interface{}, src map[string]interface{}) {
	for key, srcValue := range src {
		if dstValue, ok := dst[key]; ok {
			switch sv := srcValue.(type) {
			case map[string]interface{}:
				mergeObjects(dstValue.(map[string]interface{}), sv)
			case []interface{}:
				dst[key] = mergeArrays(dstValue.([]interface{}), srcValue.([]interface{}))
			default:
				dst[key] = sv
			}
			continue
		}
		dst[key] = srcValue
	}
}

func mergeArrays(
	dst []interface{},
	src []interface{},
) []interface{} {
	for _, srcObj := range src {
		srcObj, ok := srcObj.(map[string]interface{})
		if !ok {
			// If not an array of objects, src overwrites dst.
			return src
		}
		srcIDValue, ok := srcObj["id"]
		if !ok {
			// If src does not have the merge key, src overwrites dst.
			return src
		}
		index := -1
		for i, dstObj := range dst {
			dstObj, ok := dstObj.(map[string]interface{})
			if !ok {
				// If not an array of objects, src overwrites dst.
				// This should not happen, because if dst is an array of objects
				// so should src be.
				return src
			}
			if dstObj["id"] == srcIDValue {
				index = i
				break
			}
		}
		if index == -1 {
			// If dst does not have the merge key, add src element to the
			// result.
			dst = append(dst, srcObj)
			continue
		}
		// If dst does have a matching merge key, merge the objects.
		// Then update dstObj in dst.
		dstObj := dst[index].(map[string]interface{})
		mergeObjects(dstObj, srcObj)
		dst[index] = dstObj
	}
	return dst
}
