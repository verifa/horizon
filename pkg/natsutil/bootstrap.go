package natsutil

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nats-io/jwt/v2"
)

func BootstrapServerJWTAuth() (ServerJWTAuth, error) {
	host := "0.0.0.0"
	port := 4222

	accountServerURL := fmt.Sprintf("nats://%s:%d", host, port)
	vr := jwt.ValidationResults{}

	// Create operator
	operatorNKey, err := NewOperatorNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("creating operator: %w", err)
	}
	// Create operator signing keys
	operatorSigningKey, err := NewOperatorNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf(
			"creating operator nkey: %w",
			err,
		)
	}
	// Create system account key pair
	sysAccountNKey, err := NewAccountNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf(
			"creating system account key pair: %w",
			err,
		)
	}
	// Create system account signing key
	sysAccountSigningKey, err := NewAccountNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf(
			"creating system account signing key: %w",
			err,
		)
	}
	// Create operator JWT
	operatorClaims := jwt.NewOperatorClaims(operatorNKey.PublicKey)
	operatorClaims.Name = "horizon"
	operatorClaims.AccountServerURL = accountServerURL
	operatorClaims.SystemAccount = sysAccountNKey.PublicKey
	// Allow only encoding with signing keys for ALL entities.
	// This means accounts and users need to be encoded with signing keys.
	// An account is encoded with an operator signing key, and a user is encoded
	// with an account signing key.
	// This is a security measure.
	operatorClaims.StrictSigningKeyUsage = true
	operatorClaims.SigningKeys.Add(operatorSigningKey.PublicKey)
	operatorClaims.Validate(&vr)
	operatorJWT, err := operatorClaims.Encode(operatorNKey.KeyPair)
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("encoding operator JWT: %w", err)
	}

	// Create system account JWT
	sysAccountClaims := jwt.NewAccountClaims(sysAccountNKey.PublicKey)
	sysAccountClaims.Name = "SYS"
	sysAccountClaims.SigningKeys.Add(sysAccountSigningKey.PublicKey)
	// Export some services to import in root account
	sysAccountClaims.Exports.Add(&jwt.Export{
		Type:    jwt.Service,
		Name:    "account-monitoring-services",
		Subject: jwt.Subject("$SYS.REQ.ACCOUNT.*.CLAIMS.*"),
		Info: jwt.Info{
			Description: "Request account specific monitoring services for: SUBSZ, CONNZ, LEAFZ, JSZ and INFO",
			InfoURL:     "https://docs.nats.io/nats-server/configuration/sys_accounts",
		},
	})
	sysAccountClaims.Exports.Add(&jwt.Export{
		Type:    jwt.Service,
		Name:    "account-claims-subjects",
		Subject: jwt.Subject("$SYS.REQ.CLAIMS.*"),
		Info: jwt.Info{
			Description: "Subjects to manage accounts (create, delete, list, etc.). This avoids needing to connec to the SYS account.",
			InfoURL:     "https://docs.nats.io/running-a-nats-service/nats_admin/security/jwt#subjects-available-when-using-nats-based-resolver",
		},
	})
	sysAccountClaims.Exports.Add(&jwt.Export{
		Type:    jwt.Service,
		Name:    "test",
		Subject: jwt.Subject("test"),
	})
	sysAccountClaims.Validate(&vr)
	sysAccountJWT, err := sysAccountClaims.Encode(operatorSigningKey.KeyPair)
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf(
			"encoding system account JWT: %w",
			err,
		)
	}

	// Create system user
	sysUserNKey, err := NewUserNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("creating system user: %w", err)
	}
	// Create system user JWT
	sysUserClaims := jwt.NewUserClaims(sysUserNKey.PublicKey)
	sysUserClaims.Name = "sys"
	sysUserClaims.IssuerAccount = sysAccountNKey.PublicKey
	sysUserClaims.Validate(&vr)

	sysUserJWT, err := sysUserClaims.Encode(sysAccountSigningKey.KeyPair)
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("encoding system user JWT: %w", err)
	}

	//
	// Create Horizon Root Account
	//
	rootAccountNKey, err := NewAccountNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("creating root account: %w", err)
	}

	// Create root account signing key
	rootAccountSigningKey, err := NewAccountNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf(
			"creating root account nkey: %w",
			err,
		)
	}
	// Create root account JWT
	rootAccountClaims := jwt.NewAccountClaims(rootAccountNKey.PublicKey)
	rootAccountClaims.Name = "horizon-root"
	rootAccountClaims.SigningKeys.Add(rootAccountSigningKey.PublicKey)
	rootAccountClaims.Limits.JetStreamLimits.Consumer = -1
	rootAccountClaims.Limits.JetStreamLimits.DiskMaxStreamBytes = -1
	rootAccountClaims.Limits.JetStreamLimits.DiskStorage = -1
	rootAccountClaims.Limits.JetStreamLimits.MaxAckPending = -1
	rootAccountClaims.Limits.JetStreamLimits.MemoryMaxStreamBytes = -1
	rootAccountClaims.Limits.JetStreamLimits.MemoryStorage = -1
	rootAccountClaims.Limits.JetStreamLimits.Streams = -1
	rootAccountClaims.Imports.Add(&jwt.Import{
		Type:    jwt.Service,
		Name:    "account-monitoring-services",
		Account: sysAccountNKey.PublicKey,
		Subject: jwt.Subject("$SYS.REQ.ACCOUNT.*.CLAIMS.*"),
	})
	rootAccountClaims.Imports.Add(&jwt.Import{
		Type:    jwt.Service,
		Name:    "account-claims-subjects",
		Account: sysAccountNKey.PublicKey,
		Subject: jwt.Subject("$SYS.REQ.CLAIMS.*"),
	})
	rootAccountClaims.Validate(&vr)
	rootAccountJWT, err := rootAccountClaims.Encode(operatorSigningKey.KeyPair)
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf(
			"encoding root account JWT: %w",
			err,
		)
	}

	//
	// Create Horizon Root User
	//
	rootUserNKey, err := NewUserNKey()
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("creating root user: %w", err)
	}
	// Create root user JWT
	rootUserClaims := jwt.NewUserClaims(rootUserNKey.PublicKey)
	rootUserClaims.Name = "horizon-root"
	rootUserClaims.IssuerAccount = rootAccountNKey.PublicKey
	rootUserClaims.Validate(&vr)
	rootUserJWT, err := rootUserClaims.Encode(rootAccountSigningKey.KeyPair)
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("encoding root user JWT: %w", err)
	}

	//
	// Create Horizon Root Account
	//
	return ServerJWTAuth{
		AccountServerURL: accountServerURL,
		Operator: AuthEntity{
			NKey:       *operatorNKey,
			JWT:        operatorJWT,
			SigningKey: operatorSigningKey,
		},

		SysAccount: AuthEntity{
			NKey:       *sysAccountNKey,
			JWT:        sysAccountJWT,
			SigningKey: sysAccountSigningKey,
		},
		SysUser: AuthEntity{
			NKey: *sysUserNKey,
			JWT:  sysUserJWT,
		},

		RootAccount: AuthEntity{
			NKey:       *rootAccountNKey,
			JWT:        rootAccountJWT,
			SigningKey: rootAccountSigningKey,
		},
		RootUser: AuthEntity{
			NKey: *rootUserNKey,
			JWT:  rootUserJWT,
		},
	}, nil
}

