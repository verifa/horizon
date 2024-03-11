package serviceaccounts_test

import (
	"context"
	"testing"
	"time"

	"github.com/verifa/horizon/pkg/extensions/secrets"
	"github.com/verifa/horizon/pkg/extensions/serviceaccounts"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/hztest"
	"github.com/verifa/horizon/pkg/server"
)

func TestServiceAccounts(t *testing.T) {
	ctx := context.Background()
	ts := server.Test(t, ctx)

	svcAcc := serviceaccounts.ServiceAccount{
		ObjectMeta: hz.ObjectMeta{
			Account: hz.RootAccount,
			Name:    "test-serviceaccount",
		},
		Spec: &serviceaccounts.ServiceAccountSpec{},
	}

	client := hz.NewClient(
		ts.Conn,
		hz.WithClientDefaultManager(),
		hz.WithClientInternal(true),
	)

	if err := client.Apply(ctx, hz.WithApplyObject(svcAcc)); err != nil {
		t.Fatalf("apply service account: %v", err)
	}

	hztest.WatchWaitUntil(
		t,
		ctx,
		ts.Conn,
		time.Second*5,
		svcAcc,
		func(sa serviceaccounts.ServiceAccount) bool {
			return sa.Status != nil && sa.Status.Ready
		},
	)

	expSecret := secrets.Secret{
		ObjectMeta: hz.ObjectMeta{
			Account: hz.RootAccount,
			Name:    "test-serviceaccount",
		},
	}
	hztest.WatchWaitUntil(
		t,
		ctx,
		ts.Conn,
		time.Second*5,
		expSecret,
		func(s secrets.Secret) bool {
			return s.Data != nil
		},
	)
}
