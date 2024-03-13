package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/broker"
	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/extensions/secrets"
	"github.com/verifa/horizon/pkg/extensions/serviceaccounts"
	"github.com/verifa/horizon/pkg/gateway"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/natsutil"
	"github.com/verifa/horizon/pkg/store"
)

func WithDevMode() ServerOption {
	return func(o *serverOptions) {
		o.devMode = true

		o.runNATSServer = true
		o.runAuth = true
		o.runBroker = true
		o.runStore = true
		o.runGateway = true

		o.runSecretsController = true
		o.runAccountsController = true
		o.runServiceAccountsController = true
		o.runUsersActor = true
		o.runPortalController = true
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

func WithRunAccountsController(b bool) ServerOption {
	return func(o *serverOptions) {
		o.runAccountsController = b
	}
}

func WithAccountsControllerOptions(opts ...hz.ControllerOption) ServerOption {
	return func(o *serverOptions) {
		o.runAccountsController = true
		o.accountsControllerOptions = append(
			o.accountsControllerOptions,
			opts...)
	}
}

func WithRunUsersActor(b bool) ServerOption {
	return func(o *serverOptions) {
		o.runUsersActor = b
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

	runSecretsController         bool
	runAccountsController        bool
	runServiceAccountsController bool
	runUsersActor                bool
	runPortalController          bool

	natsOptions               []natsutil.ServerOption
	authOptions               []auth.Option
	storeOptions              []store.StoreOption
	gatewayOptions            []gateway.ServerOption
	accountsControllerOptions []hz.ControllerOption
}

type Server struct {
	Conn *nats.Conn

	NS      *natsutil.Server
	Auth    *auth.Auth
	Broker  *broker.Broker
	Store   *store.Store
	Gateway *gateway.Server

	CtlrSecrets         *hz.Controller
	CtlrAccounts        *hz.Controller
	CtlrServiceAccounts *hz.Controller
	CltrPortals         *hz.Controller
	ActorUsers          *hz.Actor[accounts.User]
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
	opt := serverOptions{}
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
		if err := ts.PublishRootAccount(); err != nil {
			return fmt.Errorf("publishing root horizon account: %w", err)
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

	if opt.runStore {
		store, err := store.StartStore(ctx, s.Conn, s.Auth, opt.storeOptions...)
		if err != nil {
			return fmt.Errorf("starting store: %w", err)
		}
		s.Store = store
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
		defaultOptions := []gateway.ServerOption{
			gateway.WithDummyAuthDefault(true),
		}
		if opt.gatewayOptions == nil {
			opt.gatewayOptions = defaultOptions
		}
		gw, err := gateway.Start(ctx, s.Conn, s.Auth, opt.gatewayOptions...)
		if err != nil {
			return fmt.Errorf("starting gateway: %w", err)
		}
		s.Gateway = gw
	}

	if opt.runSecretsController {
		ctlr, err := hz.StartController(
			ctx,
			s.Conn,
			hz.WithControllerFor(secrets.Secret{}),
		)
		if err != nil {
			return fmt.Errorf("starting secrets controller: %w", err)
		}
		s.CtlrSecrets = ctlr
	}
	if opt.runAccountsController {
		recon := accounts.AccountReconciler{
			Client: hz.NewClient(
				s.Conn,
				hz.WithClientInternal(true),
				hz.WithClientManager("ctlr-accounts"),
			),
			Conn:              s.Conn,
			OpKeyPair:         s.NS.Auth.Operator.SigningKey.KeyPair,
			RootAccountPubKey: s.NS.Auth.RootAccount.PublicKey,
		}
		defaultOptions := []hz.ControllerOption{
			hz.WithControllerFor(accounts.Account{}),
			hz.WithControllerReconciler(&recon),
		}

		ctlr, err := hz.StartController(
			ctx,
			s.Conn,
			append(defaultOptions, opt.accountsControllerOptions...)...,
		)
		if err != nil {
			return fmt.Errorf("starting accounts controller: %w", err)
		}
		s.CtlrAccounts = ctlr
	}
	if opt.runServiceAccountsController {
		recon := serviceaccounts.Reconciler{
			Client: hz.NewClient(
				s.Conn,
				hz.WithClientInternal(true),
				hz.WithClientManager(serviceaccounts.FieldManager),
			),
		}
		ctlr, err := hz.StartController(
			ctx,
			s.Conn,
			hz.WithControllerFor(serviceaccounts.ServiceAccount{}),
			hz.WithControllerReconciler(&recon),
		)
		if err != nil {
			return fmt.Errorf("starting service accounts controller: %w", err)
		}
		s.CtlrServiceAccounts = ctlr
	}

	if opt.runUsersActor {
		userCreateAction := accounts.UserCreateAction{
			Client: hz.NewClient(
				s.Conn,
				hz.WithClientInternal(true),
			),
		}
		userActor, err := hz.StartActor(
			ctx,
			s.Conn,
			hz.WithActorActioner(&userCreateAction),
		)
		if err != nil {
			return fmt.Errorf("starting user actor: %w", err)
		}
		s.ActorUsers = userActor
	}

	if opt.runPortalController {
		ctlr, err := hz.StartController(
			ctx,
			s.Conn,
			hz.WithControllerFor(hz.Portal{}),
		)
		if err != nil {
			return fmt.Errorf("starting portal controller: %w", err)
		}
		s.CltrPortals = ctlr
	}

	// Check that the root account exists as an object.
	// This is a little bit fidgety, because the root account *will* exist in
	// NATS, but we want it to exist as a horizon object in the store.
	// We cannot create the horizon object when we create the account in nats
	// because we would need the store to run, which cannot run until the root
	// account exists in nats...
	// For now, create it here but when we split the server out we'll need to
	// find a good startup process.
	if err := s.checkRootAccountObject(ctx, s.NS.Auth); err != nil {
		return fmt.Errorf("checking root account object: %w", err)
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

		fmt.Println("Below is a NATS credential for the root account.")
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
	if s.CtlrAccounts != nil {
		if err := s.CtlrAccounts.Stop(); err != nil {
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

func (s *Server) checkRootAccountObject(
	ctx context.Context,
	jwtAuth natsutil.ServerJWTAuth,
) error {
	accClient := hz.ObjectClient[accounts.Account]{
		Client: hz.NewClient(s.Conn, hz.WithClientInternal(true)),
	}
	if _, err := accClient.Get(
		ctx,
		hz.WithGetKey(hz.ObjectKey{
			Name:    hz.RootAccount,
			Account: hz.RootAccount,
		}),
	); err != nil {
		if !errors.Is(err, hz.ErrNotFound) {
			return fmt.Errorf("get root account: %w", err)
		}
		fmt.Println("Checking root account object: not found, creating...")
		// If the root account is not found, we need to create it.
		if err := accClient.Create(ctx, accounts.Account{
			ObjectMeta: hz.ObjectMeta{
				Name:    hz.RootAccount,
				Account: hz.RootAccount,
			},
			Spec: &accounts.AccountSpec{},
			Status: &accounts.AccountStatus{
				Ready:          true,
				ID:             jwtAuth.RootAccount.PublicKey,
				Seed:           jwtAuth.RootAccount.Seed,
				SigningKeySeed: jwtAuth.RootAccount.SigningKey.Seed,
				JWT:            jwtAuth.RootAccount.JWT,
			},
		}); err != nil {
			return fmt.Errorf("create root account: %w", err)
		}
	}

	return nil
}
