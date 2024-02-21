package accounts_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestAccount(t *testing.T) {
	ctx := context.Background()
	ti := server.Test(t, ctx)

	client := hz.InternalClient(ti.Conn)
	// recon := accounts.AccountReconciler{
	// 	Client:            client,
	// 	Conn:              ti.Conn,
	// 	OpKeyPair:         ti.NS.Auth.Operator.SigningKey.KeyPair,
	// 	RootAccountPubKey: ti.NS.Auth.RootAccount.PublicKey,
	// }
	// ctlr, err := hz.StartController(
	// 	ctx,
	// 	ti.Conn,
	// 	hz.WithControllerReconciler(&recon),
	// 	hz.WithControllerFor(&accounts.Account{}),
	// )
	// tu.AssertNoError(t, err)
	// defer ctlr.Stop()

	account := accounts.Account{
		ObjectMeta: hz.ObjectMeta{
			Account: hz.RootAccount,
			Name:    "test",
		},
		Spec: accounts.AccountSpec{},
	}
	accClient := hz.ObjectClient[accounts.Account]{Client: client}
	err := accClient.Create(ctx, account)
	tu.AssertNoError(t, err)

	// Create a timeout and a done channel.
	// Watch until the replicaset is ready.
	// If the timeout is reached, fail the test.
	timeout := time.After(time.Second * 5)
	done := make(chan struct{})
	watcher, err := hz.StartWatcher(
		ctx,
		ti.Conn,
		hz.WithWatcherForObject(account),
		hz.WithWatcherFn(
			func(event hz.Event) (hz.Result, error) {
				var acc accounts.Account
				if err := json.Unmarshal(event.Data, &acc); err != nil {
					return hz.Result{}, fmt.Errorf(
						"unmarshalling account: %w",
						err,
					)
				}
				if acc.Status.Ready == true {
					close(done)
				}
				return hz.Result{}, nil
			},
		),
	)
	tu.AssertNoError(t, err)
	defer watcher.Close()

	select {
	case <-timeout:
		t.Errorf("timed out waiting for account")
	case <-done:
	}
}
