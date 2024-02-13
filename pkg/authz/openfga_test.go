package authz

import (
	"context"
	"testing"

	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/testserver"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestAuthz(t *testing.T) {
	ctx := context.Background()
	ti := testserver.New(t, ctx, nil)
	client := hz.Client{Conn: ti.Conn}
	userClient := hz.ObjectClient[accounts.User]{
		Client: client,
	}

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

	err := userClient.Apply(ctx, user, hz.WithApplyManager("test"))
	tu.AssertNoError(t, err)
}
