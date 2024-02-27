package hz

import (
	"encoding/json"
	"fmt"

	"github.com/verifa/horizon/pkg/internal/managedfields"
)

// ExtractManagedFields creates an object containing the fields managed by the
// given manager. If the manager does not manage any fields, the object will be
// empty (except for the identifying fields, like metadata.name,
// metadata.account).
//
// The use of ExtractManagedFields is for a controller (for example) to get an
// object from the store, extract the fields it manages, and then update the
// object with the managed fields.
func ExtractManagedFields[T Objecter](
	object T,
	manager string,
) (T, error) {
	var t T
	bObject, err := json.Marshal(object)
	if err != nil {
		return t, fmt.Errorf("marshalling object: %w", err)
	}
	var src map[string]interface{}
	if err := json.Unmarshal(bObject, &src); err != nil {
		return t, fmt.Errorf("unmarshalling object: %w", err)
	}

	fieldManager, ok := object.ObjectManagedFields().FieldManager(manager)
	if !ok {
		// If the field manager is not found, this manager currently owns no
		// fields in the object.
		// Therefore return an empty object with the necessary fields set (i.e.
		// kind, apiVersion, metadata.name, metadata.account).
		dst := map[string]interface{}{}
		copyObjectIDToMap(object, dst)
		t, err := mapToObject[T](dst)
		if err != nil {
			return t, fmt.Errorf("converting dst map to object: %w", err)
		}
		return t, nil
	}
	ownedFields, err := managedfields.ExtractFieldsV1Object(
		src,
		fieldManager.FieldsV1,
	)
	if err != nil {
		return t, fmt.Errorf("extracting fields: %w", err)
	}
	copyObjectIDToMap(object, ownedFields)

	t, err = mapToObject[T](ownedFields)
	if err != nil {
		return t, fmt.Errorf("converting dst map to object: %w", err)
	}
	return t, nil
}

func copyObjectIDToMap(
	obj Objecter,
	m map[string]interface{},
) {
	meta, ok := m["metadata"].(map[string]interface{})
	if !ok {
		meta = map[string]interface{}{
			"name":    obj.ObjectName(),
			"account": obj.ObjectAccount(),
		}
		m["metadata"] = meta
		return
	}
	meta["name"] = obj.ObjectName()
	meta["account"] = obj.ObjectAccount()
	m["metadata"] = meta
}

func mapToObject[T Objecter](m map[string]interface{}) (T, error) {
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
