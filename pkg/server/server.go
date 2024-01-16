package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
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
		o.runBroker = true
		o.runStore = true
		o.runGateway = true
		o.runAccountsController = true
		o.runUsersController = true
		o.runUsersActor = true
	}
}

func WithNATSConn(conn *nats.Conn) ServerOption {
	return func(o *serverOptions) {
		o.conn = conn
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
	runBroker             bool
	runStore              bool
	runGateway            bool
	runAccountsController bool
	runUsersController    bool
	runUsersActor         bool

	natsOptions               []natsutil.ServerOption
	storeOptions              []store.StoreOption
	gatewayOptions            []gateway.ServerOption
	accountsControllerOptions []hz.ControllerOption
	usersControllerOptions    []hz.ControllerOption
}

type Server struct {
	conn *nats.Conn

	nats         *natsutil.Server
	broker       *broker.Broker
	store        *store.Store
	gw           *gateway.Server
	ctlrAccounts *hz.Controller
	ctlrUsers    *hz.Controller
	actorUsers   *hz.Actor[accounts.User]
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
		s.conn = opt.conn
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
		s.nats = &ts
		s.conn = conn
	}

	if s.conn == nil {
		return errors.New("nats connection required")
	}

	if opt.runStore {
		store, err := store.StartStore(ctx, s.conn, opt.storeOptions...)
		if err != nil {
			return fmt.Errorf("starting store: %w", err)
		}
		s.store = store
	}
	if opt.runBroker {
		broker := broker.Broker{
			Conn: s.conn,
		}
		if err := broker.Start(ctx); err != nil {
			return fmt.Errorf("starting broker: %w", err)
		}
		s.broker = &broker
	}
	if opt.runGateway {
		defaultOptions := []gateway.ServerOption{
			gateway.WithDummyAuthDefault(true),
		}
		if opt.gatewayOptions == nil {
			opt.gatewayOptions = defaultOptions
		}
		gw, err := gateway.Start(ctx, s.conn, opt.gatewayOptions...)
		if err != nil {
			return fmt.Errorf("starting gateway: %w", err)
		}
		s.gw = gw
	}
	if opt.runAccountsController {
		recon := accounts.AccountReconciler{
			Client:            hz.Client{Conn: s.conn},
			Conn:              s.conn,
			OpKeyPair:         s.nats.Auth.Operator.SigningKey.KeyPair,
			RootAccountPubKey: s.nats.Auth.RootAccount.PublicKey,
		}
		defaultOptions := []hz.ControllerOption{
			hz.WithControllerFor(accounts.Account{}),
			hz.WithControllerReconciler(&recon),
		}

		ctlr, err := hz.StartController(
			ctx,
			s.conn,
			append(defaultOptions, opt.accountsControllerOptions...)...,
		)
		if err != nil {
			return fmt.Errorf("starting accounts controller: %w", err)
		}
		s.ctlrAccounts = ctlr
	}

	if opt.runUsersController {
		defaultOptions := []hz.ControllerOption{
			hz.WithControllerFor(accounts.User{}),
		}
		ctlr, err := hz.StartController(
			ctx,
			s.conn,
			append(defaultOptions, opt.usersControllerOptions...)...,
		)
		if err != nil {
			return fmt.Errorf("starting users controller: %w", err)
		}
		s.ctlrUsers = ctlr
	}
	if opt.runUsersActor {
		userCreateAction := accounts.UserCreateAction{
			Client: hz.Client{Conn: s.conn},
		}
		userActor, err := hz.StartActor(
			ctx,
			s.conn,
			hz.WithActorActioner(&userCreateAction),
		)
		if err != nil {
			return fmt.Errorf("starting user actor: %w", err)
		}
		s.actorUsers = userActor
	}

	return nil
}

func (s *Server) Close() error {
	var errs error
	if s.ctlrAccounts != nil {
		if err := s.ctlrAccounts.Stop(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.ctlrUsers != nil {
		if err := s.ctlrUsers.Stop(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.gw != nil {
		if err := s.gw.Close(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.broker != nil {
		if err := s.broker.Stop(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.store != nil {
		if err := s.store.Close(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.nats != nil {
		s.nats.NS.Shutdown()
		s.nats.NS.WaitForShutdown()
	}
	return errs
}

func (s *Server) Services() []string {
	services := []string{}
	if s.gw != nil {
		services = append(services, "gateway")
	}
	if s.broker != nil {
		services = append(services, "broker")
	}
	if s.store != nil {
		services = append(services, "store")
	}
	if s.nats != nil {
		services = append(services, "nats")
	}
	if s.ctlrAccounts != nil {
		services = append(services, "ctlr-accounts")
	}
	if s.ctlrUsers != nil {
		services = append(services, "ctlr-users")
	}
	if s.actorUsers != nil {
		services = append(services, "actor-users")
	}
	return services
}
