package testserver

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/broker"
	"github.com/verifa/horizon/pkg/natsutil"
	"github.com/verifa/horizon/pkg/store"
	tu "github.com/verifa/horizon/pkg/testutil"
)

type Instance struct {
	natsutil.Server
	Conn  *nats.Conn
	Store *store.Store
}

type Config struct {
	NATSOptions  []natsutil.ServerOption
	StoreOptions []store.StoreOption
}

// New is a thin wrapper to make create a test server instance.
// It is only intended to be used in tests.
func New(t *testing.T, ctx context.Context, config *Config) Instance {
	if config == nil {
		config = &Config{}
	}
	// Default nats options.
	config.NATSOptions = append(
		[]natsutil.ServerOption{
			natsutil.WithDir(t.TempDir()),
			natsutil.WithFindAvailablePort(true),
		},
		config.NATSOptions...,
	)
	ts, err := natsutil.NewServer(
		config.NATSOptions...,
	)
	if err != nil {
		t.Fatal("creating nats server: ", err)
	}
	if err := ts.StartUntilReady(); err != nil {
		t.Fatal("starting test server: ", err)
	}
	t.Cleanup(func() {
		ts.NS.Shutdown()
	})
	if err := ts.PublishRootAccount(); err != nil {
		t.Fatal("publishing actor account: ", err)
	}

	ti := Instance{
		Server: ts,
		Conn:   RootUserConn(t, ts),
	}

	// Start store.
	store, err := store.StartStore(ctx, ti.Conn, config.StoreOptions...)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		store.Close()
	})
	ti.Store = store

	// Start broker.
	broker := broker.Broker{
		Conn: ti.Conn,
	}
	err = broker.Start(ctx)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		_ = broker.Stop()
	})

	return ti
}

func RootUserConn(t *testing.T, server natsutil.Server) *nats.Conn {
	conn, err := server.RootUserConn()
	tu.AssertNoError(t, err, "getting root user connection")
	t.Cleanup(conn.Close)
	return conn
}
