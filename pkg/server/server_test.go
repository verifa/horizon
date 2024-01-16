package server_test

import (
	"context"
	"testing"

	"github.com/verifa/horizon/pkg/natsutil"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestServerAll(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s, err := server.New(
		ctx,
		server.WithNATSOptions(
			natsutil.WithDir(t.TempDir()),
			natsutil.WithFindAvailablePort(true),
		),
		server.WithRunStore(true),
		server.WithRunBroker(true),
		server.WithRunGateway(true),
		server.WithRunAccountsController(true),
	)
	tu.AssertNoError(t, err)

	t.Cleanup(func() {
		err := s.Close()
		tu.AssertNoError(t, err)
	})

	// Run some tests.
}
