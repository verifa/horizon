package hz_test

import (
	"encoding/json"
	"testing"

	"github.com/verifa/horizon/pkg/hz"
	tu "github.com/verifa/horizon/pkg/testutil"
)

var _ (hz.Objecter) = (*DummyObject)(nil)

type DummyObject struct {
	hz.ObjectMeta `json:"metadata,omitempty" cue:""`

	Spec   struct{} `json:"spec,omitempty" cue:""`
	Status struct{} `json:"status,omitempty"`
}

func (o DummyObject) ObjectGroup() string {
	return "DummyGroup"
}

func (o DummyObject) ObjectVersion() string {
	return "v1"
}

func (o DummyObject) ObjectKind() string {
	return "DummyObject"
}

type ChildObject struct {
	hz.ObjectMeta `json:"metadata,omitempty"`

	Spec struct{} `json:"spec,omitempty" cue:",opt"`
}

func (o ChildObject) ObjectGroup() string {
	return "ChildGroup"
}

func (o ChildObject) ObjectVersion() string {
	return "v1"
}

func (o ChildObject) ObjectKind() string {
	return "ChildObject"
}

func TestGenericObjectMarshal(t *testing.T) {
	expObj := map[string]interface{}{
		"apiVersion": "core/v1",
		"kind":       "Whatever",
		"metadata": map[string]interface{}{
			"name":    "my-object",
			"account": "my-account",
		},
		"data": map[string]interface{}{
			"data_field": "some_data",
		},
		"spec": map[string]interface{}{
			"spec_field": "some_value",
		},
		"status": map[string]interface{}{
			"ready": true,
			"phase": "Pending",
		},
		"field": "a",
	}

	expObjJSON, err := json.MarshalIndent(expObj, "", "  ")
	tu.AssertNoError(t, err)

	var genObj hz.GenericObject
	err = json.Unmarshal(expObjJSON, &genObj)
	tu.AssertNoError(t, err)

	genObjJSON, err := json.MarshalIndent(genObj, "", "  ")
	tu.AssertNoError(t, err)

	var actObj map[string]interface{}
	err = json.Unmarshal(genObjJSON, &actObj)
	tu.AssertNoError(t, err)

	tu.AssertEqual(t, expObj, actObj)
}
