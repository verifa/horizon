package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/hz"
	"golang.org/x/oauth2"
)

type OIDCConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	// TODO: add more...
}

const (
	stateCookieName   = "hz.oauthstate"
	stateCookieMaxAge = time.Minute * 5
)

func newOIDCHandler(
	ctx context.Context,
	conn *nats.Conn,
	auth *auth.Auth,
	config OIDCConfig,
) (*oidcHandler, error) {
	provider, err := oidc.NewProvider(ctx, config.Issuer)
	if err != nil {
		return nil, fmt.Errorf("new oidc provider: %w", err)
	}
	// Configure an OpenID Connect aware OAuth2 client.
	oauth2Config := oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,

		// Discovery returns the OAuth2 endpoints.
		Endpoint: provider.Endpoint(),

		// "openid" is a required scope for OpenID Connect flows.
		Scopes: []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}
	oidcConfig := &oidc.Config{
		ClientID: oauth2Config.ClientID,
	}
	verifier := provider.Verifier(oidcConfig)

	return &oidcHandler{
		sessions: auth.Sessions,
		config:   &oauth2Config,
		verifier: verifier,
		provider: provider,
		conn:     conn,
	}, nil
}

type oidcHandler struct {
	sessions *auth.Sessions
	config   *oauth2.Config
	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider
	conn     *nats.Conn
}

