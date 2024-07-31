package greetings

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/httplog/v2"
	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/gateway"
	"github.com/verifa/horizon/pkg/hz"
)

const extName = "greetings"

var Portal = hz.Portal{
	ObjectMeta: hz.ObjectMeta{
		Namespace: hz.RootNamespace,
		Name:      extName,
	},
	Spec: &hz.PortalSpec{
		DisplayName: "Greetings",
		Icon:        gateway.IconCodeBracketSquare,
	},
}

type PortalHandler struct {
	Conn *nats.Conn
}

func (h *PortalHandler) Router() *chi.Mux {
	r := chi.NewRouter()
	logger := httplog.NewLogger("portal-greetings", httplog.Options{
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
	r.Post("/greet", h.postGreet)
	return r
}

func (h *PortalHandler) get(rw http.ResponseWriter, req *http.Request) {
	rendr := PortalRenderer{
		Namespace: req.Header.Get(hz.RequestNamespace),
		Portal:    req.Header.Get(hz.RequestPortal),
	}
	if req.Header.Get("HX-Request") == "" {
		_ = rendr.home().Render(req.Context(), rw)
		return
	}
	client := hz.NewClient(h.Conn, hz.WithClientSessionFromRequest(req))
	greetClient := hz.ObjectClient[Greeting]{Client: client}
	greetings, err := greetClient.List(
		req.Context(),
		hz.WithListKey(hz.ObjectKey{
			Namespace: req.Header.Get(hz.RequestNamespace),
		}),
	)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = rendr.greetingsTable(greetings).Render(req.Context(), rw)
}

func (h *PortalHandler) post(rw http.ResponseWriter, req *http.Request) {
	rendr := PortalRenderer{
		Namespace: req.Header.Get(hz.RequestNamespace),
		Portal:    req.Header.Get(hz.RequestPortal),
	}
	if err := req.ParseForm(); err != nil {
		_ = rendr.greetingsControllerForm(
			"",
			fmt.Errorf("error parsing form: %w", err),
		).Render(req.Context(), rw)
		return
	}

	reqName := req.PostForm.Get("name")

	greeting := Greeting{
		ObjectMeta: hz.ObjectMeta{
			Namespace: req.Header.Get(hz.RequestNamespace),
			Name:      reqName,
		},
		Spec: &GreetingSpec{
			Name: reqName,
		},
	}
	client := hz.NewClient(
		h.Conn,
		hz.WithClientSessionFromRequest(req),
		hz.WithClientDefaultManager(),
	)
	greetClient := hz.ObjectClient[Greeting]{Client: client}
	if err := greetClient.Create(req.Context(), greeting); err != nil {
		_ = rendr.greetingsControllerForm(reqName, err).
			Render(req.Context(), rw)
		return
	}
	rw.WriteHeader(http.StatusCreated)
	rw.Header().Add("HX-Trigger", "newGreeting")
	_ = rendr.greetingsControllerForm("", nil).Render(req.Context(), rw)
}

func (h *PortalHandler) postGreet(rw http.ResponseWriter, req *http.Request) {
	rendr := PortalRenderer{
		Namespace: req.Header.Get(hz.RequestNamespace),
		Portal:    req.Header.Get(hz.RequestPortal),
	}
	if err := req.ParseForm(); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	reqName := req.PostForm.Get("name")

	greeting := Greeting{
		ObjectMeta: hz.ObjectMeta{
			Namespace: req.Header.Get(hz.RequestNamespace),
			Name:      reqName,
		},
		Spec: &GreetingSpec{
			Name: reqName,
		},
	}
	client := hz.NewClient(h.Conn, hz.WithClientSessionFromRequest(req))
	greetClient := hz.ObjectClient[Greeting]{Client: client}
	reply, err := greetClient.Run(
		req.Context(),
		GreetingsHelloAction{},
		greeting,
	)
	if err != nil {
		_ = rendr.greetingsActorForm(reqName, reply.Status, err).
			Render(req.Context(), rw)
		return
	}

	_ = rendr.greetingsActorForm("", reply.Status, nil).
		Render(req.Context(), rw)
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
