package server

import (
	"context"
	"testing"

	"github.com/verifa/horizon/pkg/natsutil"
)

func Test(t *testing.T, ctx context.Context, opts ...ServerOption) *Server {
	t.Helper()
	// Default test options.
	opts = append(
		opts,
		WithDevMode(),
		WithNATSOptions(
			// Default nats options.
			natsutil.WithDir(t.TempDir()),
			natsutil.WithFindAvailablePort(true),
		),
	)
	s := Server{}
	if err := s.Start(ctx, opts...); err != nil {
		t.Fatalf("starting server: %v", err)
	}
	t.Cleanup(func() {
		err := s.Close()
		if err != nil {
			t.Fatalf("closing server: %v", err)
		}
	})
	return &s
}
