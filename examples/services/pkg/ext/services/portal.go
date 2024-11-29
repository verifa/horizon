package services

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v2"
	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/gateway"
	"github.com/verifa/horizon/pkg/hz"
	cloudrun "google.golang.org/api/run/v1"
)

const extName = "services"

var Portal = hz.Portal{
	ObjectMeta: hz.ObjectMeta{
		Namespace: hz.NamespaceRoot,
		Name:      extName,
	},
	Spec: &hz.PortalSpec{
		DisplayName: "Services",
		Icon:        gateway.IconRectangleStack,
	},
}

type PortalHandler struct {
	Conn           *nats.Conn
	CloudRunClient *cloudrun.APIService
}

func (h *PortalHandler) Router() *chi.Mux {
	r := chi.NewRouter()
	logger := httplog.NewLogger("portal-services", httplog.Options{
		JSON:             false,
		LogLevel:         slog.LevelInfo,
		Concise:          true,
		RequestHeaders:   true,
		MessageFieldName: "message",
		QuietDownRoutes: []string{
			"/",
			"/ping",
		},
		QuietDownPeriod: 10 * time.Second,
	})
	r.Use(httplog.RequestLogger(logger))
	r.Get("/", h.get)
	r.Post("/", h.post)
	return r
}

func (h *PortalHandler) get(rw http.ResponseWriter, req *http.Request) {
	if req.Header.Get("HX-Request") == "" {
		rendr := PortalRenderer{
			Namespace: req.Header.Get(hz.RequestNamespace),
			Portal:    req.Header.Get(hz.RequestPortal),
		}
		_ = rendr.home().Render(req.Context(), rw)
		return
	}
	h.tableBody(rw, req)
}

func (h *PortalHandler) post(rw http.ResponseWriter, req *http.Request) {
	rendr := PortalRenderer{
		Namespace: req.Header.Get(hz.RequestNamespace),
		Portal:    req.Header.Get(hz.RequestPortal),
	}
	if err := req.ParseForm(); err != nil {
		_ = rendr.form(
			"",
			fmt.Errorf("error parsing form: %w", err),
		).Render(req.Context(), rw)
		return
	}

	reqName := req.PostForm.Get("name")

	service := Service{
		ObjectMeta: hz.ObjectMeta{
			Namespace: req.Header.Get(hz.RequestNamespace),
			Name:      reqName,
		},
		Spec: &ServiceSpec{},
	}

	client := hz.ObjectClient[Service]{Client: hz.NewClient(
		h.Conn,
		hz.WithClientSessionFromRequest(req),
	)}
	if _, err := client.Apply(req.Context(), service, hz.WithApplyCreateOnly(true)); err != nil {
		_ = rendr.form(reqName, err).
			Render(req.Context(), rw)
		return
	}
	rw.WriteHeader(http.StatusCreated)
	rw.Header().Add("HX-Trigger", "newService")
	_ = rendr.form("", nil).Render(req.Context(), rw)
}

func (h *PortalHandler) tableBody(rw http.ResponseWriter, req *http.Request) {
	rendr := PortalRenderer{
		Namespace: req.Header.Get(hz.RequestNamespace),
		Portal:    req.Header.Get(hz.RequestPortal),
	}
	client := hz.ObjectClient[Service]{
		Client: hz.NewClient(h.Conn, hz.WithClientSessionFromRequest(req)),
	}
	services, err := client.List(
		req.Context(),
		hz.WithListKey(hz.ObjectKey{
			Namespace: req.Header.Get(hz.RequestNamespace),
		}),
	)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Println("SERVICES: ", services)

	_ = rendr.tableBody(services).Render(req.Context(), rw)
}

func isServiceReady(status *ServiceStatus) bool {
	if status == nil {
		return false
	}
	return status.Ready
}

type PortalRenderer struct {
	Namespace string
	Portal    string
}

func (r *PortalRenderer) URL(steps ...string) string {
	base := fmt.Sprintf("/namespaces/%s/portal/%s", r.Namespace, r.Portal)
	path := append([]string{base}, steps...)
	return strings.Join(path, "/")
}
