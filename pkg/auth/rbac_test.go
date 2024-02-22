package auth

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/verifa/horizon/pkg/hz"
)

func TestRBAC(t *testing.T) {
	ctx := context.Background()
	type testcase struct {
		req    RBACRequest
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
				req: RBACRequest{
					Groups: []string{"group-creator"},
					Verb:   "read",
					Object: hz.ObjectKey{
						Name:    "account-test",
						Account: hz.RootAccount,
						Kind:    "Account",
						Group:   "group-test",
					},
				},
				expect: true,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-creator"},
					Verb:   "read",
					Object: hz.ObjectKey{
						Name:    "account-another",
						Account: hz.RootAccount,
						Kind:    "Account",
						Group:   "group-test",
					},
				},
				expect: false,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-creator"},
					Verb:   "read",
					Object: hz.ObjectKey{
						Name:    "superfluous",
						Account: "account-test",
						Kind:    "object-test",
						Group:   "group-test",
					},
				},
				expect: true,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-creator"},
					Verb:   "create",
					Object: hz.ObjectKey{
						Name:    "superfluous",
						Account: "account-test",
						Kind:    "object-test",
						Group:   "group-test",
					},
				},
				expect: true,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-creator"},
					Verb:   "delete",
					Object: hz.ObjectKey{
						Name:    "superfluous",
						Account: "account-test",
						Kind:    "object-test",
						Group:   "group-test",
					},
				},
				expect: false,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-unknown"},
					Verb:   "read",
					Object: hz.ObjectKey{
						Name:    "superfluous",
						Account: "account-test",
						Kind:    "object-test",
						Group:   "group-test",
					},
				},
				expect: false,
			},
		},
	}

	testAllowRun := test{
		name: "allow-run",
		roles: []Role{
			{
				ObjectMeta: hz.ObjectMeta{
					Name:    "role-runner",
					Account: "account-test",
				},
				Spec: RoleSpec{
					Allow: []Verbs{
						{
							Run: &VerbFilter{
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
						Name: "role-runner",
					},
					Subjects: []Subject{
						{
							Kind: "Group",
							Name: "group-runner",
						},
					},
				},
			},
		},
		cases: []testcase{
			{
				req: RBACRequest{
					Groups: []string{"group-runner"},
					Verb:   "run",
					Object: hz.ObjectKey{
						Name:    "superfluous",
						Account: "account-test",
						Kind:    "object-test",
						Group:   "group-test",
					},
				},
				expect: true,
			},
		},
	}
	testDenyDelete := test{
		name: "deny-delete",
		roles: []Role{
			{
				ObjectMeta: hz.ObjectMeta{
					Name:    "role-allow-all",
					Account: "account-test",
				},
				Spec: RoleSpec{
					Allow: []Verbs{
						{
							Read:   &VerbFilter{},
							Update: &VerbFilter{},
							Create: &VerbFilter{},
							Delete: &VerbFilter{},
							Run:    &VerbFilter{},
						},
					},
				},
			},
			{
				ObjectMeta: hz.ObjectMeta{
					Name:    "role-deny-delete",
					Account: "account-test",
				},
				Spec: RoleSpec{
					Deny: []Verbs{
						{
							Delete: &VerbFilter{},
						},
					},
				},
			},
		},
		bindings: []RoleBinding{
			{
				ObjectMeta: hz.ObjectMeta{
					Name:    "rolebinding-allow-all",
					Account: "account-test",
				},
				Spec: RoleBindingSpec{
					RoleRef: RoleRef{
						Name: "role-allow-all",
					},
					Subjects: []Subject{
						{
							Kind: "Group",
							Name: "group-deny-delete",
						},
					},
				},
			},
			{
				ObjectMeta: hz.ObjectMeta{
					Name:    "rolebinding-deny-delete",
					Account: "account-test",
				},
				Spec: RoleBindingSpec{
					RoleRef: RoleRef{
						Name: "role-deny-delete",
					},
					Subjects: []Subject{
						{
							Kind: "Group",
							Name: "group-deny-delete",
						},
					},
				},
			},
		},
		cases: []testcase{
			{
				req: RBACRequest{
					Groups: []string{"group-deny-delete"},
					Verb:   "run",
					Object: hz.ObjectKey{
						Name:    "superfluous",
						Account: "account-test",
						Kind:    "object-test",
						Group:   "group-test",
					},
				},
				expect: true,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-deny-delete"},
					Verb:   "create",
					Object: hz.ObjectKey{
						Name:    "superfluous",
						Account: "account-test",
						Kind:    "object-test",
						Group:   "group-test",
					},
				},
				expect: true,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-deny-delete"},
					Verb:   "delete",
					Object: hz.ObjectKey{
						Name:    "superfluous",
						Account: "account-test",
						Kind:    "object-test",
					},
				},
				expect: false,
			},
		},
	}

	tests := []test{
		testCreateAllowsRead,
		testAllowRun,
		testDenyDelete,
	}
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
				ok := rbac.Check(ctx, tc.req)
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
		Key:       hz.KeyFromObject(obj),
	}
}

func TestCheckStringPatter(t *testing.T) {
	type test struct {
		pattern string
		value   string
		expect  bool
	}
	tests := []test{
		{
			pattern: "foo",
			value:   "foo",
			expect:  true,
		},
		{
			pattern: "foo*",
			value:   "foobar",
			expect:  true,
		},
		{
			pattern: "foo*",
			value:   "foo",
			expect:  true,
		},
		{
			pattern: "foo*",
			value:   "foo",
			expect:  true,
		},
		{
			pattern: "foo*",
			value:   "fo",
			expect:  false,
		},
		{
			// Pattern does not end with a "*" therefore it treats it as an
			// exact match.
			pattern: "foo*zoo",
			value:   "foobar",
			expect:  false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.pattern+"->"+tc.value, func(t *testing.T) {
			ok := checkStringPattern(&tc.pattern, tc.value)
			if ok != tc.expect {
				t.Fatal("test case failed")
			}
		})
	}
}
