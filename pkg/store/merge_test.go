package store_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/store"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestMergeManagedFields(t *testing.T) {
	type test struct {
		name          string
		managedFields string
		merge         string
		expConflict   func(hz.FieldsV1) []hz.FieldsV1
		expRemoved    func([]hz.FieldManager) []hz.FieldsV1
	}
	tests := []test{
		{
			name: "object",
			managedFields: `[
				{
					"manager": "m1",
					"fieldsV1": {
						"f:metadata": {
							"f:name": {},
							"f:labels": {
								"f:app": {}
							}
						}
					}
				}
			]`,
			merge: `{
				"manager": "m2",
				"fieldsV1": {
					"f:metadata": {
						"f:labels": {
							"f:test": {}
						}
					}
				}
			}`,
			expConflict: func(fields hz.FieldsV1) []hz.FieldsV1 { return nil },
			expRemoved:  func(fms []hz.FieldManager) []hz.FieldsV1 { return nil },
		},
		{
			name: "array",
			managedFields: `[
				{
					"manager": "m1",
					"fieldsV1": {
						"f:slice": {
							"k:{\"id\":\"1\"}": {}
						}
					}
				}
			]`,
			merge: `{
				"manager": "m2",
				"fieldsV1": {
					"f:slice": {
						"k:{\"id\":\"2\"}": {}
					}
				}
			}`,
			expConflict: func(fields hz.FieldsV1) []hz.FieldsV1 { return nil },
			expRemoved:  func(fms []hz.FieldManager) []hz.FieldsV1 { return nil },
		},
		{
			name: "removed object",
			managedFields: `[
				{
					"manager": "m1",
					"fieldsV1": {
						"f:metadata": {
							"f:name": {},
							"f:labels": {
								"f:app": {}
							}
						}
					}
				}
			]`,
			merge: `{
				"manager": "m1",
				"fieldsV1": {
					"f:metadata": {
						"f:name": {}
					}
				}
			}`,
			expConflict: func(fields hz.FieldsV1) []hz.FieldsV1 { return nil },
			expRemoved: func(fms []hz.FieldManager) []hz.FieldsV1 {
				return []hz.FieldsV1{
					fms[0].FieldsV1.Fields[fkey("metadata")].Fields[fkey("labels")],
				}
			},
		},
		{
			name: "removed array",
			managedFields: `[
				{
					"manager": "m1",
					"fieldsV1": {
						"f:slice": {
							"k:{\"id\":\"1\"}": {},
							"k:{\"id\":\"2\"}": {}
						}
					}
				}
			]`,
			merge: `{
				"manager": "m1",
				"fieldsV1": {
					"f:slice": {
						"k:{\"id\":\"1\"}": {}
					}
				}
			}`,
			expConflict: func(fields hz.FieldsV1) []hz.FieldsV1 { return nil },
			expRemoved: func(fms []hz.FieldManager) []hz.FieldsV1 {
				return []hz.FieldsV1{
					fms[0].FieldsV1.Fields[fkey("slice")].Elements[hz.FieldsV1Key{
						Type:  hz.FieldsV1KeyArray,
						Key:   "id",
						Value: "2",
					}],
				}
			},
		},
		{
			name: "conflict object",
			managedFields: `[
				{
					"manager": "m1",
					"fieldsV1": {
						"f:metadata": {
							"f:name": {}
						}
					}
				}
			]`,
			merge: `{
				"manager": "m2",
				"fieldsV1": {
					"f:metadata": {
						"f:name": {}
					}
				}
			}`,
			expConflict: func(fields hz.FieldsV1) []hz.FieldsV1 {
				return []hz.FieldsV1{
					fields.Fields[fkey("metadata")].Fields[fkey("name")],
				}
			},
			expRemoved: func(fms []hz.FieldManager) []hz.FieldsV1 { return nil },
		},
		{
			name: "conflict array",
			managedFields: `[
				{
					"manager": "m1",
					"fieldsV1": {
						"f:slice": {
							"k:{\"id\":\"1\"}": {}
						}
					}
				}
			]`,
			merge: `{
				"manager": "m2",
				"fieldsV1": {
					"f:slice": {
						"k:{\"id\":\"1\"}": {}
					}
				}
			}`,
			expConflict: func(fields hz.FieldsV1) []hz.FieldsV1 {
				return []hz.FieldsV1{
					fields.Fields[fkey("slice")].Elements[hz.FieldsV1Key{
						Type:  hz.FieldsV1KeyArray,
						Key:   "id",
						Value: "1",
					}],
				}
			},
			expRemoved: func(fms []hz.FieldManager) []hz.FieldsV1 { return nil },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var managedFields []hz.FieldManager
			err := json.Unmarshal([]byte(tc.managedFields), &managedFields)
			tu.AssertNoError(t, err, "parsing managed fields json")
			var merge hz.FieldManager
			err = json.Unmarshal([]byte(tc.merge), &merge)
			tu.AssertNoError(t, err, "parsing field manager json")

			var expErr error
			expConflictFields := tc.expConflict(merge.FieldsV1)
			if len(expConflictFields) > 0 {
				expErr = &store.Conflict{
					Fields: expConflictFields,
				}
			}
			expRm := tc.expRemoved(managedFields)
			result, err := store.MergeManagedFields(managedFields, merge)
			tu.AssertEqual(t, expErr, err, cmpOptIgnoreParent)
			tu.AssertEqual(
				t,
				expRm,
				result.Removed,
				cmpOptIgnoreParent,
			)
		})
	}
	mf := []hz.FieldManager{
		{
			Manager: "m1",
			FieldsV1: hz.FieldsV1{
				Fields: map[hz.FieldsV1Key]hz.FieldsV1{
					fkey("metadata"): {
						Fields: map[hz.FieldsV1Key]hz.FieldsV1{
							fkey("name"): {},
						},
					},
					fkey("spec"): {
						Fields: map[hz.FieldsV1Key]hz.FieldsV1{
							fkey("replicas"): {},
						},
					},
				},
			},
		},
		{
			Manager: "m2",
			FieldsV1: hz.FieldsV1{
				Fields: map[hz.FieldsV1Key]hz.FieldsV1{
					fkey("metadata"): {
						Fields: map[hz.FieldsV1Key]hz.FieldsV1{
							fkey("name"): {},
						},
					},
					fkey("spec"): {
						Fields: map[hz.FieldsV1Key]hz.FieldsV1{
							fkey("field"): {},
						},
					},
				},
			},
		},
	}

	m2 := hz.FieldManager{
		Manager: "m2",
		FieldsV1: hz.FieldsV1{
			Fields: map[hz.FieldsV1Key]hz.FieldsV1{
				fkey("metadata"): {
					Fields: map[hz.FieldsV1Key]hz.FieldsV1{
						fkey("name"): {},
					},
				},
			},
		},
	}

	result, _ := store.MergeManagedFields(mf, m2)
	fmt.Println(result.Removed)
}

