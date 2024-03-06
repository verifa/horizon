package gateway

import (
	_ "embed"
	"fmt"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/hz"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
)

type AccountsHandler struct {
	Middleware chi.Middlewares
	Conn       *nats.Conn
	Auth       *auth.Auth
	Portals    map[string]hz.Portal
}

func (h *AccountsHandler) Router() *chi.Mux {
	r := chi.NewRouter()
	r.Use(h.Middleware...)
	r.Use(h.middlewareAccount)
	r.Get("/", h.getAccount)
	r.Get("/users", h.serveAccountUsers)
	r.Post("/userconfig", h.postAccountUserConfig)
	r.HandleFunc("/{portal}", h.servePortal)
	r.HandleFunc("/{portal}/*", h.servePortal)
	return r
}

func (h *AccountsHandler) middlewareAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account := chi.URLParam(r, "account")
		if account == "" {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}
		ok, err := h.Auth.Check(r.Context(), auth.CheckRequest{
			Session: hz.SessionFromRequest(r),
			Verb:    auth.VerbRead,
			Object: hz.ObjectKey{
				Group:   accounts.ObjectGroup,
				Kind:    accounts.ObjectKind,
				Account: hz.RootAccount,
				Name:    account,
			},
		})
		if err != nil {
			httpError(w, err)
			return
		}
		if !ok {
			// TODO: display a prettty 403 page instead.
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		client := hz.Client{
			Conn:    h.Conn,
			Session: hz.SessionFromRequest(r),
		}
		if _, err := client.Get(r.Context(), hz.WithGetKey(hz.ObjectKey{
			Group:   accounts.ObjectGroup,
			Version: accounts.ObjectVersion,
			Kind:    accounts.ObjectKind,
			Account: hz.RootAccount,
			Name:    account,
		})); err != nil {
			// TODO: display a pretty 404 page instead.
			httpError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(r.Context()))
	})
}

func (h *AccountsHandler) getAccount(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := r.Context().Value(authContext).(auth.UserInfo)
	if !ok {
		http.Error(w, "no auth context", http.StatusUnauthorized)
		return
	}
	account := chi.URLParam(r, "account")
	body := accountLayout(account, h.Portals, accountPage())
	layout("Account", &userInfo, body).Render(r.Context(), w)
}

func (h *AccountsHandler) serveAccountUsers(
	w http.ResponseWriter,
	r *http.Request,
) {
	userInfo, ok := r.Context().Value(authContext).(auth.UserInfo)
	if !ok {
		http.Error(w, "no auth context", http.StatusUnauthorized)
		return
	}
	account := chi.URLParam(r, "account")
	body := accountLayout(account, h.Portals, accountUsersPage(account))
	layout("Users", &userInfo, body).Render(r.Context(), w)
}

func (h *AccountsHandler) postAccountUserConfig(
	w http.ResponseWriter,
	r *http.Request,
) {
	account := chi.URLParam(r, "account")
	name := r.FormValue("user-name")
	user := accounts.User{
		ObjectMeta: hz.ObjectMeta{
			Name:    name,
			Account: account,
		},
	}
	client := hz.Client{
		Conn:    h.Conn,
		Session: hz.SessionFromRequest(r),
	}
	userClient := hz.ObjectClient[accounts.User]{Client: client}
	reply, err := userClient.Run(
		r.Context(),
		&accounts.UserCreateAction{},
		user,
	)
	if err != nil {
		httpError(w, err)
		return
	}
	creds, err := jwt.FormatUserConfig(
		reply.Status.JWT,
		[]byte(reply.Status.Seed),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(creds)
}

func (h *AccountsHandler) servePortal(
	w http.ResponseWriter,
	r *http.Request,
) {
	userInfo, ok := r.Context().Value(authContext).(auth.UserInfo)
	if !ok {
		http.Error(w, "no auth context", http.StatusUnauthorized)
		return
	}

	account := chi.URLParam(r, "account")
	portal := chi.URLParam(r, "portal")
	subpath := chi.URLParam(r, "*")

	// If the request accepts text/event-stream it is an SSE connection request.
	// SSE connection requests should be handled by the portal.
	isEventStream := r.Header.Get("Accept") == "text/event-stream"
	// If the request is an HX request, it should be handled by the portal.
	isHXRequest := r.Header.Get("HX-Request") == "true"
	isHZPortalLoad := r.Header.Get("HZ-Portal-Load-Request") == "true"

	if isHXRequest || isEventStream {
		if isHZPortalLoad {
			r.Header.Del("HX-Request")
			r.Header.Del("HZ-Portal-Load-Request")
		}
		proxy := httputil.ReverseProxy{}
		proxy.Rewrite = func(req *httputil.ProxyRequest) {
			// Remove prefix from the request URL.
			prefix := fmt.Sprintf("/accounts/%s/%s", account, portal)
			req.Out.URL.Path = strings.TrimPrefix(req.Out.URL.Path, prefix)
			req.Out.Header.Set(hz.RequestAccount, account)
			req.Out.Header.Set(hz.RequestPortal, portal)
			req.SetXForwarded()
		}
		proxy.Transport = &NATSHTTPTransport{
			conn:    h.Conn,
			subject: fmt.Sprintf(hz.SubjectPortalRender, portal),
			account: account,
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			// NOTE: this only handles errors returned from the proxy.
			// I.e. if an HTTP response is received, then it is not considered
			// an error.
			w.WriteHeader(http.StatusOK)
			_ = portalError(err).Render(r.Context(), w)
		}
		// This is one idea to handle errors returned from portals.
		// Ideally portals should only return 2xx status codes, as per the
		// HATEOS way of handling things.
		// https://htmx.org/essays/hateoas/
		// proxy.ModifyResponse = func(resp *http.Response) error {
		// 	// Modify the response if the status code is not 2xx.
		// 	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 		// Modify the response here to be 2xx for HTMX to render it.
		// 	}
		// 	return nil
		// }
		proxy.ServeHTTP(w, r)
		return
	}
	body := accountLayout(
		account,
		h.Portals,
		portalProxy(account, portal, subpath),
	)
	layout(portal, &userInfo, body).Render(r.Context(), w)
}
