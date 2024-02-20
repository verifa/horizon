package auth_test

import (
	"context"
	"testing"

	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestAuth(t *testing.T) {
	ctx := context.Background()
	ts := server.Test(t, ctx)
	client := hz.Client{Conn: ts.Conn}

	// Create a user
	user := accounts.User{
		ObjectMeta: hz.ObjectMeta{
			Name:    "testuser",
			Account: "test",
		},
		Spec: accounts.UserSpec{
			Claims: &accounts.UserClaims{
				Sub:    hz.P("testuser"),
				Iss:    hz.P("test"),
				Name:   hz.P("Test User"),
				Email:  hz.P("testemail"),
				Groups: []string{"test"},
			},
		},
	}

	err := client.Apply(
		ctx,
		hz.WithApplyObject(user),
		hz.WithApplyManager("test"),
	)
	tu.AssertNoError(t, err)

	// Create a group
	group := accounts.Group{
		ObjectMeta: hz.ObjectMeta{
			Name:    "test",
			Account: hz.RootAccount,
		},
		Spec: accounts.GroupSpec{
			Accounts: map[string]accounts.GroupAccount{
				"test": {
					Relations: map[string]accounts.GroupAccountRelation{
						"viewer": {},
					},
				},
			},
		},
	}
	err = client.Apply(
		ctx,
		hz.WithApplyObject(group),
		hz.WithApplyManager("test"),
	)
	tu.AssertNoError(t, err)

	// Need to start a controller for the objects.
	ctlr, err := hz.StartController(
		ctx,
		ts.Conn,
		hz.WithControllerFor(hz.EmptyObjectWithMeta{}),
	)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		_ = ctlr.Stop()
	})
	obj := hz.EmptyObjectWithMeta{
		ObjectMeta: hz.ObjectMeta{
			Name:    "test",
			Account: "test",
		},
	}
	err = client.Apply(
		ctx,
		hz.WithApplyObject(obj),
		hz.WithApplyManager("test"),
	)
	tu.AssertNoError(t, err)

	// Now test if the user can read object from the account.
	authz := auth.Auth{
		Conn: ts.Conn,
	}
	err = authz.Start(ctx)
	tu.AssertNoError(t, err)

	// TODO: use the auth.Auth API methods...
	// ok, err := authz.Authorizer.server.Check(ctx, &openfgav1.CheckRequest{
	// 	StoreId: authz.Authorizer.storeID,
	// 	TupleKey: &openfgav1.CheckRequestTupleKey{
	// 		User:     "user:" + user.Name,
	// 		Relation: "can_read",
	// 		Object:   "object:" + objecterID(obj),
	// 	},
	// })
	// tu.AssertNoError(t, err)
	// tu.AssertEqual(t, true, ok.Allowed)
}
