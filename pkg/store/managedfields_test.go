package store_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/store"
	tu "github.com/verifa/horizon/pkg/testutil"
)

var cmpOptIgnoreRevision = cmp.FilterPath(func(p cmp.Path) bool {
	if len(p) != 4 {
		return false
	}
	return p.Index(1).String() == "[\"metadata\"]" &&
		p.Last().String() == "[\"revision\"]"
}, cmp.Ignore())

var cmpOptIgnoreParent = cmp.FilterPath(func(p cmp.Path) bool {
	// Ignore the parent field.
	return p.Last().Type() == reflect.TypeOf(&hz.FieldsV1Step{})
}, cmp.Ignore())

func fkey(k string) hz.FieldsV1Key {
	return hz.FieldsV1Key{
		Key: k,
	}
}

func TestManagedFieldsV1(t *testing.T) {
	type test struct {
		name string
		json string
		exp  hz.FieldsV1
	}
	tests := []test{
		{
			name: "object",
			json: `
			{
				"object": {
					"name": "test"
				}
			}`,
			exp: hz.FieldsV1{
				Fields: map[hz.FieldsV1Key]hz.FieldsV1{
					fkey("object"): {
						Fields: map[hz.FieldsV1Key]hz.FieldsV1{
							fkey("name"): {},
						},
					},
				},
			},
		},
		{
			name: "array",
			json: `
			{
				"slice": [
					{"id": "1", "field": "value"}
				]
			}`,
			exp: hz.FieldsV1{
				Fields: map[hz.FieldsV1Key]hz.FieldsV1{
					fkey("slice"): {
						Elements: map[hz.FieldsV1Key]hz.FieldsV1{
							{
								Type:  hz.FieldsV1KeyArray,
								Key:   "id",
								Value: "1",
							}: {
								Fields: map[hz.FieldsV1Key]hz.FieldsV1{
									fkey("id"):    {},
									fkey("field"): {},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "complex",
			json: `{
			"metadata": {
				"name": "test"
			},
			"spec": {
				"replicas": 3,
				"slice": [
						"one",
						"two",
						"three"
					],
					"objSlice": [
						{
							"id": "1",
							"field": "value"
						},
						{
							"id": "2",
							"field": "value"
						}
					],
					"template": {
						"metadata": {
							"labels": {
								"app": "test"
							}
						}
					}
				}
			}`,
			exp: hz.FieldsV1{
				Fields: map[hz.FieldsV1Key]hz.FieldsV1{
					fkey("spec"): {
						Fields: map[hz.FieldsV1Key]hz.FieldsV1{
							fkey("replicas"): {},
							fkey("slice"):    {},
							fkey("objSlice"): {
								Elements: map[hz.FieldsV1Key]hz.FieldsV1{
									{
										Type:  hz.FieldsV1KeyArray,
										Key:   "id",
										Value: "1",
									}: {
										Fields: map[hz.FieldsV1Key]hz.FieldsV1{
											fkey("id"):    {},
											fkey("field"): {},
										},
									},
									{
										Type:  hz.FieldsV1KeyArray,
										Key:   "id",
										Value: "2",
									}: {
										Fields: map[hz.FieldsV1Key]hz.FieldsV1{
											fkey("id"):    {},
											fkey("field"): {},
										},
									},
								},
							},
							fkey("template"): {
								Fields: map[hz.FieldsV1Key]hz.FieldsV1{
									fkey("metadata"): {
										Fields: map[hz.FieldsV1Key]hz.FieldsV1{
											fkey("labels"): {
												Fields: map[hz.FieldsV1Key]hz.FieldsV1{
													fkey("app"): {},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Traverse raw and calculate managed fields.
			fields, err := store.ManagedFieldsV1([]byte(tc.json))
			tu.AssertNoError(t, err)
			tu.AssertEqual(t, tc.exp, fields, cmpOptIgnoreParent)

			bFields, err := json.Marshal(fields)
			tu.AssertNoError(t, err)

			uFields := hz.FieldsV1{}
			err = json.Unmarshal(bFields, &uFields)
			tu.AssertNoError(t, err)

			tu.AssertEqual(t, tc.exp, uFields, cmpOptIgnoreParent)

			checkParent(t, fields)
		})
	}
}

func checkParent(t *testing.T, fields hz.FieldsV1) {
	for _, field := range fields.Fields {
		if field.Parent == nil {
			t.Errorf("parent is nil")
		}
		checkParent(t, field)
	}
	for _, field := range fields.Elements {
		if field.Parent == nil {
			t.Errorf("parent is nil")
		}
		checkParent(t, field)
	}
}
