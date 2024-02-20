package gateway

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/hz"
)

const HeaderAccount = "Hz-Account"

func (s *Server) servePortal(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := r.Context().Value(authContext).(auth.UserInfo)
	if !ok {
		http.Error(w, "no auth context", http.StatusUnauthorized)
		return
	}
	account := chi.URLParam(r, "account")
	portal := chi.URLParam(r, "portal")
	subPath := chi.URLParam(r, "*")

	body := accountLayout(
		account,
		s.portals,
		actorProxyPage(account, portal, subPath),
	)
	layout(portal, &userInfo, body).Render(r.Context(), w)
}

func (s *Server) handlePortal(w http.ResponseWriter, r *http.Request) {
	_, ok := r.Context().Value(authContext).(auth.UserInfo)
	if !ok {
		http.Error(w, "no auth context", http.StatusUnauthorized)
		return
	}

	account := chi.URLParam(r, "account")
	portal := chi.URLParam(r, "portal")
	proxy := httputil.ReverseProxy{}
	proxy.Rewrite = func(req *httputil.ProxyRequest) {
		// Remove prefix from the request URL.
		prefix := fmt.Sprintf("/portal/%s/%s", account, portal)
		req.Out.URL.Path = strings.TrimPrefix(req.Out.URL.Path, prefix)
		req.SetXForwarded()
	}
	proxy.Transport = &NATSHTTPTransport{
		conn:    s.Conn,
		subject: fmt.Sprintf(hz.ActorSubjectHTTPRender, portal),
		account: account,
	}
	proxy.ServeHTTP(w, r)
}

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
	r.Header.Add(HeaderAccount, t.account)
	reqBuf := bytes.Buffer{}
	if err := r.Write(&reqBuf); err != nil {
		return nil, fmt.Errorf("transport writing request: %w", err)
	}

	reply, err := t.conn.Request(
		t.subject,
		reqBuf.Bytes(),
		time.Second,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"transport nats request: subject %s: %w",
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