// LoadServerJWTAuth loads the ServerJWTAuth from the given file.
// This method is intended to use for testing controllers/actors or other
// services with a local running instance of the horizon server.
//
// For other use cases, you should create nats credentials via the horizon
// server and provide those to the services you need to connect to NATS/Horizon.
func LoadServerJWTAuth(file string) (ServerJWTAuth, error) {
	f, err := os.Open(file)
	if err != nil {
		return ServerJWTAuth{}, fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	var jwtAuth ServerJWTAuth
	if err := json.NewDecoder(f).Decode(&jwtAuth); err != nil {
		return ServerJWTAuth{}, fmt.Errorf("decode: %w", err)
	}
	if err := jwtAuth.LoadKeyPairs(); err != nil {
		return ServerJWTAuth{}, fmt.Errorf("load key pairs: %w", err)
	}
	return jwtAuth, nil
}

// loadOrBootstrapJWTAuth loads the JWT auth from the given file, or bootstraps
// it if the file does not exist.
// If the file does not exist, the auth is bootstrapped and saved to the file.
func loadOrBootstrapJWTAuth(file string) (ServerJWTAuth, error) {
	f, err := os.Open(file)
	if err != nil {
		if !os.IsNotExist(err) {
			return ServerJWTAuth{}, fmt.Errorf("open: %w", err)
		}
		// If it doesn't exist, bootstrap the auth and save it
		jwtAuth, err := BootstrapServerJWTAuth()
		if err != nil {
			return ServerJWTAuth{}, fmt.Errorf("bootstrap: %w", err)
		}
		// Ensure the workdir directory exists
		if err := os.MkdirAll(filepath.Dir(file), os.ModePerm); err != nil {
			return ServerJWTAuth{}, fmt.Errorf("mkdir: %w", err)
		}
		// Save the auth
		b, err := json.Marshal(jwtAuth)
		if err != nil {
			return ServerJWTAuth{}, fmt.Errorf("marshal: %w", err)
		}
		if err := os.WriteFile(file, b, os.ModePerm); err != nil {
			return ServerJWTAuth{}, fmt.Errorf("write file: %w", err)
		}
		return jwtAuth, nil
	}
	defer f.Close()
	var jwtAuth ServerJWTAuth
	if err := json.NewDecoder(f).Decode(&jwtAuth); err != nil {
		return ServerJWTAuth{}, fmt.Errorf("decode: %w", err)
	}
	if err := jwtAuth.LoadKeyPairs(); err != nil {
		return ServerJWTAuth{}, fmt.Errorf("load key pairs: %w", err)
	}
	return jwtAuth, nil
}
