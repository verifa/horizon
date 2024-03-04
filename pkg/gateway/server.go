package gateway

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/gateway/dummyoidc"
	"github.com/verifa/horizon/pkg/gateway/dummyoidc/storage"
	"github.com/verifa/horizon/pkg/hz"
	"golang.org/x/text/language"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v2"
	"github.com/nats-io/nats.go"
)

//go:embed dist/htmx-1.9.8.min.js
var htmxJS []byte

//go:embed dist/htmx-ext-response-targets-1.9.8.js
var htmxExtResponseTargetsJS []byte

//go:embed dist/htmx-ext-sse-1.9.8.js
var htmxExtSSEJS []byte

//go:embed dist/_hyperscript-0.9.12.min.js
var hyperscriptJS []byte

//go:embed dist/tailwind.css
var tailwindCSS []byte

// contextKey is used to store context values
type contextKey string

var authContext contextKey = "AUTH_CONTEXT"

func WithOIDCConfig(oidc OIDCConfig) ServerOption {
	return func(o *serverOptions) {
		o.oidc = &oidc
	}
}

func WithDummyAuthUsers(users ...storage.User) ServerOption {
	return func(o *serverOptions) {
		if o.dummyAuthUsers == nil {
			o.dummyAuthUsers = make(map[string]*storage.User)
		}
		for _, user := range users {
			u := user
			o.dummyAuthUsers[user.ID] = &u
		}
	}
}

func WithDummyAuthDefault(b bool) ServerOption {
	return func(o *serverOptions) {
		o.dummyAuthDefault = b
	}
}

func WithPort(port int) ServerOption {
	return func(o *serverOptions) {
		o.port = port
	}
}

type ServerOption func(*serverOptions)

type serverOptions struct {
	Conn *nats.Conn
	oidc *OIDCConfig

	dummyAuthUsers   map[string]*storage.User
	dummyAuthDefault bool

	listener net.Listener
	port     int
}

var defaultServerOptions = serverOptions{
	port: 9999,
}

func Start(
	ctx context.Context,
	conn *nats.Conn,
	auth *auth.Auth,
	opts ...ServerOption,
) (*Server, error) {
	s := Server{
		Conn:    conn,
		Auth:    auth,
		portals: make(map[string]hz.Portal),
	}

	if err := s.start(ctx, opts...); err != nil {
		return nil, fmt.Errorf("initializing server: %w", err)
	}
	return &s, nil
}

type Server struct {
	Conn       *nats.Conn
	Auth       *auth.Auth
	httpServer *http.Server
	dummyOIDC  *dummyoidc.Server

	portals map[string]hz.Portal
	watcher *hz.Watcher
}

