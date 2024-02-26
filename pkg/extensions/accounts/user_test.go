package accounts_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestUser(t *testing.T) {
	ctx := context.Background()
	ti := server.Test(t, ctx)
	client := hz.NewClient(ti.Conn, hz.WithClientInternal(true))
	createAction := accounts.UserCreateAction{
		Client: client,
	}

	actor, err := hz.StartActor(
		ctx,
		ti.Conn,
		hz.WithActorActioner(&createAction),
	)
	tu.AssertNoError(t, err)
	defer actor.Stop()

	// In order to publish a user, the account the user references
	// must exist in the NATS KV store.
	recon := accounts.AccountReconciler{
		Client:            client,
		Conn:              ti.Conn,
		OpKeyPair:         ti.NS.Auth.Operator.SigningKey.KeyPair,
		RootAccountPubKey: ti.NS.Auth.RootAccount.PublicKey,
	}
	ctlr, err := hz.StartController(
		ctx,
		ti.Conn,
		hz.WithControllerReconciler(&recon),
		hz.WithControllerFor(accounts.Account{}),
	)
	tu.AssertNoError(t, err)
	defer ctlr.Stop()

	account := accounts.Account{
		ObjectMeta: hz.ObjectMeta{
			Account: hz.RootAccount,
			Name:    "test",
		},
		Spec: &accounts.AccountSpec{},
	}
	accstatus, err := recon.CreateAccount(account.Name)
	tu.AssertNoError(t, err)
	account.Status = accstatus
	accClient := hz.ObjectClient[accounts.Account]{Client: client}
	err = accClient.Create(ctx, account)
	tu.AssertNoError(t, err)

	user := accounts.User{
		ObjectMeta: hz.ObjectMeta{
			Account: "test",
			Name:    "test",
		},
	}
	userClient := hz.ObjectClient[accounts.User]{Client: client}
	reply, err := userClient.Run(ctx, &createAction, user)
	tu.AssertNoError(t, err)
	// Give the NATS server a minute to process the account we just created.
	time.Sleep(time.Millisecond * 100)
	// Test logging in.
	userNC, err := nats.Connect(
		ti.NS.NS.ClientURL(),
		nats.UserJWTAndSeed(reply.Status.JWT, reply.Status.Seed),
	)
	tu.AssertNoError(t, err)
	defer userNC.Close()
	_, err = userNC.GetClientID()
	tu.AssertNoError(t, err)
}
