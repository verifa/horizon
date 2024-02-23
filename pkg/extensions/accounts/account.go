package accounts

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/natsutil"
)

const (
	ObjectKind  = "Account"
	ObjectGroup = "hz-internal"
)

type Account struct {
	hz.ObjectMeta `json:"metadata,omitempty" cue:""`

	Spec   AccountSpec   `json:"spec"`
	Status AccountStatus `json:"status"`
}

func (a Account) ObjectAPIVersion() string {
	return "v1"
}

func (a Account) ObjectGroup() string {
	return ObjectGroup
}

func (a Account) ObjectKind() string {
	return ObjectKind
}

type AccountSpec struct{}

type AccountStatus struct {
	Ready bool `json:"ready"`
	// ID of the account, which for NATS is the public key of the account
	// and the subject of the account's JWT.
	ID string `json:"id,omitempty"`
	// Seed of the account.
	// The "seed" can be converted into the account public
	// and private keys.
	Seed string `json:"seed,omitempty"`
	// SigningKeySeed is the seed of the account signing key.
	// The account signing key should be used for signing all the JWTs for the
	// account.
	SigningKeySeed string             `json:"signing_key_seed,omitempty"`
	Claims         *jwt.AccountClaims `json:"claims,omitempty" cue:",opt"`
}

var _ (hz.Reconciler) = (*AccountReconciler)(nil)

type AccountReconciler struct {
	hz.Client
	Conn *nats.Conn

	OpKeyPair         nkeys.KeyPair
	RootAccountPubKey string
}

// Reconcile implements hz.Reconciler.
func (r *AccountReconciler) Reconcile(
	ctx context.Context,
	req hz.Request,
) (hz.Result, error) {
	accClient := hz.ObjectClient[Account]{Client: r.Client}
	account, err := accClient.Get(ctx, hz.WithGetObjectKey(req.Key))
	if err != nil {
		return hz.Result{}, hz.IgnoreNotFound(err)
	}

	if account.Status.ID == "" {
		// If the ID is empty, we need to create the account.
		status, err := r.CreateAccount(account.Name)
		if err != nil {
			return hz.Result{}, fmt.Errorf("creating account spec: %w", err)
		}
		if _, err := AccountClaimsUpdate(ctx, r.Conn, r.OpKeyPair, status.Claims); err != nil {
			return hz.Result{}, fmt.Errorf("updating account: %w", err)
		}
		account.Status = *status
		account.Status.Ready = true
		// Save the account and trigger a requeue to publish the account in
		// nats.
		if err := accClient.Apply(ctx, *account); err != nil {
			return hz.Result{}, fmt.Errorf("updating account: %w", err)
		}
		return hz.Result{}, nil
	}

	ready := true
	claims, err := AccountClaimsLookup(ctx, r.Conn, account.Status.ID)
	if err != nil {
		if !errors.Is(err, ErrAccountNotFound) {
			return hz.Result{}, fmt.Errorf("looking up account: %w", err)
		}
		// If the account is not found, we need to create it.
		if _, err := AccountClaimsUpdate(ctx, r.Conn, r.OpKeyPair, account.Status.Claims); err != nil {
			return hz.Result{}, fmt.Errorf("updating account: %w", err)
		}
		ready = false
	}

	if ready && !cmp.Equal(claims, account.Status.Claims) {
		if _, err := AccountClaimsUpdate(ctx, r.Conn, r.OpKeyPair, account.Status.Claims); err != nil {
			return hz.Result{}, fmt.Errorf("updating account: %w", err)
		}
		ready = false
	}

	if !ready {
		if account.Status.Ready {
			account.Status.Ready = false
			if err := accClient.Apply(ctx, *account); err != nil {
				return hz.Result{}, fmt.Errorf("updating account: %w", err)
			}
			return hz.Result{}, nil
		}
		return hz.Result{}, nil
	}

	if !account.Status.Ready {
		account.Status.Ready = true
		if err := accClient.Apply(ctx, *account); err != nil {
			return hz.Result{}, fmt.Errorf("updating account: %w", err)
		}
	}

	return hz.Result{}, nil
}

func (r *AccountReconciler) CreateAccount(
	name string,
) (*AccountStatus, error) {
	accNKey, err := natsutil.NewAccountNKey()
	if err != nil {
		return nil, fmt.Errorf("new account nkey: %w", err)
	}

	accSK, err := natsutil.NewAccountNKey()
	if err != nil {
		return nil, fmt.Errorf("new account signing key: %w", err)
	}

	claims := jwt.NewAccountClaims(accNKey.PublicKey)
	claims.Name = name
	claims.SigningKeys.Add(accSK.PublicKey)
	claims.Limits.JetStreamLimits.Consumer = -1
	claims.Limits.JetStreamLimits.DiskMaxStreamBytes = -1
	claims.Limits.JetStreamLimits.DiskStorage = -1
	claims.Limits.JetStreamLimits.MaxAckPending = -1
	claims.Limits.JetStreamLimits.MemoryMaxStreamBytes = -1
	claims.Limits.JetStreamLimits.MemoryStorage = -1
	claims.Limits.JetStreamLimits.Streams = -1
	// claims.Imports.Add(&jwt.Import{
	// 	Type: jwt.Service,
	// 	Name: "all-actors",
	// 	// Account is the public key of the account which exported the service.
	// 	Account: r.NCPAccountPubKey,
	// 	// Subject is the exported account's subject.
	// 	Subject: jwt.Subject(
	// 		fmt.Sprintf(hz.ActionImportSubject, accNKey.PublicKey),
	// 	),
	// 	// LocalSubject is the subject local to this account.
	// 	LocalSubject: jwt.RenamingSubject(hz.ActionImportLocalSubject),
	// })
	// Export the Jetstream API for this account, which we will import into
	// the actor account, making this account's Jetstream API available to
	// connections from the actor account.
	claims.Exports.Add(&jwt.Export{
		Type:    jwt.Service,
		Name:    "js-api",
		Subject: jwt.Subject("$JS.API.>"),
	})

	// To fully populate the claims, we need to encode them into a JWT.
	// Then we can decode the JWT and get the "full" claims, so there
	// won't be a drift with the NATS servers.
	accJWT, err := claims.Encode(r.OpKeyPair)
	if err != nil {
		return nil, fmt.Errorf("encoding claims: %w", err)
	}
	accClaims, err := jwt.DecodeAccountClaims(accJWT)
	if err != nil {
		return nil, fmt.Errorf("decoding claims: %w", err)
	}

	spec := AccountStatus{
		ID:             accNKey.PublicKey,
		Seed:           accNKey.Seed,
		SigningKeySeed: accSK.Seed,
		Claims:         accClaims,
	}
	return &spec, nil
}
