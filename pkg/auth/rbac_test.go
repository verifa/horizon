package auth

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/testutil"
)

func TestRBAC(t *testing.T) {
	ctx := context.Background()
	type testcase struct {
		req    RBACRequest
		expect bool
	}

	type test struct {
		name        string
		adminGroups []string
		roles       []Role
		bindings    []RoleBinding

		cases []testcase
	}

	testCreateAllowsRead := test{
		name: "allow-create-allows-read",
		roles: []Role{
			{
				ObjectMeta: hz.ObjectMeta{
					Name:      "role-creator",
					Namespace: "namespace-test",
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
					Name:      "rolebinding-test",
					Namespace: "namespace-test",
				},
				Spec: RoleBindingSpec{
					RoleRef: RoleRef{
						Group: "core",
						Kind:  "Role",
						Name:  "role-creator",
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
						Group:     "group-test",
						Kind:      "Namespace",
						Namespace: hz.RootNamespace,
						Name:      "namespace-test",
					},
				},
				expect: true,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-creator"},
					Verb:   "read",
					Object: hz.ObjectKey{
						Group:     "group-test",
						Kind:      "Namespace",
						Namespace: hz.RootNamespace,
						Name:      "namespace-another",
					},
				},
				expect: false,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-creator"},
					Verb:   "read",
					Object: hz.ObjectKey{
						Group:     "group-test",
						Kind:      "object-test",
						Namespace: "namespace-test",
						Name:      "superfluous",
					},
				},
				expect: true,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-creator"},
					Verb:   "create",
					Object: hz.ObjectKey{
						Group:     "group-test",
						Kind:      "object-test",
						Namespace: "namespace-test",
						Name:      "superfluous",
					},
				},
				expect: true,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-creator"},
					Verb:   "delete",
					Object: hz.ObjectKey{
						Group:     "group-test",
						Kind:      "object-test",
						Namespace: "namespace-test",
						Name:      "superfluous",
					},
				},
				expect: false,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-unknown"},
					Verb:   "read",
					Object: hz.ObjectKey{
						Group:     "group-test",
						Kind:      "object-test",
						Namespace: "namespace-test",
						Name:      "superfluous",
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
					Name:      "role-runner",
					Namespace: "namespace-test",
				},
				Spec: RoleSpec{
					Allow: []Verbs{
						{
							Run: &VerbFilter{
								Group: hz.P("group-test"),
								Kind:  hz.P("object-test"),
							},
						},
					},
				},
			},
		},
		bindings: []RoleBinding{
			{
				ObjectMeta: hz.ObjectMeta{
					Name:      "rolebinding-test",
					Namespace: "namespace-test",
				},
				Spec: RoleBindingSpec{
					RoleRef: RoleRef{
						Group: "core",
						Kind:  "Role",
						Name:  "role-runner",
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
						Group:     "group-test",
						Kind:      "object-test",
						Namespace: "namespace-test",
						Name:      "superfluous",
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
					Name:      "role-allow-all",
					Namespace: "namespace-test",
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
					Name:      "role-deny-delete",
					Namespace: "namespace-test",
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
					Name:      "rolebinding-allow-all",
					Namespace: "namespace-test",
				},
				Spec: RoleBindingSpec{
					RoleRef: RoleRef{
						Group: "core",
						Kind:  "Role",
						Name:  "role-allow-all",
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
					Name:      "rolebinding-deny-delete",
					Namespace: "namespace-test",
				},
				Spec: RoleBindingSpec{
					RoleRef: RoleRef{
						Group: "core",
						Kind:  "Role",
						Name:  "role-deny-delete",
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
						Group:     "group-test",
						Kind:      "object-test",
						Namespace: "namespace-test",
						Name:      "superfluous",
					},
				},
				expect: true,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-deny-delete"},
					Verb:   "create",
					Object: hz.ObjectKey{
						Group:     "group-test",
						Kind:      "object-test",
						Namespace: "namespace-test",
						Name:      "superfluous",
					},
				},
				expect: true,
			},
			{
				req: RBACRequest{
					Groups: []string{"group-deny-delete"},
					Verb:   "delete",
					Object: hz.ObjectKey{
						Name:      "superfluous",
						Namespace: "namespace-test",
						Kind:      "object-test",
					},
				},
				expect: false,
			},
		},
	}

	testAdminGroup := test{
		name:        "admin-group",
		adminGroups: []string{"admin"},
		cases: []testcase{
			{
				req: RBACRequest{
					Groups: []string{"admin"},
					Verb:   "delete",
					Object: hz.ObjectKey{
						Group:     "group-test",
						Kind:      "object-test",
						Namespace: "whatever-namespace-doesnt-matter",
						Name:      "superfluous",
					},
				},
				expect: true,
			},
		},
	}

	tests := []test{
		testAdminGroup,
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
				AdminGroups:  test.adminGroups,
			}
			for _, role := range test.roles {
				_, err := rbac.HandleRoleEvent(event(t, role))
				testutil.AssertNoError(t, err)
			}
			for _, binding := range test.bindings {
				_, err := rbac.HandleRoleBindingEvent(event(t, binding))
				testutil.AssertNoError(t, err)
			}
			// Call refresh in case of no roles or rolebindings.
			rbac.refresh()
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
		Key:       obj,
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
			pattern := tc.pattern
			ok := checkStringPattern(&pattern, tc.value)
			if ok != tc.expect {
				t.Fatal("test case failed")
			}
		})
	}
}
