package accounts_test

import (
	"context"
	"testing"

	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestUserReconciler(t *testing.T) {
	ctx := context.Background()

	ti := server.Test(t, ctx)

	userCtlr, err := hz.StartController(
		ctx,
		ti.Conn,
		hz.WithControllerReconciler(&accounts.UserReconciler{}),
		hz.WithControllerFor(accounts.User{}),
		hz.WithControllerValidatorCUE(),
	)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		userCtlr.Stop()
	})

	groupCtlr, err := hz.StartController(
		ctx,
		ti.Conn,
		// hz.WithControllerReconciler(&GroupReconciler{}),
		hz.WithControllerFor(accounts.Group{}),
		hz.WithControllerValidatorCUE(),
	)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		groupCtlr.Stop()
	})

	// Create a group.
	groupClient := hz.ObjectClient[accounts.Group]{
		Client: hz.InternalClient(ti.Conn),
	}
	group := accounts.Group{
		ObjectMeta: hz.ObjectMeta{
			Name:    "group1",
			Account: "test",
		},
		Spec: accounts.GroupSpec{},
	}
	err = groupClient.Apply(ctx, group, hz.WithApplyManager("test"))
	tu.AssertNoError(t, err)

	// Create a user with membership to that group.
	userClient := hz.ObjectClient[accounts.User]{
		Client: hz.InternalClient(ti.Conn),
	}
	user := accounts.User{
		ObjectMeta: hz.ObjectMeta{
			Name:    "user1",
			Account: hz.RootAccount,
		},
		Spec: accounts.UserSpec{
			Claims: &accounts.UserClaims{
				Sub:    hz.P("user1"),
				Iss:    hz.P("test"),
				Name:   hz.P("User 1"),
				Email:  hz.P("user@localhost"),
				Groups: []string{"group1"},
			},
		},
	}
	err = userClient.Apply(ctx, user, hz.WithApplyManager("test"))
	tu.AssertNoError(t, err)
}
