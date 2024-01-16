package natsutil

import (
	"fmt"

	"github.com/nats-io/nkeys"
)

func NewOperatorNKey() (*NKey, error) {
	kp, err := nkeys.CreateOperator()
	if err != nil {
		return nil, fmt.Errorf("creating nkey: %w", err)
	}
	return newNKeyFromKeyPair(kp)
}

func NewAccountNKey() (*NKey, error) {
	kp, err := nkeys.CreateAccount()
	if err != nil {
		return nil, fmt.Errorf("creating nkey: %w", err)
	}
	return newNKeyFromKeyPair(kp)
}

func NewUserNKey() (*NKey, error) {
	kp, err := nkeys.CreateUser()
	if err != nil {
		return nil, fmt.Errorf("creating nkey: %w", err)
	}
	return newNKeyFromKeyPair(kp)
}

func newNKeyFromKeyPair(kp nkeys.KeyPair) (*NKey, error) {
	seed, err := kp.Seed()
	if err != nil {
		return nil, fmt.Errorf("getting seed: %w", err)
	}
	pk, err := kp.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("getting public key: %w", err)
	}
	return &NKey{
		Seed:      string(seed),
		KeyPair:   kp,
		PublicKey: pk,
	}, nil
}

type NKey struct {
	// Seed contains the public and private key in base32.
	// The KeyPair can be extracted from the seed, and used to sign JWTs.
	// Warning: keep this secure!!
	Seed    string        `json:"seed"`
	KeyPair nkeys.KeyPair `json:"-"`
	// PublicKey is used to identify the entity.
	// It acts like an ID in NATS.
	PublicKey string `json:"public_key"`
}

func (n *NKey) LoadNKey() error {
	var err error
	n.KeyPair, err = nkeys.FromSeed([]byte(n.Seed))
	if err != nil {
		return fmt.Errorf("loading keypair from seed: %w", err)
	}
	return nil
}

// AuthEntity is an entity that can be authenticated by the NATS server using
// JWT.
type AuthEntity struct {
	NKey
	// JWT is the JWT for the entity, signed with the KeyPair.
	JWT        string `json:"jwt"`
	SigningKey *NKey  `json:"signing_key,omitempty"`
}

func (a *AuthEntity) LoadNKey() error {
	if err := a.NKey.LoadNKey(); err != nil {
		return fmt.Errorf("loading nkey: %w", err)
	}
	if a.SigningKey != nil {
		if err := a.SigningKey.LoadNKey(); err != nil {
			return fmt.Errorf("loading signing key: %w", err)
		}
	}

	return nil
}

type ServerJWTAuth struct {
	// AccountServerURL is the configured `account_server_url` for the operator
	AccountServerURL string `json:"account_server_url"`

	Operator AuthEntity `json:"operator"`

	// SysAccount is the NATS system account.
	// It should not be needed outside of bootstrapping.
	SysAccount AuthEntity `json:"sys_account"`
	// SysUser is a user that has access to the the NATS system account.
	// It should not be needed outside of bootstrapping.
	SysUser AuthEntity `json:"sys_user"`

	// RootAccount is the Horizon root account.
	// The root account is where all the data is stored.
	RootAccount AuthEntity `json:"root_account"`
	// RootUser is a user that has access to the the Horizon root account.
	// It should not be needed outside of bootstrapping.
	RootUser AuthEntity `json:"root_user"`
}

func (s *ServerJWTAuth) LoadKeyPairs() error {
	if err := s.Operator.LoadNKey(); err != nil {
		return fmt.Errorf("loading operator nkeys: %w", err)
	}
	if err := s.SysAccount.LoadNKey(); err != nil {
		return fmt.Errorf("loading system account nkeys: %w", err)
	}
	if err := s.SysUser.LoadNKey(); err != nil {
		return fmt.Errorf("loading system user nkeys: %w", err)
	}
	if err := s.RootAccount.LoadNKey(); err != nil {
		return fmt.Errorf("loading root account nkeys: %w", err)
	}
	if err := s.RootUser.LoadNKey(); err != nil {
		return fmt.Errorf("loading root user nkeys: %w", err)
	}
	return nil
}
