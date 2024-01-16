package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

const (
	subjLookupAccount = "$SYS.REQ.ACCOUNT.%s.CLAIMS.LOOKUP"
	// For some reason this returns an error:
	// 	{"account":"n/a","code":500,"description":"jwt update resulted in error
	// 	expected 3 chunks"}
	// subjUpdateAccount = "$SYS.REQ.ACCOUNT.%s.CLAIMS.UPDATE"
	subjUpdateAccount = "$SYS.REQ.CLAIMS.UPDATE"
)

var ErrAccountNotFound = errors.New("account not found")

func AccountClaimsLookup(
	ctx context.Context,
	nc *nats.Conn,
	accountPublicKey string,
) (*jwt.AccountClaims, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	lookupMessage, err := nc.RequestWithContext(
		ctx,
		fmt.Sprintf(subjLookupAccount, accountPublicKey),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"looking up account with ID %s: %w",
			accountPublicKey,
			err,
		)
	}
	if lookupMessage.Data == nil || len(lookupMessage.Data) == 0 {
		return nil, ErrAccountNotFound
	}
	ac, err := jwt.DecodeAccountClaims(string(lookupMessage.Data))
	if err != nil {
		return nil, fmt.Errorf("decoding account claims: %w", err)
	}
	return ac, nil
}

func AccountJWTUpdate(
	ctx context.Context,
	nc *nats.Conn,
	accJWT string,
) (string, error) {
	updateReply, err := nc.Request(
		subjUpdateAccount,
		[]byte(accJWT),
		time.Second,
	)
	if err != nil {
		return "", fmt.Errorf("updating account claims: %w", err)
	}
	resp := server.ServerAPIResponse{}
	if err := json.Unmarshal(updateReply.Data, &resp); err != nil {
		return "", fmt.Errorf("unmarshaling claims update reply: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("claims update error: %w", resp.Error)
	}
	return accJWT, nil
}

func AccountClaimsUpdate(
	ctx context.Context,
	nc *nats.Conn,
	operatorKeyPair nkeys.KeyPair,
	acc *jwt.AccountClaims,
) (string, error) {
	if err := validateClaims(acc); err != nil {
		return "", fmt.Errorf("validating account claims: %w", err)
	}
	accJWT, err := acc.Encode(operatorKeyPair)
	if err != nil {
		return "", fmt.Errorf("encoding account claims: %w", err)
	}
	return AccountJWTUpdate(ctx, nc, accJWT)
}

func AccountClaimsUpdateFn(
	ctx context.Context,
	nc *nats.Conn,
	operatorKeyPair nkeys.KeyPair,
	accountPublicKey string,
	fn func(claims *jwt.AccountClaims) error,
) error {
	ac, err := AccountClaimsLookup(ctx, nc, accountPublicKey)
	if err != nil {
		return fmt.Errorf("looking up account claims: %w", err)
	}
	if err := fn(ac); err != nil {
		return fmt.Errorf("updating account claims: %w", err)
	}
	_, err = AccountClaimsUpdate(ctx, nc, operatorKeyPair, ac)
	return err
}

func validateClaims(claims jwt.Claims) error {
	vr := jwt.ValidationResults{}
	claims.Validate(&vr)
	if vr.IsBlocking(true) {
		var vErr error
		for _, iss := range vr.Issues {
			vErr = errors.Join(vErr, iss)
		}
		return vErr
	}
	return nil
}
