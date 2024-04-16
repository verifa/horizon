package accounts

import (
	"context"
	"fmt"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/natsutil"
)

var _ (hz.Objecter) = (*User)(nil)

// User represents a NATS user.
type User struct {
	hz.ObjectMeta `json:"metadata"`

	Spec   UserSpec   `json:"spec"`
	Status UserStatus `json:"status"`
}

func (u User) ObjectVersion() string {
	return "v1"
}

func (u User) ObjectGroup() string {
	return "core"
}

func (u User) ObjectKind() string {
	return "User"
}

type UserSpec struct {
	Claims *UserClaims `json:"claims,omitempty" cue:""`
}

type UserClaims struct {
	Sub     *string  `json:"sub,omitempty" cue:""`
	Iss     *string  `json:"iss,omitempty" cue:""`
	Name    *string  `json:"name,omitempty" cue:""`
	Email   *string  `json:"email,omitempty" cue:""`
	Groups  []string `json:"groups,omitempty"`
	Picture *string  `json:"picture,omitempty"`
}

type UserStatus struct {
	// ID of the user, which for NATS is the public key.
	ID string `json:"id"`
	// Seed of the user.
	// The Seed (or "seed") can be converted into the user public
	// and private keys. The public key must match the user ID.
	Seed string `json:"nkey"`
	// JWT of the user.
	// The JWT contains the user claims (i.e. name, config, limits, etc.)
	// and is signed using an account NKey.
	JWT string `json:"jwt"`
}

var _ (hz.Action[User]) = (*UserCreateAction)(nil)

type UserCreateAction struct {
	hz.Client
}

// Action implements hz.Action.
func (a *UserCreateAction) Action() string {
	return "create"
}

// Do implements hz.Action.
func (a *UserCreateAction) Do(ctx context.Context, user User) (User, error) {
	if err := a.userCreate(ctx, &user); err != nil {
		return User{}, fmt.Errorf("creating user claims and keys: %w", err)
	}
	// TODO: do we want to store users? If so, need to consider that some users
	// are created via the UI, and some are created via the ncpctl auth login
	// command.
	// userClient := hz.ObjectClient[User]{Client: a.Client}
	// if _, err := userClient.Create(ctx, user); err != nil {
	// 	return User{}, fmt.Errorf("storing user in nats store: %w", err)
	// }

	return user, nil
}

func (a *UserCreateAction) userCreate(ctx context.Context, user *User) error {
	accClient := hz.ObjectClient[Account]{Client: a.Client}
	account, err := accClient.Get(
		ctx,
		hz.WithGetKey(hz.ObjectKey{
			Name:    hz.RootAccount,
			Account: hz.RootAccount,
		}),
	)
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}
	signingKey, err := nkeys.FromSeed([]byte(account.Status.SigningKeySeed))
	if err != nil {
		return fmt.Errorf("get account key pair: %w", err)
	}
	userNKey, err := natsutil.NewUserNKey()
	if err != nil {
		return fmt.Errorf("new user nkey: %w", err)
	}
	claims := jwt.NewUserClaims(userNKey.PublicKey)
	claims.Name = user.Name
	claims.IssuerAccount = account.Status.ID
	if err := validateClaims(claims); err != nil {
		return fmt.Errorf("validate claims: %w", err)
	}
	jwt, err := claims.Encode(signingKey)
	if err != nil {
		return fmt.Errorf("encode claims: %w", err)
	}
	user.Status.ID = userNKey.PublicKey
	user.Status.Seed = userNKey.Seed
	user.Status.JWT = jwt
	return nil
}
