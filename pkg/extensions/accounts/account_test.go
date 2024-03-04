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
	err := accClient.Apply(ctx, account)
	tu.AssertNoError(t, err)

	// Create a timeout and a done channel.
	// Watch until the replicaset is ready.
	// If the timeout is reached, fail the test.
	timeout := time.After(time.Second * 5)
	done := make(chan struct{})
	watcher, err := hz.StartWatcher(
		ctx,
		ti.Conn,
		hz.WithWatcherFor(account),
		hz.WithWatcherFn(
			func(event hz.Event) (hz.Result, error) {
				t.Log("watch event for account")
				var acc accounts.Account
				if err := json.Unmarshal(event.Data, &acc); err != nil {
					return hz.Result{}, fmt.Errorf(
						"unmarshalling account: %w",
						err,
					)
				}
				if acc.Status == nil {
					return hz.Result{}, nil
				}
				t.Log("account ready? ", acc.Status.Ready)
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
