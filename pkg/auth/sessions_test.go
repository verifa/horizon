package auth

import (
	"context"
	"testing"

	"github.com/verifa/horizon/pkg/natsutil"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestSessions(t *testing.T) {
	s, err := natsutil.NewServer(
		natsutil.WithDir(t.TempDir()),
		natsutil.WithFindAvailablePort(true),
	)
	tu.AssertNoError(t, err)

	err = s.StartUntilReady()
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		s.NS.Shutdown()
		s.NS.WaitForShutdown()
	})

	err = s.PublishRootNamespace()
	tu.AssertNoError(t, err)

	conn, err := s.RootUserConn()
	tu.AssertNoError(t, err)

	ctx := context.Background()
	sessions := Sessions{
		Conn: conn,
	}
	err = sessions.Start(ctx)
	tu.AssertNoError(t, err)

	claims := UserInfo{
		Sub: "test",
		Iss: "test",
	}
	sessionID, err := sessions.New(ctx, claims)
	tu.AssertNoError(t, err)

	userInfo, err := sessions.Get(ctx, sessionID)
	tu.AssertNoError(t, err)
	tu.AssertEqual(t, claims, userInfo)

	err = sessions.Delete(ctx, sessionID)
	tu.AssertNoError(t, err)

	_, err = sessions.Get(ctx, sessionID)
	tu.AssertErrorIs(t, err, ErrInvalidCredentials)
}
