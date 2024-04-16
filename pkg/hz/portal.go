package hz

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nats-io/nats.go"
)

const (
	RequestPortal  = "HZ-Request-Portal"
	RequestAccount = "HZ-Request-Account"
)

var _ Objecter = (*Portal)(nil)

type Portal struct {
	ObjectMeta `json:"metadata,omitempty" cue:""`

	Spec   *PortalSpec   `json:"spec,omitempty" cue:""`
	Status *PortalStatus `json:"status,omitempty" cue:",opt"`
}

func (e Portal) ObjectVersion() string {
	return "v1"
}

func (e Portal) ObjectGroup() string {
	return "core"
}

func (e Portal) ObjectKind() string {
	return "Portal"
}

type PortalSpec struct {
	DisplayName string `json:"displayName,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

type PortalStatus struct{}

func StartPortal(
	ctx context.Context,
	nc *nats.Conn,
	ext Portal,
	handler http.Handler,
) (*PortalHandler, error) {
	e := PortalHandler{
		conn:    nc,
		ext:     ext,
		handler: handler,
	}

	if ext.ObjectName() == "" {
		return nil, fmt.Errorf("extension name is required")
	}

	if err := e.Start(ctx); err != nil {
		return nil, fmt.Errorf("starting extension: %w", err)
	}

	return &e, nil
}

// PortalHandler is a NATS to HTTP proxy handler for a portal extension.
// It subscribes to NATS and proxies the requests to the given handler.
type PortalHandler struct {
	conn    *nats.Conn
	ext     Portal
	handler http.Handler

	sub *nats.Subscription
}

// Start registers the portal with Horizon so that it is available in the UI,
// and then subscribes to NATS to handle the requests.
func (e *PortalHandler) Start(ctx context.Context) error {
	client := NewClient(
		e.conn,
		WithClientInternal(true),
		WithClientDefaultManager(),
	)
	extClient := ObjectClient[Portal]{Client: client}
	// TODO: field manager.
	applyResult, err := extClient.Apply(ctx, e.ext)
	if err != nil {
		return fmt.Errorf("putting extension: %w", err)
	}
	switch applyResult {
	case ApplyOpResultCreated:
		slog.Info("created portal", "name", e.ext.Name)
	case ApplyOpResultUpdated:
		slog.Info("updated portal", "name", e.ext.Name)
	}

	// Subscribe to nats to receive http requests and proxy them to the handler.
	subject := fmt.Sprintf(SubjectPortalRender, e.ext.Name)
	sub, err := e.conn.QueueSubscribe(
		subject,
		e.ext.Name,
		func(msg *nats.Msg) {
			req, err := http.ReadRequest(
				bufio.NewReader(strings.NewReader(string(msg.Data))),
			)
			if err != nil {
				slog.Error("reading request", "error", err)
				resp := http.Response{
					StatusCode: http.StatusBadRequest,
				}
				var buf bytes.Buffer
				if err := resp.Write(&buf); err != nil {
					slog.Error("writing response", "error", err)
					return
				}
				if err := msg.Respond(buf.Bytes()); err != nil {
					slog.Error("responding", "error", err)
					return
				}
			}
			outReq := req.WithContext(ctx)
			rr := NATSResponseWriter{
				conn:    e.conn,
				subject: msg.Reply,
			}
			go func() {
				defer rr.Flush()
				e.handler.ServeHTTP(&rr, outReq)
			}()
		},
	)
	if err != nil {
		return fmt.Errorf("subscribing: %w", err)
	}
	e.sub = sub
	return nil
}

func (e *PortalHandler) Stop() error {
	return e.sub.Unsubscribe()
}
