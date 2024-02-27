package hz_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/managedfields"
	tu "github.com/verifa/horizon/pkg/testutil"
)

var cmpOptIgnoreMetaManagedFields = cmp.FilterPath(func(p cmp.Path) bool {
	return p.Last().String() == ".ManagedFields" &&
		p.Last().Type() == reflect.TypeOf(managedfields.ManagedFields{})
}, cmp.Ignore())

func TestExtractObjectFields(t *testing.T) {
	obj := extractFieldsObject{
		ObjectMeta: hz.ObjectMeta{
			Name:    "name",
			Account: "account",
			Labels:  map[string]string{"label": "value"},
		},
		Spec: &struct {
			Foo *string `json:"foo,omitempty"`
			Bar *string `json:"bar,omitempty"`
		}{
			Foo: hz.P("foo"),
			Bar: hz.P("bar"),
		},
	}

	raw, err := json.Marshal(obj)
	tu.AssertNoError(t, err)
	fields, err := managedfields.ManagedFieldsV1(raw)
	tu.AssertNoError(t, err)
	obj.ObjectMeta.ManagedFields = managedfields.ManagedFields{
		{
			Manager:    "test",
			FieldsType: "FieldsV1",
			FieldsV1:   fields,
		},
	}

	objManagedFields, err := hz.ExtractManagedFields[extractFieldsObject](
		obj,
		"test",
	)
	tu.AssertNoError(t, err)
	tu.AssertEqual(t, obj, objManagedFields, cmpOptIgnoreMetaManagedFields)

	idObj := extractFieldsObject{
		ObjectMeta: hz.ObjectMeta{
			Name:    obj.Name,
			Account: obj.Account,
		},
	}

	idObjManagedFields, err := hz.ExtractManagedFields[extractFieldsObject](
		idObj,
		"not-a-manager",
	)
	tu.AssertNoError(t, err)
	tu.AssertEqual(t, idObj, idObjManagedFields, cmpOptIgnoreMetaManagedFields)
}

var _ hz.Objecter = (*extractFieldsObject)(nil)

type extractFieldsObject struct {
	hz.ObjectMeta `json:"metadata,omitempty"`

	Spec *struct {
		Foo *string `json:"foo,omitempty"`
		Bar *string `json:"bar,omitempty"`
	} `json:"spec,omitempty"`
	Status *struct {
		Baz *string `json:"baz,omitempty"`
	} `json:"status,omitempty"`
}

func (o extractFieldsObject) ObjectKind() string {
	return "extractFieldsObject"
}

func (o extractFieldsObject) ObjectGroup() string {
	return "extractFieldsGroup"
}

func (o extractFieldsObject) ObjectVersion() string {
	return "v1"
}
