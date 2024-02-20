package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/broker"
	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/gateway"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/natsutil"
	"github.com/verifa/horizon/pkg/store"
)

func WithDevMode() ServerOption {
	return func(o *serverOptions) {
		o.runNATSServer = true
		o.runAuth = true
		o.runBroker = true
		o.runStore = true
		o.runGateway = true

		o.runAccountsController = true
		o.runUsersController = true
		o.runUsersActor = true
		o.runGroupsController = true
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

func WithRunUsersController(b bool) ServerOption {
	return func(o *serverOptions) {
		o.runUsersController = b
	}
}

func WithUsersControllerOptions(opts ...hz.ControllerOption) ServerOption {
	return func(o *serverOptions) {
		o.runUsersController = true
		o.usersControllerOptions = append(
			o.usersControllerOptions,
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

	runNATSServer         bool
	runAuth               bool
	runBroker             bool
	runStore              bool
	runGateway            bool
	runAccountsController bool
	runUsersController    bool
	runUsersActor         bool
	runGroupsController   bool

	natsOptions               []natsutil.ServerOption
	storeOptions              []store.StoreOption
	gatewayOptions            []gateway.ServerOption
	accountsControllerOptions []hz.ControllerOption
	usersControllerOptions    []hz.ControllerOption
}

type Server struct {
	Conn *nats.Conn

	NS           *natsutil.Server
	Auth         *auth.Auth
	Broker       *broker.Broker
	Store        *store.Store
	Gateway      *gateway.Server
	CtlrAccounts *hz.Controller
	CtlrUsers    *hz.Controller
	ActorUsers   *hz.Actor[accounts.User]
}

func New(
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
			return fmt.Errorf("starting test server: %w", err)
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
		auth, err := auth.Start(ctx, s.Conn)
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
	if opt.runAccountsController {
		recon := accounts.AccountReconciler{
			Client:            hz.InternalClient(s.Conn),
			Conn:              s.Conn,
			OpKeyPair:         s.NS.Auth.Operator.SigningKey.KeyPair,
			RootAccountPubKey: s.NS.Auth.RootAccount.PublicKey,
		}
		defaultOptions := []hz.ControllerOption{
			hz.WithControllerFor(accounts.Account{}),
			hz.WithControllerReconciler(&recon),
			hz.WithControllerValidatorCUE(),
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

	if opt.runUsersController {
		defaultOptions := []hz.ControllerOption{
			hz.WithControllerFor(accounts.User{}),
			hz.WithControllerValidatorCUE(),
		}
		ctlr, err := hz.StartController(
			ctx,
			s.Conn,
			append(defaultOptions, opt.usersControllerOptions...)...,
		)
		if err != nil {
			return fmt.Errorf("starting users controller: %w", err)
		}
		s.CtlrUsers = ctlr
	}
	if opt.runUsersActor {
		userCreateAction := accounts.UserCreateAction{
			Client: hz.InternalClient(s.Conn),
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
	if opt.runGroupsController {
		ctlr, err := hz.StartController(
			ctx,
			s.Conn,
			hz.WithControllerFor(accounts.Group{}),
			hz.WithControllerValidatorCUE(),
		)
		if err != nil {
			return fmt.Errorf("starting groups controller: %w", err)
		}
		s.CtlrUsers = ctlr
	}

	return nil
}

func (s *Server) Close() error {
	var errs error
	if s.CtlrAccounts != nil {
		if err := s.CtlrAccounts.Stop(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.CtlrUsers != nil {
		if err := s.CtlrUsers.Stop(); err != nil {
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
	if s.NS != nil {
		s.NS.NS.Shutdown()
		s.NS.NS.WaitForShutdown()
	}
	return errs
}

func (s *Server) Services() []string {
	services := []string{}
	if s.Gateway != nil {
		services = append(services, "gateway")
	}
	if s.Broker != nil {
		services = append(services, "broker")
	}
	if s.Store != nil {
		services = append(services, "store")
	}
	if s.NS != nil {
		services = append(services, "nats")
	}
	if s.CtlrAccounts != nil {
		services = append(services, "ctlr-accounts")
	}
	if s.CtlrUsers != nil {
		services = append(services, "ctlr-users")
	}
	if s.ActorUsers != nil {
		services = append(services, "actor-users")
	}
	return services
}
