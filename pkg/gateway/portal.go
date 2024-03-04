package gateway

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
)

const HeaderAccount = "Hz-Account"

var _ http.RoundTripper = (*NATSHTTPTransport)(nil)

// NATSHTTPTransport is an http.RoundTripper that transports HTTP requests
// over NATS.
//
// It is used together with httputil.ReverseProxy to create an HTTP
// reverse proxy that transports requests over NATS.
type NATSHTTPTransport struct {
	conn    *nats.Conn
	subject string
	account string
}

func (t *NATSHTTPTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	reqBuf := bytes.Buffer{}
	if err := r.Write(&reqBuf); err != nil {
		return nil, fmt.Errorf("transport writing request: %w", err)
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	reply, err := t.conn.RequestWithContext(
		ctx,
		t.subject,
		reqBuf.Bytes(),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"transport nats request: subject %q: %w",
			t.subject,
			err,
		)
	}
	resp, err := http.ReadResponse(
		bufio.NewReader(bytes.NewBuffer(reply.Data)),
		r,
	)
	if err != nil {
		return nil, fmt.Errorf("transport reading response: %w", err)
	}
	return resp, nil
}
