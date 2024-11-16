package auth_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/extensions/core"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	"github.com/verifa/horizon/pkg/testutil"
)

func TestRBACServer(t *testing.T) {
	ctx := context.Background()
	ts := server.Test(t, ctx)

	// Create roles and rolebindings for team-a and team-b.
	// team-a has access to all resources in the "team-a" namespace.
	// team-b has access to all resources in the "team-b" namespace.
	teamANS := core.Namespace{
		ObjectMeta: hz.ObjectMeta{
			Name:      "team-a",
			Namespace: hz.RootNamespace,
		},
	}
	teamBNS := core.Namespace{
		ObjectMeta: hz.ObjectMeta{
			Name:      "team-b",
			Namespace: hz.RootNamespace,
		},
	}
	client := hz.NewClient(ts.Conn, hz.WithClientInternal(true))
	if _, err := client.Apply(ctx, hz.WithApplyObject(teamANS)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Apply(ctx, hz.WithApplyObject(teamBNS)); err != nil {
		t.Fatal(err)
	}

	roleA := auth.Role{
		ObjectMeta: hz.ObjectMeta{
			Name:      "team-a",
			Namespace: "team-a",
		},
		Spec: auth.RoleSpec{
			Allow: []auth.Verbs{
				{
					Create: &auth.VerbFilter{
						Group: hz.P("*"),
						Kind:  hz.P("*"),
						Name:  hz.P("*"),
					},
				},
			},
		},
	}
	roleBindingA := auth.RoleBinding{
		ObjectMeta: hz.ObjectMeta{
			Name:      "team-a",
			Namespace: "team-a",
		},
		Spec: auth.RoleBindingSpec{
			RoleRef: auth.RoleRef{
				Group: roleA.ObjectGroup(),
				Kind:  roleA.ObjectKind(),
				Name:  roleA.ObjectMeta.Name,
			},
			Subjects: []auth.Subject{
				{
					Kind: "Group",
					Name: "team-a",
				},
			},
		},
	}

	if _, err := client.Apply(ctx, hz.WithApplyObject(roleA)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Apply(ctx, hz.WithApplyObject(roleBindingA)); err != nil {
		t.Fatal(err)
	}

	roleB := auth.Role{
		ObjectMeta: hz.ObjectMeta{
			Name:      "team-b",
			Namespace: "team-b",
		},
		Spec: auth.RoleSpec{
			Allow: []auth.Verbs{
				{
					Create: &auth.VerbFilter{
						Group: hz.P("*"),
						Kind:  hz.P("*"),
						Name:  hz.P("*"),
					},
				},
			},
		},
	}
	roleBindingB := auth.RoleBinding{
		ObjectMeta: hz.ObjectMeta{
			Name:      "team-b",
			Namespace: "team-b",
		},
		Spec: auth.RoleBindingSpec{
			RoleRef: auth.RoleRef{
				Group: roleB.ObjectGroup(),
				Kind:  roleB.ObjectKind(),
				Name:  roleB.ObjectMeta.Name,
			},
			Subjects: []auth.Subject{
				{
					Kind: "Group",
					Name: "team-b",
				},
			},
		},
	}
	if _, err := client.Apply(ctx, hz.WithApplyObject(roleB)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Apply(ctx, hz.WithApplyObject(roleBindingB)); err != nil {
		t.Fatal(err)
	}

	userTeamA := auth.UserInfo{
		Sub:    "team-a",
		Iss:    "horizon",
		Groups: []string{"team-a"},
	}
	userASess, err := ts.Auth.Sessions.New(ctx, userTeamA)
	if err != nil {
		t.Fatal(err)
	}

	userTeamB := auth.UserInfo{
		Sub:    "team-b",
		Iss:    "horizon",
		Groups: []string{"team-b"},
	}
	userBSess, err := ts.Auth.Sessions.New(ctx, userTeamB)
	if err != nil {
		t.Fatal(err)
	}

	// Test team-a access.
	// We can create just a role as a test.
	testRoleA := auth.Role{
		ObjectMeta: hz.ObjectMeta{
			Name:      "test-role-a",
			Namespace: "team-a",
		},
		Spec: auth.RoleSpec{},
	}
	userAClient := hz.NewClient(ts.Conn, hz.WithClientSession(userASess))
	if _, err := userAClient.Apply(ctx, hz.WithApplyObject(testRoleA)); err != nil {
		t.Fatal(err)
	}

	testRoleB := auth.Role{
		ObjectMeta: hz.ObjectMeta{
			Name:      "test-role-b",
			Namespace: "team-b",
		},
		Spec: auth.RoleSpec{},
	}
	userBClient := hz.NewClient(ts.Conn, hz.WithClientSession(userBSess))
	if _, err := userBClient.Apply(ctx, hz.WithApplyObject(testRoleB)); err != nil {
		t.Fatal(err)
	}

	{
		// Try user A to create test-role-b.
		_, err := userAClient.Apply(ctx, hz.WithApplyObject(testRoleB))
		expErr := &hz.Error{
			Status:  http.StatusForbidden,
			Message: "forbidden",
		}
		testutil.AssertEqual(t, expErr, err)
	}
	{
		// Try user B to create test-role-a.
		_, err := userBClient.Apply(ctx, hz.WithApplyObject(testRoleA))
		expErr := &hz.Error{
			Status:  http.StatusForbidden,
			Message: "forbidden",
		}
		testutil.AssertEqual(t, expErr, err)
	}
}
