package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/broker"
	"github.com/verifa/horizon/pkg/controller"
	"github.com/verifa/horizon/pkg/extensions/core"
	"github.com/verifa/horizon/pkg/gateway"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/natsutil"
	"github.com/verifa/horizon/pkg/store"
)

func WithDevMode() ServerOption {
	return func(o *serverOptions) {
		o.devMode = true
	}
}

func WithNATSConn(conn *nats.Conn) ServerOption {
	return func(o *serverOptions) {
		o.conn = conn
	}
}

func WithRunAuth(b bool) ServerOption {
	return func(o *serverOptions) {
		o.runAuth = b
	}
}

func WithAuthOptions(opts ...auth.Option) ServerOption {
	return func(o *serverOptions) {
		o.runAuth = true
		o.authOptions = append(o.authOptions, opts...)
	}
}

func WithRunBroker(b bool) ServerOption {
	return func(o *serverOptions) {
		o.runBroker = b
	}
}

func WithRunNATS(b bool) ServerOption {
	return func(o *serverOptions) {
		o.runNATSServer = b
	}
}

func WithNATSOptions(opts ...natsutil.ServerOption) ServerOption {
	return func(o *serverOptions) {
		o.runNATSServer = true
		o.natsOptions = append(o.natsOptions, opts...)
	}
}

func WithRunStore(b bool) ServerOption {
	return func(o *serverOptions) {
		o.runStore = b
	}
}

func WithStoreOptions(opts ...store.StoreOption) ServerOption {
	return func(o *serverOptions) {
		o.runStore = true
		o.storeOptions = append(o.storeOptions, opts...)
	}
}

func WithRunGateway(b bool) ServerOption {
	return func(o *serverOptions) {
		o.runGateway = b
	}
}

func WithGatewayOptions(opts ...gateway.ServerOption) ServerOption {
	return func(o *serverOptions) {
		o.runGateway = true
		o.gatewayOptions = append(o.gatewayOptions, opts...)
	}
}

type ServerOption func(*serverOptions)

type serverOptions struct {
	conn *nats.Conn

	devMode bool

	runNATSServer bool
	runAuth       bool
	runBroker     bool
	runStore      bool
	runGateway    bool

	runCustomResourceDefinitionController bool
	runSecretsController                  bool
	runNamespaceController                bool
	runPortalController                   bool

	natsOptions                []natsutil.ServerOption
	authOptions                []auth.Option
	storeOptions               []store.StoreOption
	gatewayOptions             []gateway.ServerOption
	namespaceControllerOptions []controller.Option
}

type Server struct {
	Conn *nats.Conn

	NS      *natsutil.Server
	Auth    *auth.Auth
	Broker  *broker.Broker
	Store   *store.Store
	Gateway *gateway.Server

	CtlrCustomResourceDefinitions *controller.Controller
	CtlrSecrets                   *controller.Controller
	CtlrNamespaces                *controller.Controller
	CltrPortals                   *controller.Controller
}

func Start(
	ctx context.Context,
	opts ...ServerOption,
) (*Server, error) {
	s := Server{}

	if err := s.Start(ctx, opts...); err != nil {
		return nil, fmt.Errorf("starting server: %w", err)
	}
	return &s, nil
}

