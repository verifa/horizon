package natsproxy

import (
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/natsutil"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestConn(t *testing.T) {
	ts, err := natsutil.NewServer(
		natsutil.WithDir(t.TempDir()),
		natsutil.WithFindAvailablePort(true),
	)
	tu.AssertNoError(t, err)
	err = ts.StartUntilReady()
	tu.AssertNoError(t, err)
	err = ts.PublishRootAccount()
	tu.AssertNoError(t, err)
	rootConn, err := ts.RootUserConn()
	tu.AssertNoError(t, err)

	subject := nats.NewInbox()
	conn := Conn{
		Conn:    rootConn,
		Subject: subject,
	}

	err = conn.Start()
	tu.AssertNoError(t, err)
}
