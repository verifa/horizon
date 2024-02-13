package accounts

import (
	"context"
	"testing"

	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/testserver"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestUserReconciler(t *testing.T) {
	ctx := context.Background()

	ti := testserver.New(t, ctx, nil)

	userCtlr, err := hz.StartController(
		ctx,
		ti.Conn,
		hz.WithControllerReconciler(&UserReconciler{}),
		hz.WithControllerFor(User{}),
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
		hz.WithControllerFor(Group{}),
		hz.WithControllerValidatorCUE(),
	)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		groupCtlr.Stop()
	})

	// Create a group.
	groupClient := hz.ObjectClient[Group]{Client: hz.Client{Conn: ti.Conn}}
	group := Group{
		ObjectMeta: hz.ObjectMeta{
			Name:    "group1",
			Account: "test",
		},
		Spec: GroupSpec{},
	}
	err = groupClient.Apply(ctx, group, hz.WithApplyManager("test"))
	tu.AssertNoError(t, err)

	// Create a user with membership to that group.
	userClient := hz.ObjectClient[User]{Client: hz.Client{Conn: ti.Conn}}
	user := User{
		ObjectMeta: hz.ObjectMeta{
			Name:    "user1",
			Account: hz.RootAccount,
		},
		Spec: UserSpec{
			Claims: &UserClaims{
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