func (s *Server) Start(ctx context.Context, opts ...ServerOption) error {
	opt := serverOptions{
		runNATSServer: true,
		runAuth:       true,
		runBroker:     true,
		runStore:      true,
		runGateway:    true,

		runCustomResourceDefinitionController: true,
		runSecretsController:                  true,
		runNamespaceController:                true,
		runPortalController:                   true,
	}
	for _, o := range opts {
		o(&opt)
	}
	if opt.conn != nil {
		s.Conn = opt.conn
	}
	if opt.runNATSServer {
		ts, err := natsutil.NewServer(opt.natsOptions...)
		if err != nil {
			return fmt.Errorf("creating nats server: %w", err)
		}
		if err := ts.StartUntilReady(); err != nil {
			return fmt.Errorf("starting nats server: %w", err)
		}
		if err := ts.PublishRootNamespace(); err != nil {
			return fmt.Errorf("publishing root horizon namespace: %w", err)
		}
		conn, err := ts.RootUserConn()
		if err != nil {
			return fmt.Errorf("connecting to nats: %w", err)
		}
		s.NS = &ts
		s.Conn = conn
	}

	if s.Conn == nil {
		return errors.New("nats connection required")
	}

	if err := store.InitKeyValue(ctx, s.Conn, opt.storeOptions...); err != nil {
		return fmt.Errorf("initializing key value store: %w", err)
	}
	if opt.runStore {
		store, err := store.Start(ctx, s.Conn, s.Auth, opt.storeOptions...)
		if err != nil {
			return fmt.Errorf("starting store: %w", err)
		}
		s.Store = store
	}

	if opt.runCustomResourceDefinitionController {
		ctlr, err := controller.Start(
			ctx,
			s.Conn,
			controller.WithFor(core.CustomResourceDefinition{}),
			controller.WithValidatorCUE(false),
			controller.WithValidator(
				&hz.ValidateNamespaceRoot{},
			),
			controller.WithValidator(
				&core.CustomResourceDefinitionValidate{},
			),
		)
		if err != nil {
			return fmt.Errorf("starting crd controller: %w", err)
		}
		s.CtlrCustomResourceDefinitions = ctlr
	}

	if opt.runAuth {
		auth, err := auth.Start(ctx, s.Conn, opt.authOptions...)
		if err != nil {
			return fmt.Errorf("starting auth: %w", err)
		}
		s.Auth = auth
	}

	if s.Auth == nil {
		return errors.New("auth service/component required")
	}

	if opt.runBroker {
		broker := broker.Broker{
			Conn: s.Conn,
			Auth: s.Auth,
		}
		if err := broker.Start(ctx); err != nil {
			return fmt.Errorf("starting broker: %w", err)
		}
		s.Broker = &broker
	}
	if opt.runGateway {
		gw, err := gateway.Start(ctx, s.Conn, s.Auth, opt.gatewayOptions...)
		if err != nil {
			return fmt.Errorf("starting gateway: %w", err)
		}
		s.Gateway = gw
	}

	if opt.runSecretsController {
		ctlr, err := controller.Start(
			ctx,
			s.Conn,
			controller.WithFor(core.Secret{}),
		)
		if err != nil {
			return fmt.Errorf("starting secrets controller: %w", err)
		}
		s.CtlrSecrets = ctlr
	}
	if opt.runNamespaceController {
		defaultOptions := []controller.Option{
			controller.WithFor(core.Namespace{}),
			controller.WithValidator(&hz.ValidateNamespaceRoot{}),
		}

		ctlr, err := controller.Start(
			ctx,
			s.Conn,
			append(defaultOptions, opt.namespaceControllerOptions...)...,
		)
		if err != nil {
			return fmt.Errorf("starting namespaces controller: %w", err)
		}
		s.CtlrNamespaces = ctlr
	}
	if opt.runPortalController {
		ctlr, err := controller.Start(
			ctx,
			s.Conn,
			controller.WithFor(hz.Portal{}),
			controller.WithValidator(&hz.ValidateNamespaceRoot{}),
		)
		if err != nil {
			return fmt.Errorf("starting portal controller: %w", err)
		}
		s.CltrPortals = ctlr
	}

	if opt.devMode {
		userConfig, err := jwt.FormatUserConfig(
			s.NS.Auth.RootUser.JWT,
			[]byte(s.NS.Auth.RootUser.Seed),
		)
		if err != nil {
			return fmt.Errorf("formatting user config: %w", err)
		}
		fmt.Println(`
 _                _
| |__   ___  _ __(_)_______  _ __
| '_ \ / _ \| '__| |_  / _ \| '_ \
| | | | (_) | |  | |/ / (_) | | | |
|_| |_|\___/|_|  |_/___\___/|_| |_|
    _                         _
 __| |_____ __  _ __  ___  __| |___
/ _` + "`" + ` / -_) V / | '  \/ _ \/ _` + "`" + ` / -_)
\__,_\___|\_/  |_|_|_\___/\__,_\___|
		`)

		fmt.Println("Below is a NATS credential for the root NATS account.")
		fmt.Println("Copy it to a file such as \"nats.creds\"")

		fmt.Println("")
		fmt.Println("")
		fmt.Println(string(userConfig))
		fmt.Println("")
		fmt.Println("")
	}

	return nil
}

func (s *Server) Close() error {
	var errs error
	if s.CltrPortals != nil {
		if err := s.CltrPortals.Stop(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.CtlrNamespaces != nil {
		if err := s.CtlrNamespaces.Stop(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.Gateway != nil {
		if err := s.Gateway.Close(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.Broker != nil {
		if err := s.Broker.Stop(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.Store != nil {
		if err := s.Store.Close(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.Auth != nil {
		if err := s.Auth.Close(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.NS != nil {
		if err := s.NS.Close(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}
