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

var _ Objecter = (*Portal)(nil)

type Portal struct {
	ObjectMeta `json:"metadata,omitempty"`

	Spec   PortalSpec   `json:"spec,omitempty"`
	Status PortalStatus `json:"status,omitempty"`
}

func (e Portal) ObjectAPIVersion() string {
	return "v1"
}

func (e Portal) ObjectGroup() string {
	return "hz-internal"
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
		nc:      nc,
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
	nc      *nats.Conn
	ext     Portal
	handler http.Handler

	sub *nats.Subscription
}

// Start registers the portal with Horizon so that it is available in the UI,
// and then subscribes to NATS to handle the requests.
func (e *PortalHandler) Start(ctx context.Context) error {
	client := NewClient(
		e.nc,
		WithClientManager("TODO"),
	)
	extClient := ObjectClient[Portal]{Client: client}
	// TODO: field manager.
	if err := extClient.Apply(ctx, e.ext); err != nil {
		return fmt.Errorf("putting extension: %w", err)
	}

	// Subscribe to nats to receive http requests and proxy them to the handler.
	subject := fmt.Sprintf(ActorSubjectHTTPRender, e.ext.Name)
	sub, err := e.nc.QueueSubscribe(
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
			rr := ResponseRecorder{}
			e.handler.ServeHTTP(&rr, req)
			bResp, err := rr.Response()
			if err != nil {
				slog.Error("getting response", "error", err)
				return
			}
			if err := msg.Respond(bResp); err != nil {
				slog.Error("responding", "error", err)
				return
			}
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