func (s *Server) start(
	ctx context.Context,
	opts ...ServerOption,
) error {
	opt := defaultServerOptions
	for _, o := range opts {
		o(&opt)
	}
	watcher, err := hz.StartWatcher(
		ctx,
		s.Conn,
		hz.WithWatcherFor(hz.Portal{}),
		hz.WithWatcherFn(s.handlePortalEvent),
	)
	if err != nil {
		return fmt.Errorf("starting portal watcher: %w", err)
	}
	s.watcher = watcher

	logger := httplog.NewLogger("horizon", httplog.Options{
		JSON:             false,
		LogLevel:         slog.LevelInfo,
		Concise:          true,
		RequestHeaders:   true,
		MessageFieldName: "message",
		// TimeFieldFormat: time.RFC850,
		QuietDownRoutes: []string{
			"/",
			"/ping",
		},
		QuietDownPeriod: 10 * time.Second,
	})
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(httplog.RequestLogger(
		logger,
		[]string{
			"/dist/tailwind.css",
			"/dist/htmx.js",
			"/dist/htmx-ext-response-targets.js",
			"/dist/htmx-ext-sse.js",
		}),
	)
	r.Use(middleware.Recoverer)

	if !validateOneTrue(
		opt.oidc != nil,
		opt.dummyAuthUsers != nil,
		opt.dummyAuthDefault,
	) {
		opt.dummyAuthDefault = true
	}
	//
	// Auth
	//
	if opt.oidc == nil {
		if opt.dummyAuthDefault {
			opt.dummyAuthUsers = map[string]*storage.User{
				"admin": {
					ID:            "admin",
					Username:      "admin",
					Password:      "admin",
					Groups:        []string{"admin"},
					FirstName:     "Admin",
					LastName:      "Admin",
					Email:         "admin@localhost",
					EmailVerified: true,
					// How posh of you, admin!
					PreferredLanguage: language.BritishEnglish,
				},
			}
		}
		// Configure the dummyoidc server.
		dummyServer, err := dummyoidc.Start(ctx, dummyoidc.Config{
			Users: opt.dummyAuthUsers,
		})
		if err != nil {
			return fmt.Errorf("starting dummyoidc server: %w", err)
		}
		s.dummyOIDC = dummyServer
		opt.oidc = &OIDCConfig{
			Issuer:       dummyServer.Issuer,
			ClientID:     "web",
			ClientSecret: "secret",
			RedirectURL:  "http://localhost:9999/auth/callback",
		}
	}
	oidcHandler, err := newOIDCHandler(ctx, s.Conn, s.Auth, *opt.oidc)
	if err != nil {
		return fmt.Errorf("oidc auth middleware: %w", err)
	}
	//
	// Unprotected routes.
	//
	r.Get("/login", oidcHandler.login)
	r.Get("/logout", oidcHandler.logout)
	r.Get("/loggedout", s.serveLoggedOut)
	r.Get("/auth/callback", oidcHandler.authCallback)

	// This should be passable as an option to the server, allowing users to
	// override the default handler.
	var h GatewayHandler = &DefaultHandler{
		Conn: s.Conn,
	}
	//
	// Protected routes.
	//
	r.Group(func(r chi.Router) {
		r.Use(oidcHandler.authMiddleware)
		r.Get("/", h.GetHome)
		r.Get("/accounts", h.GetAccounts)
		r.Get("/accounts/new", h.GetAccountsNew)
		r.Post("/accounts", h.PostAccounts)
	})

	accountsHandler := AccountsHandler{
		Middleware: chi.Middlewares{oidcHandler.authMiddleware},
		Auth:       s.Auth,
		Conn:       s.Conn,
		Portals:    s.portals,
	}
	accountsRouter := accountsHandler.Router()
	r.Mount("/accounts/{account}", accountsRouter)

	objHandler := ObjectsHandler{
		Conn: s.Conn,
	}
	objRouter := objHandler.router()
	r.Mount("/v1/objects", objRouter)

	//
	// Static files.
	//
	r.Get("/dist/htmx.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/javascript")
		w.Write(htmxJS)
	})
	r.Get(
		"/dist/htmx-ext-response-targets.js",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Type", "text/javascript")
			w.Write(htmxExtResponseTargetsJS)
		},
	)
	r.Get(
		"/dist/htmx-ext-sse.js",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Type", "text/javascript")
			w.Write(htmxExtSSEJS)
		},
	)
	r.Get(
		"/dist/_hyperscript.js",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Type", "text/javascript")
			w.Write(hyperscriptJS)
		},
	)
	r.Get("/dist/tailwind.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/css")
		w.Write(tailwindCSS)
	})

	if opt.listener == nil {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", opt.port))
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		opt.listener = l
	}

	srv := http.Server{
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		Handler:           r,
	}
	s.httpServer = &srv

	go func() {
		if err := srv.Serve(opt.listener); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				slog.Error("http server", "error", err.Error())
			}
		}
	}()

	return nil
}

func (s *Server) Close() error {
	var errs error
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(context.TODO()); err != nil {
			errs = errors.Join(
				errs,
				fmt.Errorf("shutting down http server: %w", err),
			)
		}
	}
	if s.dummyOIDC != nil {
		if err := s.dummyOIDC.Close(); err != nil {
			errs = errors.Join(
				errs,
				fmt.Errorf("closing dummyoidc server: %w", err),
			)
		}
	}
	if s.watcher != nil {
		s.watcher.Close()
	}
	return errs
}

func (s *Server) handlePortalEvent(
	event hz.Event,
) (hz.Result, error) {
	switch event.Operation {
	case hz.EventOperationPut:
		var portal hz.Portal
		if err := json.Unmarshal(event.Data, &portal); err != nil {
			return hz.Result{}, fmt.Errorf("unmarshalling portal: %w", err)
		}
		s.portals[hz.KeyFromObject(event.Key)] = portal
		return hz.Result{}, nil
	case hz.EventOperationDelete, hz.EventOperationPurge:
		delete(s.portals, hz.KeyFromObject(event.Key))
	}
	return hz.Result{}, nil
}

func (s *Server) serveLoggedOut(w http.ResponseWriter, r *http.Request) {
	body := loggedOutPage()
	layout("Logged Out", nil, body).Render(r.Context(), w)
}

func validateOneTrue(b ...bool) bool {
	var count int
	for _, v := range b {
		if v {
			count++
		}
	}
	return count == 1
}

func httpError(w http.ResponseWriter, err error) {
	var hzErr *hz.Error
	if errors.As(err, &hzErr) {
		http.Error(w, hzErr.Message, hzErr.Status)
		return
	}

	http.Error(w, err.Error(), http.StatusInternalServerError)
}
