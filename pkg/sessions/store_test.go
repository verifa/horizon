package sessions

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/natsutil"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestStore(t *testing.T) {
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

	err = s.PublishRootAccount()
	tu.AssertNoError(t, err)

	conn, err := s.RootUserConn()
	tu.AssertNoError(t, err)

	ctx := context.Background()
	store, err := Start(ctx, conn)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})

	claims := UserInfo{
		Sub: "test",
		Iss: "test",
	}
	t.Run("with session id", func(t *testing.T) {
		sessionID, err := New(ctx, conn, claims)
		tu.AssertNoError(t, err)

		userInfo, err := Get(ctx, conn, WithSessionID(sessionID))
		tu.AssertNoError(t, err)
		tu.AssertEqual(t, claims, userInfo)

		err = Delete(ctx, conn, WithSessionID(sessionID))
		tu.AssertNoError(t, err)

		_, err = Get(ctx, conn, WithSessionID(sessionID))
		tu.AssertErrorIs(t, err, ErrInvalidCredentials)
	})

	t.Run("with message header", func(t *testing.T) {
		sessionID, err := New(ctx, conn, claims)
		tu.AssertNoError(t, err)

		fwdMsg := nats.NewMsg("whatever")
		fwdMsg.Header.Add(hz.HeaderAuthorization, sessionID)

		userInfo, err := Get(ctx, conn, WithSessionFromMsg(fwdMsg))
		tu.AssertNoError(t, err)
		tu.AssertEqual(t, claims, userInfo)

		err = Delete(ctx, conn, WithSessionFromMsg(fwdMsg))
		tu.AssertNoError(t, err)

		_, err = Get(ctx, conn, WithSessionFromMsg(fwdMsg))
		tu.AssertErrorIs(t, err, ErrInvalidCredentials)
	})
}