func TestMergeObjects(t *testing.T) {
	type test struct {
		name string
		dst  string
		src  string
		exp  string
	}
	tests := []test{
		{
			name: "objects",
			dst: `{
				"metadata": {
					"name": "test",
					"labels": {
						"app": "test"
					}
				}
			}`,
			src: `{
				"metadata": {
					"name": "test",
					"labels": {
						"test": "test"
					}
				}
			}`,
			exp: `{
				"metadata": {
					"name": "test",
					"labels": {
						"app": "test",
						"test": "test"
					}
				}
			}`,
		},
		{
			name: "arrays",
			dst: `{
				"slice": ["a"],
				"objects":[
					{
						"id": "1",
						"field": "value"
					}
				],
				"nested":[
					{
						"id": "1",
						"slice": ["a"],
						"object": {
							"a": "a",
							"z": "z"
						}
					}
				]
			}`,
			src: `{
				"slice": ["overwrite"],
				"objects":[
					{
						"id": "1",
						"field": "overwrite"
					},
					{
						"id": "2",
						"field": "append"
					}
				],
				"nested":[
					{
						"id": "1",
						"slice": ["overwrite"],
						"object": {
							"b": "b"
						}
					}
				]
			}`,
			exp: `{
				"slice": ["overwrite"],
				"objects":[
					{
						"id": "1",
						"field": "overwrite"
					},
					{
						"id": "2",
						"field": "append"
					}
				],
				"nested":[
					{
						"id": "1",
						"slice": ["overwrite"],
						"object": {
							"a": "a",
							"b": "b",
							"z": "z"
						}
					}
				]
			}`,
		},
		{
			name: "complex",
			dst: `{
				"metadata": {
					"name": "test",
					"labels": {
						"app": "test"
					}
				},
				"spec": {
					"replicas": 3,
					"objslice": [
						{"id": "1", "field": "value"}
					]
				}
			}`,
			src: `{
				"metadata": {
					"name": "test",
					"labels": {
						"test": "test"
					}
				},
				"spec": {
					"replicas": 4,
					"objslice": [
						{"id": "2", "field": "value"}
					]
				}
			}`,
			exp: `{
				"metadata": {
					"name": "test",
					"labels": {
						"app": "test",
						"test": "test"
					}
				},
				"spec": {
					"replicas": 4,
					"objslice": [
						{"id": "1", "field": "value"},
						{"id": "2", "field": "value"}
					]
				}
			}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var dst, src, exp map[string]interface{}
			err := json.Unmarshal([]byte(tc.dst), &dst)
			tu.AssertNoError(t, err, "parsing dst json")
			err = json.Unmarshal([]byte(tc.src), &src)
			tu.AssertNoError(t, err, "parsing src json")
			err = json.Unmarshal([]byte(tc.exp), &exp)
			tu.AssertNoError(t, err, "parsing exp json")
			store.MergeObjects(dst, src)
			tu.AssertEqual(t, exp, dst)
		})
	}
}

func TestPurgeRemoveFields(t *testing.T) {
	type test struct {
		name    string
		obj     map[string]interface{}
		exp     map[string]interface{}
		removed func(hz.FieldsV1) []hz.FieldsV1
	}
	tests := []test{
		{
			name: "object",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "test",
				},
			},
			exp: map[string]interface{}{
				"metadata": map[string]interface{}{},
			},
			removed: func(fields hz.FieldsV1) []hz.FieldsV1 {
				return []hz.FieldsV1{
					fields.Fields[fkey("metadata")].Fields[fkey("name")],
				}
			},
		},
		{
			name: "array",
			obj: map[string]interface{}{
				"slice": []interface{}{
					map[string]interface{}{
						"id":    "1",
						"field": "value",
					},
				},
			},
			exp: map[string]interface{}{
				"slice": []interface{}{},
			},
			removed: func(fields hz.FieldsV1) []hz.FieldsV1 {
				return []hz.FieldsV1{
					fields.Fields[fkey("slice")].Elements[hz.FieldsV1Key{
						Type:  hz.FieldsV1KeyArray,
						Key:   "id",
						Value: "1",
					}],
				}
			},
		},
		{
			name: "complex",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "test",
					"labels": map[string]interface{}{
						"app": "test",
					},
				},
				"spec": map[string]interface{}{
					"replicas": 3,
					"objslice": []interface{}{
						map[string]interface{}{
							"id":    "1",
							"field": "value",
						},
						map[string]interface{}{
							"id":    "2",
							"field": "value",
						},
					},
				},
			},
			exp: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":   "test",
					"labels": map[string]interface{}{
						// "app": "test",
					},
				},
				"spec": map[string]interface{}{
					"replicas": 3,
					"objslice": []interface{}{
						map[string]interface{}{
							"id":    "2",
							"field": "value",
						},
					},
				},
			},
			removed: func(fields hz.FieldsV1) []hz.FieldsV1 {
				return []hz.FieldsV1{
					fields.Fields[fkey("metadata")].Fields[fkey("labels")].Fields[fkey("app")],
					fields.Fields[fkey("spec")].Fields[fkey("objslice")].Elements[hz.FieldsV1Key{
						Type:  hz.FieldsV1KeyArray,
						Key:   "id",
						Value: "1",
					}],
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields := store.ManagedFieldsV1Object(nil, tc.obj)
			removed := tc.removed(fields)
			err := store.PurgeRemovedFields(tc.obj, removed)
			tu.AssertNoError(t, err)
			tu.AssertEqual(t, tc.exp, tc.obj)
		})
	}
}
