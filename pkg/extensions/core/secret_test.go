package core_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/verifa/horizon/pkg/extensions/core"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestSecrets(t *testing.T) {
	ctx := context.Background()
	ts := server.Test(t, ctx)

	ctlr, err := hz.StartController(
		ctx,
		ts.Conn,
		hz.WithControllerFor(core.Secret{}),
	)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		_ = ctlr.Stop()
	})

	secret := core.Secret{
		ObjectMeta: hz.ObjectMeta{
			Name:      "my-secret",
			Namespace: "test",
		},
		Data: core.SecretData{
			"username": "admin",
			"password": "password",
		},
	}
	client := hz.NewClient(
		ts.Conn,
		hz.WithClientInternal(true),
	)
	_, err = client.Apply(ctx, hz.WithApplyObject(secret))
	tu.AssertNoError(t, err)

	raw, err := client.Get(ctx, hz.WithGetKey(secret))
	tu.AssertNoError(t, err)

	getSecret := core.Secret{}
	err = json.Unmarshal(raw, &getSecret)
	tu.AssertNoError(t, err)

	tu.AssertEqual(t, secret.Data, getSecret.Data)
}
