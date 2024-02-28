package gateway

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/natsutil"
)

// handleAuthLogin generates nats user credentials for the root nats account.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	superAccountClient := hz.ObjectClient[accounts.Account]{
		Client: hz.NewClient(s.Conn, hz.WithClientInternal(true)),
	}
	rootAccount, err := superAccountClient.Get(
		r.Context(),
		hz.WithGetKey(hz.ObjectKey{
			Name:    hz.RootAccount,
			Account: hz.RootAccount,
		}),
	)
	if err != nil {
		httpError(w, err)
		return
	}
	userNKey, err := natsutil.NewUserNKey()
	if err != nil {
		httpError(w, fmt.Errorf("new user nkey: %w", err))
		return
	}
	signingKey, err := nkeys.FromSeed([]byte(rootAccount.Status.SigningKeySeed))
	if err != nil {
		httpError(w, fmt.Errorf("get account key pair: %w", err))
		return
	}
	claims := jwt.NewUserClaims(userNKey.PublicKey)
	claims.Name = uuid.NewString()
	claims.IssuerAccount = rootAccount.Status.ID
	claims.Pub.Allow.Add(hz.SubjectAPIAllowAll)
	claims.Expires = time.Now().Add(time.Hour * 24).Unix()
	claims.Claims()
	userJWT, err := claims.Encode(signingKey)
	if err != nil {
		httpError(w, fmt.Errorf("encode claims: %w", err))
		return
	}
	userConfig, err := jwt.FormatUserConfig(
		userJWT,
		[]byte(userNKey.Seed),
	)
	if err != nil {
		httpError(w, fmt.Errorf("format user config: %w", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(userConfig)
}
