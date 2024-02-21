package auth

import (
	"encoding/json"
	"testing"

	"github.com/verifa/horizon/pkg/hz"
)

// TODO: the real trick is how to sync the groups-->account relations when roles
// and rolebdings change.

func TestRBAC(t *testing.T) {
	type testcase struct {
		req    ObjectRequest
		expect bool
	}

	type test struct {
		name     string
		roles    []Role
		bindings []RoleBinding

		cases []testcase
	}

	testCreateAllowsRead := test{
		name: "allow-create-allows-read",
		roles: []Role{
			{
				ObjectMeta: hz.ObjectMeta{
					Name:    "role-creator",
					Account: "account-test",
				},
				Spec: RoleSpec{
					Allow: []Verbs{
						{
							Create: &VerbFilter{
								Kind:  hz.P("object-test"),
								Group: hz.P("group-test"),
							},
						},
					},
				},
			},
		},
		bindings: []RoleBinding{
			{
				ObjectMeta: hz.ObjectMeta{
					Name:    "rolebinding-test",
					Account: "account-test",
				},
				Spec: RoleBindingSpec{
					RoleRef: RoleRef{
						Name: "role-creator",
					},
					Subjects: []Subject{
						{
							Kind: "Group",
							Name: "group-creator",
						},
					},
				},
			},
		},
		cases: []testcase{
			{
				req: ObjectRequest{
					User:   "Margarine",
					Groups: []string{"group-creator"},
					Verb:   "read",
					Object: hz.Key{
						Name:    "account-test",
						Account: hz.RootAccount,
						Kind:    "Account",
					},
				},
				expect: true,
			},
			// {
			// 	req: ObjectRequest{
			// 		User:   "Margarine",
			// 		Groups: []string{"group-creator"},
			// 		Verb:   "read",
			// 		Object: hz.Key{
			// 			Name:    "account-another",
			// 			Account: hz.RootAccount,
			// 			Kind:    "Account",
			// 		},
			// 	},
			// 	expect: false,
			// },
			// {
			// 	req: ObjectRequest{
			// 		User:   "Margarine",
			// 		Groups: []string{"group-creator"},
			// 		Verb:   "read",
			// 		Object: hz.Key{
			// 			Name:    "superfluous",
			// 			Account: "account-test",
			// 			Kind:    "object-test",
			// 		},
			// 	},
			// 	expect: true,
			// },
			// {
			// 	req: ObjectRequest{
			// 		User:   "Margarine",
			// 		Groups: []string{"group-creator"},
			// 		Verb:   "create",
			// 		Object: hz.Key{
			// 			Name:    "superfluous",
			// 			Account: "account-test",
			// 			Kind:    "object-test",
			// 		},
			// 	},
			// 	expect: true,
			// },
			// {
			// 	req: ObjectRequest{
			// 		User:   "Margarine",
			// 		Groups: []string{"group-creator"},
			// 		Verb:   "delete",
			// 		Object: hz.Key{
			// 			Name:    "superfluous",
			// 			Account: "account-test",
			// 			Kind:    "object-test",
			// 		},
			// 	},
			// 	expect: false,
			// },
			// {
			// 	req: ObjectRequest{
			// 		User:   "Margarine",
			// 		Groups: []string{"group-unknown"},
			// 		Verb:   "read",
			// 		Object: hz.Key{
			// 			Name:    "superfluous",
			// 			Account: "account-test",
			// 			Kind:    "object-test",
			// 		},
			// 	},
			// 	expect: false,
			// },
		},
	}

	tests := []test{testCreateAllowsRead}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rbac := RBAC{
				RoleBindings: make(map[string]RoleBinding),
				Roles:        make(map[string]Role),
				Permissions:  make(map[string]*Group),
			}
			for _, role := range test.roles {
				rbac.HandleRoleEvent(event(t, role))
			}
			for _, binding := range test.bindings {
				rbac.HandleRoleBindingEvent(event(t, binding))
			}
			for index, tc := range test.cases {
				ok := rbac.CheckObject(tc.req)
				if ok != tc.expect {
					t.Fatal("test case failed: ", index)
				}
			}
		})
	}
}

func event[T hz.Objecter](t *testing.T, obj T) hz.Event {
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshalling object: %v", err)
	}
	return hz.Event{
		Operation: hz.EventOperationPut,
		Data:      data,
		Key:       hz.KeyForObject(obj),
	}
}
