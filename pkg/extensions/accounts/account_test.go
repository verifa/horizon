package accounts_test

import (
	"context"
	"testing"
	"time"

	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/hztest"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestAccount(t *testing.T) {
	ctx := context.Background()
	ti := server.Test(t, ctx)

	client := hz.NewClient(
		ti.Conn,
		hz.WithClientInternal(true),
		hz.WithClientManager("test"),
	)

	account := accounts.Account{
		ObjectMeta: hz.ObjectMeta{
			Account: hz.RootAccount,
			Name:    "test",
		},
		Spec: &accounts.AccountSpec{},
	}
	accClient := hz.ObjectClient[accounts.Account]{Client: client}
	_, err := accClient.Apply(ctx, account)
	tu.AssertNoError(t, err)

	hztest.WatchWaitUntil(
		t,
		ctx,
		ti.Conn,
		time.Second*5,
		account,
		func(acc accounts.Account) bool {
			return acc.Status != nil && acc.Status.Ready
		},
	)
}
