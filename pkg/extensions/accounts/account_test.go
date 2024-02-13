package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/testserver"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestAccount(t *testing.T) {
	ctx := context.Background()
	ti := testserver.New(t, ctx, nil)

	client := hz.Client{Conn: ti.Conn}
	recon := AccountReconciler{
		Client:            client,
		Conn:              ti.Conn,
		OpKeyPair:         ti.Auth.Operator.SigningKey.KeyPair,
		RootAccountPubKey: ti.Auth.RootAccount.PublicKey,
	}
	ctlr, err := hz.StartController(
		ctx,
		ti.Conn,
		hz.WithControllerReconciler(&recon),
		hz.WithControllerFor(&Account{}),
	)
	tu.AssertNoError(t, err)
	defer ctlr.Stop()

	account := Account{
		ObjectMeta: hz.ObjectMeta{
			Account: hz.RootAccount,
			Name:    "test",
		},
		Spec: AccountSpec{},
	}
	accClient := hz.ObjectClient[Account]{Client: client}
	err = accClient.Create(ctx, account)
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
			func(ctx context.Context, event hz.Event) (hz.Result, error) {
				var acc Account
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
