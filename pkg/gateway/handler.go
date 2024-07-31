package gateway

import (
	"net/http"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/extensions/core"
	"github.com/verifa/horizon/pkg/hz"
)

type GatewayHandler interface {
	GetHome(w http.ResponseWriter, r *http.Request)
	GetNamespaces(w http.ResponseWriter, r *http.Request)
	GetNamespacesNew(w http.ResponseWriter, r *http.Request)
	PostNamespaces(w http.ResponseWriter, r *http.Request)
}

var _ GatewayHandler = (*DefaultHandler)(nil)

type DefaultHandler struct {
	Conn *nats.Conn
}

func (d *DefaultHandler) GetHome(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := r.Context().Value(authContext).(auth.UserInfo)
	if !ok {
		http.Error(w, "no auth context", http.StatusUnauthorized)
		return
	}
	_ = layout("Home", &userInfo, home()).Render(r.Context(), w)
}

// GetNamespaces implements GatewayHandler.
func (d *DefaultHandler) GetNamespaces(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := r.Context().Value(authContext).(auth.UserInfo)
	if !ok {
		http.Error(w, "no auth context", http.StatusUnauthorized)
		return
	}
	client := hz.Client{
		Conn:    d.Conn,
		Session: hz.SessionFromRequest(r),
	}
	nsClient := hz.ObjectClient[core.Namespace]{Client: client}
	namespaces, err := nsClient.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body := namespacesPage(namespaces)
	_ = layout("Namespaces", &userInfo, body).Render(r.Context(), w)
}

// GetNamespacesNew implements GatewayHandler.
func (d *DefaultHandler) GetNamespacesNew(
	w http.ResponseWriter,
	r *http.Request,
) {
	userInfo, ok := r.Context().Value(authContext).(auth.UserInfo)
	if !ok {
		http.Error(w, "no auth context", http.StatusUnauthorized)
		return
	}
	body := namespacesNewPage()
	_ = layout("New Namespace", &userInfo, body).Render(r.Context(), w)
}

// PostNamespaces implements GatewayHandler.
func (d *DefaultHandler) PostNamespaces(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.FormValue("namespace-name")
	client := hz.Client{
		Conn:    d.Conn,
		Session: hz.SessionFromRequest(r),
	}
	nsClient := hz.ObjectClient[core.Namespace]{Client: client}
	ns := core.Namespace{
		ObjectMeta: hz.ObjectMeta{
			Name:      name,
			Namespace: hz.RootNamespace,
		},
	}
	err := nsClient.Create(r.Context(), ns)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Add("HX-Redirect", "/namespaces/"+name)
}
