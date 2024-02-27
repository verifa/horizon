package managedfields

import (
	"fmt"
	"slices"
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
	force bool,
) (MergeResult, error) {
	mr := MergeResult{}
	conflicts := Conflict{}
	for i, mgrs := range managedFields {
		// Don't compare the same manager (if it exists).
		// That's just asking for trouble (and conflicts).
		if mgrs.Manager == reqFM.Manager {
			continue
		}
		newFields := conflictOrForceOverrideFields(
			&conflicts,
			mgrs.FieldsV1,
			reqFM.FieldsV1,
			force,
		)
		mgrs.FieldsV1 = newFields
		managedFields[i] = mgrs
	}
	// If any managers are now "empty" (i.e. they own no fields or elements),
	// remove them.
	managedFields = slices.DeleteFunc(
		managedFields,
		func(mgr FieldManager) bool {
			return mgr.FieldsV1.IsLeaf()
		},
	)
	if len(conflicts.Fields) > 0 {
		return MergeResult{}, &conflicts
	}
	// fieldsIndex returns the index of the field manager in the managedFields.
	fieldsIndex := func(mgFields []FieldManager, req FieldManager) int {
		for i, mgrs := range mgFields {
			if mgrs.Manager == req.Manager {
				return i
			}
		}
		return -1
	}
	index := fieldsIndex(managedFields, reqFM)
	if index >= 0 {
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

func conflictOrForceOverrideFields(
	conflicts *Conflict,
	old FieldsV1,
	req FieldsV1,
	force bool,
) FieldsV1 {
	// Object values.
	for key, value := range req.Fields {
		// If the key exists and the field is a leaf (no children), then we have
		// a conflict (unless force).
		// If the key exists, but it is not a leaf there is no conflict and we
		// need to recurse.
		if subField, ok := old.Fields[key]; ok {
			if value.IsLeaf() {
				if !force {
					conflicts.Fields = append(conflicts.Fields, value)
					break
				}
				// If force is true, remove ownership of the field from old.
				delete(old.Fields, key)
				break
			}
			subField = conflictOrForceOverrideFields(
				conflicts,
				subField,
				value,
				force,
			)
			// After traversing subField, if it is not a leaf (i.e. it has no
			// fields or elements) we want to remove it altogether.
			if subField.IsLeaf() {
				delete(old.Fields, key)
				continue
			}
			old.Fields[key] = subField
		}
	}
	// Same as for above but with elements (arrays).
	for key, value := range req.Elements {
		if subField, ok := old.Elements[key]; ok {
			if value.IsLeaf() {
				if !force {
					conflicts.Fields = append(conflicts.Fields, value)
					break
				}
				// If force is true, remove ownership of the field from old.
				delete(old.Elements, key)
				break
			}
			subField = conflictOrForceOverrideFields(
				conflicts,
				subField,
				value,
				force,
			)
			// After traversing subField, if it is not a leaf (i.e. it has no
			// fields or elements) we want to remove it altogether.
			if subField.IsLeaf() {
				delete(old.Fields, key)
				continue
			}
			old.Elements[key] = subField
		}
	}
	return old
}

func fieldsDiff(mr *MergeResult, oldFields, newFields FieldsV1) {
	// Check diff in fields (objects).
	for oldKey, oldValue := range oldFields.Fields {
		newValue, ok := newFields.Fields[oldKey]
		if !ok {
			mr.Removed = append(mr.Removed, oldValue)
			continue
		}
		fieldsDiff(mr, oldValue, newValue)
	}
	// Check diff in elements (arrays).
	for oldKey, oldValue := range oldFields.Elements {
		newValue, ok := newFields.Elements[oldKey]
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
		if err := purgeRemovedFieldsObject(obj, field.Path()); err != nil {
			return err
		}
	}
	return nil
}

func purgeRemovedFieldsObject(
	obj map[string]interface{},
	path []FieldsV1Step,
) error {
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
		if err := purgeRemovedFieldsObject(v, path[1:]); err != nil {
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

	arrayVal, err := purgeRemovedFieldsArray(stepObj.([]interface{}), path[1:])
	if err != nil {
		return err
	}
	obj[step.Key.Key] = arrayVal

	return nil
}

func purgeRemovedFieldsArray(
	obj []interface{},
	path []FieldsV1Step,
) ([]interface{}, error) {
	// Get the next step in the path and find the index of the array element.
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
	// If there are no more steps in the path, we remove the found element from
	// the array.
	if len(path) == 1 {
		// Remove the element from the array.
		return append(obj[:index], obj[index+1:]...), nil
	}
	// If there more steps in the path, we need to recurse.
	nextObj, ok := obj[index].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf(
			"expected map[string]interface{}, got %T at %q",
			obj[index],
			step.String(),
		)
	}
	if err := purgeRemovedFieldsObject(nextObj, path[1:]); err != nil {
		return nil, err
	}
	return obj, nil
}

// MergeObjects merges the src map into the dst map according to the fields.
// It is expected that the fields have been generated from the src object/data.
func MergeObjects(
	dst map[string]interface{},
	src map[string]interface{},
	fields FieldsV1,
) {
	mergeObjects(dst, src, fields)
}

func mergeObjects(
	dst map[string]interface{},
	src map[string]interface{},
	fields FieldsV1,
) {
	for key, subFields := range fields.Fields {
		// If the field is a leaf, we can just copy the value from src to dst.
		// Else, we need to recurse.
		if subFields.IsLeaf() {
			dst[key.Key] = src[key.Key]
			continue
		}
		dstField, ok := dst[key.Key]
		if !ok {
			dst[key.Key] = src[key.Key]
			continue
		}
		if len(subFields.Fields) > 0 {
			mergeObjects(
				dstField.(map[string]interface{}),
				src[key.Key].(map[string]interface{}),
				subFields,
			)
		}
		if len(subFields.Elements) > 0 {
			dstField = mergeArray(
				dstField.([]interface{}),
				src[key.Key].([]interface{}),
				subFields,
			)
		}
		dst[key.Key] = dstField
	}
}

func mergeArray(
	dst []interface{},
	src []interface{},
	fields FieldsV1,
) []interface{} {
	for key, subFields := range fields.Elements {
		dstIndex := FindIndexArrayByKey(dst, key)
		srcIndex := FindIndexArrayByKey(src, key)
		// If the field is not found in dst, we can just append the value from
		// src to dst.
		if dstIndex == -1 {
			dst = append(dst, src[srcIndex])
			continue
		}
		dstObj := dst[dstIndex].(map[string]interface{})
		srcObj := src[srcIndex].(map[string]interface{})
		mergeObjects(dstObj, srcObj, subFields)
		dst[dstIndex] = dstObj
	}
	return dst
}
