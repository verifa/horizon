package serviceaccounts

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/extensions/secrets"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/natsutil"
)

const FieldManager = "ctlr-serviceaccounts"

var _ hz.Reconciler = (*Reconciler)(nil)

type Reconciler struct {
	Client hz.Client
}

func (r Reconciler) Reconcile(
	ctx context.Context,
	req hz.Request,
) (hz.Result, error) {
	saClient := hz.ObjectClient[ServiceAccount]{Client: r.Client}
	secretClient := hz.ObjectClient[secrets.Secret]{Client: r.Client}

	sa, err := saClient.Get(ctx, hz.WithGetKey(req.Key))
	if err != nil {
		return hz.Result{}, hz.IgnoreNotFound(err)
	}

	saApply, err := hz.ExtractManagedFields(sa, FieldManager)
	if err != nil {
		return hz.Result{}, fmt.Errorf("extracting managed fields: %w", err)
	}

	if sa.DeletionTimestamp.IsPast() {
		// Nothing to do, the GC will delete child secrets.
		return hz.Result{}, nil
	}

	saSecret, err := secretClient.Get(
		ctx,
		hz.WithGetKey(
			secrets.Secret{
				ObjectMeta: hz.ObjectMeta{
					Account: sa.Account,
					Name:    sa.Name,
				},
			},
		),
	)
	if hz.IgnoreNotFound(err) != nil {
		return hz.Result{}, fmt.Errorf("getting secret: %w", err)
	}
	if hz.IgnoreNotFound(err) == nil {
		// Existing secret does not exist, so create it.
		userCreds, err := r.generateNATSCredentials(ctx, sa)
		if err != nil {
			return hz.Result{}, fmt.Errorf(
				"generating nats credentials: %w",
				err,
			)
		}

		// Create the secret containing credentials.
		saSecret = secrets.Secret{
			ObjectMeta: hz.ObjectMeta{
				Account: sa.Account,
				Name:    sa.Name,
				// Set service account as an owner reference.
				// That means if the service account is deleted, the secret will
				// be deleted by the GC.
				OwnerReferences: []hz.OwnerReference{
					{
						Group:   sa.ObjectGroup(),
						Version: sa.ObjectVersion(),
						Kind:    sa.ObjectKind(),
						Account: sa.Account,
						Name:    sa.Name,
					},
				},
			},
			Data: secrets.SecretData{
				"nats.creds": userCreds,
			},
		}
		if _, err := secretClient.Apply(ctx, saSecret); err != nil {
			return hz.Result{}, fmt.Errorf("applying secret: %w", err)
		}
	}

	if sa.Status == nil {
		saApply.Status = &ServiceAccountStatus{
			Ready:                     true,
			NATSCredentialsSecretName: &saSecret.Name,
		}
		if _, err := saClient.Apply(ctx, saApply); err != nil {
			return hz.Result{}, fmt.Errorf("applying service account: %w", err)
		}
	}
	return hz.Result{}, nil
}

func (r Reconciler) generateNATSCredentials(
	ctx context.Context,
	sa ServiceAccount,
) (string, error) {
	accClient := hz.ObjectClient[accounts.Account]{Client: r.Client}
	account, err := accClient.Get(ctx, hz.WithGetKey(hz.ObjectKey{
		Account: hz.RootAccount,
		Name:    sa.Account,
	}))
	if err != nil {
		return "", fmt.Errorf("getting horizon account: %w", err)
	}
	if account.Status == nil {
		return "", fmt.Errorf("account status is nil")
	}

	userNKey, err := natsutil.NewUserNKey()
	if err != nil {
		return "", fmt.Errorf("new user nkey: %w", err)
	}
	signingKey, err := nkeys.FromSeed([]byte(account.Status.SigningKeySeed))
	if err != nil {
		return "", fmt.Errorf("getting account key pair: %w", err)
	}
	claims := jwt.NewUserClaims(userNKey.PublicKey)
	claims.Name = uuid.NewString()
	claims.IssuerAccount = account.Status.ID
	claims.Pub.Allow.Add(hz.SubjectAPIAllowAll)
	claims.Expires = time.Now().Add(time.Hour * 24).Unix()
	claims.Claims()
	userJWT, err := claims.Encode(signingKey)
	if err != nil {
		return "", fmt.Errorf("encoding user claims: %w", err)
	}
	userConfig, err := jwt.FormatUserConfig(
		userJWT,
		[]byte(userNKey.Seed),
	)
	if err != nil {
		return "", fmt.Errorf("formatting user config: %w", err)
	}

	return base64.StdEncoding.EncodeToString(userConfig), nil
}
