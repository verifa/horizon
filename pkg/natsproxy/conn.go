package natsproxy

import (
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	HeaderFin = "Hz-Connection-Fin"
)

var (
	StatusOK    []byte = []uint8{0}
	StatusError []byte = []uint8{1}
)

func isStatusOK(b []byte) bool {
	return len(b) == 1 && b[0] == 0
}

var _ (net.Conn) = (*Conn)(nil)

type Conn struct {
	Conn    *nats.Conn
	Subject string

	msgCh chan *nats.Msg
	sub   *nats.Subscription
	inbox string
}

func (c *Conn) Start() error {
	inbox := nats.NewInbox()
	msgCh := make(chan *nats.Msg, 100)
	sub, err := c.Conn.Subscribe(inbox, func(msg *nats.Msg) {
		// Handle different cases here.
		if msg.Header.Get(HeaderFin) != "" {
			_ = c.Close()
			return
		}
		msgCh <- msg
	})
	if err != nil {
		return err
	}

	c.msgCh = msgCh
	c.inbox = inbox
	c.sub = sub

	return nil
}

// Close implements net.Conn.
func (c *Conn) Close() error {
	var errs error
	if err := c.sub.Unsubscribe(); err != nil {
		errs = errors.Join(errs, fmt.Errorf("unsubscribing: %w", err))
	}
	close(c.msgCh)
	// TODO: do we notify the other side that we're closing?
	return errs
}

// Read implements net.Conn.
func (c *Conn) Read(b []byte) (int, error) {
	msg, ok := <-c.msgCh
	if !ok {
		// Channel is closed, so we are done.
		return 0, io.EOF
	}
	n := copy(b, msg.Data)
	if n < len(msg.Data) {
		// TODO: maybe we want to create some kind of buffer?
		// Let's wait and see if this becomes a problem...
		if err := msg.Respond(StatusError); err != nil {
			return 0, fmt.Errorf("responding to nats message: %w", err)
		}
		return n, io.ErrShortBuffer
	}
	if err := msg.Respond(StatusOK); err != nil {
		return 0, fmt.Errorf("responding to nats message: %w", err)
	}
	return n, nil
}

// Write implements net.Conn.
func (c *Conn) Write(b []byte) (int, error) {
	msg := nats.NewMsg(c.Subject)
	msg.Reply = c.inbox
	msg.Data = b
	if err := c.Conn.PublishMsg(msg); err != nil {
		return 0, fmt.Errorf("nats write: %w", err)
	}
	reply, err := c.Conn.RequestMsg(msg, time.Second)
	if err != nil {
		return 0, fmt.Errorf(
			"nats conn request to %q: %w",
			msg.Subject,
			err,
		)
	}
	// TODO: if status is not OK, what do we do?!
	if !isStatusOK(reply.Data) {
		return 0, fmt.Errorf("nats write: %s", reply.Data)
	}
	return len(msg.Data), nil
}

// LocalAddr implements net.Conn.
func (c *Conn) LocalAddr() net.Addr {
	panic("unimplemented")
}

// RemoteAddr implements net.Conn.
func (c *Conn) RemoteAddr() net.Addr {
	panic("unimplemented")
}

// SetDeadline implements net.Conn.
func (c *Conn) SetDeadline(t time.Time) error {
	panic("unimplemented")
}

// SetReadDeadline implements net.Conn.
func (c *Conn) SetReadDeadline(t time.Time) error {
	panic("unimplemented")
}

// SetWriteDeadline implements net.Conn.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	panic("unimplemented")
}
