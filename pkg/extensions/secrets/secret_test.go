package secrets_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/verifa/horizon/pkg/extensions/secrets"
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
		hz.WithControllerFor(secrets.Secret{}),
	)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		_ = ctlr.Stop()
	})

	secret := secrets.Secret{
		ObjectMeta: hz.ObjectMeta{
			Name:    "my-secret",
			Account: "my-account",
		},
		Data: secrets.SecretData{
			"username": "admin",
			"password": "password",
		},
	}
	client := hz.NewClient(
		ts.Conn,
		hz.WithClientInternal(true),
		hz.WithClientDefaultManager(),
	)
	_, err = client.Apply(ctx, hz.WithApplyObject(secret))
	tu.AssertNoError(t, err)

	raw, err := client.Get(ctx, hz.WithGetKey(secret))
	tu.AssertNoError(t, err)

	getSecret := secrets.Secret{}
	err = json.Unmarshal(raw, &getSecret)
	tu.AssertNoError(t, err)

	tu.AssertEqual(t, secret.Data, getSecret.Data)
}