func (or *oidcHandler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" ||
			r.URL.Path == "/loggedout" ||
			r.URL.Path == "/auth/callback" {
			next.ServeHTTP(w, r)
			return
		}
		sessionID, err := getSessionCookie(r)
		if err != nil {
			loginReturnURL := "/login?return_url=" + url.QueryEscape(
				r.RequestURI,
			)
			http.Redirect(w, r, loginReturnURL, http.StatusSeeOther)
			return
		}
		userInfo, err := or.sessions.Get(
			r.Context(),
			sessionID.String(),
		)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidCredentials) {
				loginReturnURL := "/login?return_url=" + url.QueryEscape(
					r.RequestURI,
				)
				http.Redirect(w, r, loginReturnURL, http.StatusSeeOther)
				return
			}
			httpError(w, err)
			return
		}
		ctx := context.WithValue(r.Context(), authContext, userInfo)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (or *oidcHandler) login(w http.ResponseWriter, req *http.Request) {
	state, err := or.stateCookie(w, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	nonce := uuid.New().String()
	c := &http.Cookie{
		Name:     "nonce",
		Value:    nonce,
		MaxAge:   int(time.Hour.Seconds()),
		Secure:   req.TLS != nil,
		HttpOnly: true,
	}
	http.SetCookie(w, c)
	http.Redirect(
		w,
		req,
		or.config.AuthCodeURL(state, oidc.Nonce(nonce)),
		http.StatusFound,
	)
}

func (or *oidcHandler) logout(w http.ResponseWriter, req *http.Request) {
	sessionID, err := getSessionCookie(req)
	if err != nil {
		if !errors.Is(err, http.ErrNoCookie) {
			http.Error(
				w,
				"Invalid session: "+err.Error(),
				http.StatusUnauthorized,
			)
		}
		w.Header().Add("HX-Redirect", "/loggedout")
		return
	}
	if err := or.sessions.Delete(
		req.Context(),
		sessionID.String(),
	); err != nil {
		httpError(w, err)
		return
	}
	w.Header().Add("HX-Redirect", "/loggedout")
}

func (or *oidcHandler) authCallback(w http.ResponseWriter, req *http.Request) {
	if errMsg := req.FormValue("error"); errMsg != "" {
		errorDesc := req.FormValue("error_description")
		http.Error(
			w,
			html.EscapeString(errMsg)+": "+html.EscapeString(errorDesc),
			http.StatusBadRequest,
		)
		return
	}

	returnURL, err := or.verifyState(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	oauth2Token, err := or.config.Exchange(
		req.Context(),
		req.URL.Query().Get("code"),
	)
	if err != nil {
		http.Error(w, "exchange: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Extract the ID Token from OAuth2 token.
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	// Parse and verify ID Token payload.
	idToken, err := or.verifier.Verify(req.Context(), rawIDToken)
	if err != nil {
		http.Error(
			w,
			"token verification failed: "+err.Error(),
			http.StatusUnauthorized,
		)
		return
	}
	// Verify the nonce
	nonce, err := req.Cookie("nonce")
	if err != nil {
		http.Error(w, "Nonce not found", http.StatusUnauthorized)
		return
	}
	if idToken.Nonce != nonce.Value {
		http.Error(
			w,
			"Nonce did not match token nonce",
			http.StatusUnauthorized,
		)
		return
	}
	// TODO: make this configurable by config...
	// Some OIDC providers require querying the UserInfo endpoint to get claims.
	// Make this configurable via the OIDC configs.
	userInfo, err := or.provider.UserInfo(
		req.Context(),
		oauth2.StaticTokenSource(oauth2Token),
	)
	if err != nil {
		http.Error(
			w,
			"failed to get user info: "+err.Error(),
			http.StatusUnauthorized,
		)
		return
	}
	var i interface{}
	if err := userInfo.Claims(&i); err != nil {
		http.Error(
			w,
			"unmarshalling user info: "+err.Error(),
			http.StatusUnauthorized,
		)
		return
	}
	spew.Dump(i)

	var claims auth.UserInfo
	if err := idToken.Claims(&claims); err != nil {
		http.Error(
			w,
			"unmarshalling claims: "+err.Error(),
			http.StatusUnauthorized,
		)
		return
	}
	if err := userInfo.Claims(&claims); err != nil {
		http.Error(
			w,
			"unmarshalling user info: "+err.Error(),
			http.StatusUnauthorized,
		)
		return
	}

	sessionID, err := or.sessions.New(req.Context(), claims)
	if err != nil {
		httpError(w, err)
		return
	}
	writeSessionCookieHeader(w, sessionID)
	http.Redirect(w, req, returnURL, http.StatusSeeOther)
}

func (or *oidcHandler) stateCookie(
	w http.ResponseWriter,
	req *http.Request,
) (string, error) {
	// Check if there is a return_url given to the login request.
	returnURL := req.FormValue("return_url")
	randStr, err := randString(24)
	if err != nil {
		return "", fmt.Errorf("failed to generate app state: %w", err)
	}
	if returnURL == "" {
		returnURL = "/"
	}
	cookieValue := fmt.Sprintf("%s:%s", randStr, returnURL)
	// From ArgoCD:
	// https://github.com/argoproj/argo-cd/blob/740df9a13e7a82ca98ccd7577e0fe6ab32b33bd7/util/oidc/oidc.go#L163C2-L163C2
	// if encrypted, err := crypto.Encrypt([]byte(cookieValue),
	// a.encryptionKey); err != nil {
	// 	return "", err
	// }
	// cookieValue = hex.EncodeToString(encrypted)
	hexCookieValue := hex.EncodeToString([]byte(cookieValue))
	slog.Info(
		"state cookie",
		"return_url",
		returnURL,
		"cookie_value",
		cookieValue,
		"hex_cookie_value",
		hexCookieValue,
	)
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    hexCookieValue,
		Expires:  time.Now().Add(stateCookieMaxAge),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   req.TLS != nil,
	})
	return randStr, nil
}

func (or *oidcHandler) verifyState(
	req *http.Request,
) (string, error) {
	state := req.FormValue("state")
	if state == "" {
		return "", errors.New("missing state from request")
	}
	stateCookieVal := ""
	cookies := req.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == stateCookieName {
			if cookie.Value != "" {
				stateCookieVal = cookie.Value
			}
		}
	}
	if stateCookieVal == "" {
		return "", errors.New("missing or empty state cookie")
	}
	rawState, err := hex.DecodeString(stateCookieVal)
	if err != nil {
		return "", fmt.Errorf("failed to decode state cookie: %w", err)
	}
	slog.Info(
		"verifyState",
		"rawState",
		string(rawState),
		"stateCookie",
		stateCookieVal,
		"state",
		state,
	)
	stateParts := strings.SplitN(string(rawState), ":", 2)
	if len(stateParts) != 2 {
		return "", errors.New("invalid state cookie")
	}

	nonce := stateParts[0]
	// TODO: we probably want to check the returnURL is valid...
	returnURL := stateParts[1]

	if state != nonce {
		return "", errors.New("invalid state")
	}

	return returnURL, nil
}

func getSessionCookie(r *http.Request) (uuid.UUID, error) {
	sessionCookie, err := r.Cookie(hz.CookieSession)
	if err != nil {
		return uuid.UUID{}, err
	}
	return uuid.Parse(sessionCookie.Value)
}

func writeSessionCookieHeader(w http.ResponseWriter, sessionID string) {
	// Create cookie
	http.SetCookie(w, &http.Cookie{
		Name:     hz.CookieSession,
		Value:    sessionID,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(time.Hour.Seconds() * 8),
		Path:     "/",
	})
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// randString generates, from the set of capital and lowercase letters, a
// cryptographically-secure pseudo-random string of a given length.
func randString(n int) (string, error) {
	return randStringFromCharset(n, letterBytes)
}

// randStringFromCharset generates, from a given charset, a
// cryptographically-secure pseudo-random string of a given length.
func randStringFromCharset(n int, charset string) (string, error) {
	b := make([]byte, n)
	maxIdx := big.NewInt(int64(len(charset)))
	for i := 0; i < n; i++ {
		randIdx, err := rand.Int(rand.Reader, maxIdx)
		if err != nil {
			return "", fmt.Errorf("failed to generate random string: %w", err)
		}
		// randIdx is necessarily safe to convert to int, because the max came
		// from an int.
		randIdxInt := int(randIdx.Int64())
		b[i] = charset[randIdxInt]
	}
	return string(b), nil
}
