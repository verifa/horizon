package managedfields

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	tu "github.com/verifa/horizon/pkg/testutil"
)

var cmpOptIgnoreParent = cmp.FilterPath(func(p cmp.Path) bool {
	// Ignore the parent field.
	return p.Last().Type() == reflect.TypeOf(&FieldsV1Step{})
}, cmp.Ignore())

func fkey(k string) FieldsV1Key {
	return FieldsV1Key{
		Key: k,
	}
}

func TestManagedFieldsV1(t *testing.T) {
	type test struct {
		name string
		json string
		exp  FieldsV1
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
			exp: FieldsV1{
				Fields: map[FieldsV1Key]FieldsV1{
					fkey("object"): {
						Fields: map[FieldsV1Key]FieldsV1{
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
			exp: FieldsV1{
				Fields: map[FieldsV1Key]FieldsV1{
					fkey("slice"): {
						Elements: map[FieldsV1Key]FieldsV1{
							{
								Type:  FieldsV1KeyArray,
								Key:   "id",
								Value: "1",
							}: {
								Fields: map[FieldsV1Key]FieldsV1{
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
			name: "empty-metadata",
			json: `{
				"metadata": {
					"name": "test",
					"namespace": "test"
				},
				"spec": {
					"replicas": 3
				}
			}`,
			exp: FieldsV1{
				Fields: map[FieldsV1Key]FieldsV1{
					fkey("spec"): {
						Fields: map[FieldsV1Key]FieldsV1{
							fkey("replicas"): {},
						},
					},
				},
			},
		},
		{
			name: "complex",
			json: `{
			"metadata": {
				"name": "test",
				"labels": {
					"app": "test"
				}
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
			exp: FieldsV1{
				Fields: map[FieldsV1Key]FieldsV1{
					fkey("metadata"): {
						Fields: map[FieldsV1Key]FieldsV1{
							fkey("labels"): {
								Fields: map[FieldsV1Key]FieldsV1{
									fkey("app"): {},
								},
							},
						},
					},
					fkey("spec"): {
						Fields: map[FieldsV1Key]FieldsV1{
							fkey("replicas"): {},
							fkey("slice"):    {},
							fkey("objSlice"): {
								Elements: map[FieldsV1Key]FieldsV1{
									{
										Type:  FieldsV1KeyArray,
										Key:   "id",
										Value: "1",
									}: {
										Fields: map[FieldsV1Key]FieldsV1{
											fkey("id"):    {},
											fkey("field"): {},
										},
									},
									{
										Type:  FieldsV1KeyArray,
										Key:   "id",
										Value: "2",
									}: {
										Fields: map[FieldsV1Key]FieldsV1{
											fkey("id"):    {},
											fkey("field"): {},
										},
									},
								},
							},
							fkey("template"): {
								Fields: map[FieldsV1Key]FieldsV1{
									fkey("metadata"): {
										Fields: map[FieldsV1Key]FieldsV1{
											fkey("labels"): {
												Fields: map[FieldsV1Key]FieldsV1{
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
			fields, err := ManagedFieldsV1([]byte(tc.json))
			tu.AssertNoError(t, err)
			tu.AssertEqual(t, tc.exp, fields, cmpOptIgnoreParent)

			bFields, err := json.Marshal(fields)
			tu.AssertNoError(t, err)

			uFields := FieldsV1{}
			err = json.Unmarshal(bFields, &uFields)
			tu.AssertNoError(t, err)

			tu.AssertEqual(t, tc.exp, uFields, cmpOptIgnoreParent)

			checkParent(t, fields)
		})
	}
}

func checkParent(t *testing.T, fields FieldsV1) {
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
