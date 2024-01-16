package natsutil

import (
	"fmt"

	"github.com/nats-io/jwt/v2"
)

func BootstrapServerJWTAuth() (ServerJWTAuth, error) {
	host := "0.0.0.0"
	port := 4222

	accountServerURL := fmt.Sprintf("nats://%s:%d", host, port)
	vr := jwt.ValidationResults{}

	// Create operator
	opNK, err := NewOperatorNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("creating operator: %w", err)
	}
	// Create operator signing keys
	opSK, err := NewOperatorNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf(
			"creating operator nkey: %w",
			err,
		)
	}
	// Create system account key pair
	saNK, err := NewAccountNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf(
			"creating system account key pair: %w",
			err,
		)
	}
	// Create system account signing key
	saSK, err := NewAccountNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf(
			"creating system account signing key: %w",
			err,
		)
	}
	// Create operator JWT
	opC := jwt.NewOperatorClaims(opNK.PublicKey)
	opC.Name = "test"
	opC.AccountServerURL = accountServerURL
	opC.SystemAccount = saNK.PublicKey
	// Allow only encoding with signing keys for ALL entities.
	// This means accounts and users need to be encoded with signing keys.
	// An account is encoded with an operator signing key, and a user is encoded
	// with an account signing key.
	// This is a security measure.
	opC.StrictSigningKeyUsage = true
	opC.SigningKeys.Add(opSK.PublicKey)
	opC.Validate(&vr)
	opJWT, err := opC.Encode(opNK.KeyPair)
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("encoding operator JWT: %w", err)
	}

	// Create system account JWT
	saC := jwt.NewAccountClaims(saNK.PublicKey)
	saC.Name = "SYS"
	saC.SigningKeys.Add(saSK.PublicKey)
	// Export some services to import in actor account
	saC.Exports.Add(&jwt.Export{
		Type:    jwt.Service,
		Name:    "account-monitoring-services",
		Subject: jwt.Subject("$SYS.REQ.ACCOUNT.*.CLAIMS.*"),
		Info: jwt.Info{
			Description: "Request account specific monitoring services for: SUBSZ, CONNZ, LEAFZ, JSZ and INFO",
			InfoURL:     "https://docs.nats.io/nats-server/configuration/sys_accounts",
		},
	})
	saC.Exports.Add(&jwt.Export{
		Type:    jwt.Service,
		Name:    "account-claims-subjects",
		Subject: jwt.Subject("$SYS.REQ.CLAIMS.*"),
		Info: jwt.Info{
			Description: "Subjects to manage accounts (create, delete, list, etc.). This avoids needing to connec to the SYS account.",
			InfoURL:     "https://docs.nats.io/running-a-nats-service/nats_admin/security/jwt#subjects-available-when-using-nats-based-resolver",
		},
	})
	saC.Exports.Add(&jwt.Export{
		Type:    jwt.Service,
		Name:    "test",
		Subject: jwt.Subject("test"),
	})
	saC.Validate(&vr)
	saJWT, err := saC.Encode(opSK.KeyPair)
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf(
			"encoding system account JWT: %w",
			err,
		)
	}

	// Create system user
	suNK, err := NewUserNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("creating system user: %w", err)
	}
	// Create system user JWT
	suC := jwt.NewUserClaims(suNK.PublicKey)
	suC.Name = "sys"
	suC.IssuerAccount = saNK.PublicKey
	suC.Validate(&vr)

	suJWT, err := suC.Encode(saSK.KeyPair)
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("encoding system user JWT: %w", err)
	}

	//
	// Create Horizon Root Account
	//
	aaNK, err := NewAccountNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("creating root account: %w", err)
	}

	// Create actor account signing key
	aaSK, err := NewAccountNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf(
			"creating root account nkey: %w",
			err,
		)
	}
	// Create actor account JWT
	aaC := jwt.NewAccountClaims(aaNK.PublicKey)
	aaC.Name = "horizon-root"
	aaC.SigningKeys.Add(aaSK.PublicKey)
	aaC.Limits.JetStreamLimits.Consumer = -1
	aaC.Limits.JetStreamLimits.DiskMaxStreamBytes = -1
	aaC.Limits.JetStreamLimits.DiskStorage = -1
	aaC.Limits.JetStreamLimits.MaxAckPending = -1
	aaC.Limits.JetStreamLimits.MemoryMaxStreamBytes = -1
	aaC.Limits.JetStreamLimits.MemoryStorage = -1
	aaC.Limits.JetStreamLimits.Streams = -1
	aaC.Imports.Add(&jwt.Import{
		Type:    jwt.Service,
		Name:    "account-monitoring-services",
		Account: saNK.PublicKey,
		Subject: jwt.Subject("$SYS.REQ.ACCOUNT.*.CLAIMS.*"),
	})
	aaC.Imports.Add(&jwt.Import{
		Type:    jwt.Service,
		Name:    "account-claims-subjects",
		Account: saNK.PublicKey,
		Subject: jwt.Subject("$SYS.REQ.CLAIMS.*"),
	})
	aaC.Validate(&vr)
	aaJWT, err := aaC.Encode(opSK.KeyPair)
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf(
			"encoding root account JWT: %w",
			err,
		)
	}

	//
	// Create Horizon Root User
	//
	auNK, err := NewUserNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("creating root user: %w", err)
	}
	// Create root user JWT
	auC := jwt.NewUserClaims(auNK.PublicKey)
	auC.Name = "horizon-root"
	auC.IssuerAccount = aaNK.PublicKey
	auC.Validate(&vr)
	auJWT, err := auC.Encode(aaSK.KeyPair)
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("encoding root user JWT: %w", err)
	}

	//
	// Create Horizon Root Account
	//
	return ServerJWTAuth{
		AccountServerURL: accountServerURL,
		Operator: AuthEntity{
			NKey:       *opNK,
			JWT:        opJWT,
			SigningKey: opSK,
		},

		SysAccount: AuthEntity{
			NKey:       *saNK,
			JWT:        saJWT,
			SigningKey: saSK,
		},
		SysUser: AuthEntity{
			NKey: *suNK,
			JWT:  suJWT,
		},

		RootAccount: AuthEntity{
			NKey:       *aaNK,
			JWT:        aaJWT,
			SigningKey: aaSK,
		},
		RootUser: AuthEntity{
			NKey: *auNK,
			JWT:  auJWT,
		},
	}, nil
}
