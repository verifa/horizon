package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/hz"
)

type ObjectsHandler struct {
	Conn *nats.Conn
}

func (o *ObjectsHandler) router() *chi.Mux {
	r := chi.NewRouter()
	r.Get("/", o.get)
	r.Post("/", o.create)
	r.Patch("/", o.apply)
	r.Delete("/{group}/{version}/{kind}/{namespace}/{name}", o.delete)
	return r
}

func (o *ObjectsHandler) create(w http.ResponseWriter, r *http.Request) {
	client := hz.NewClient(o.Conn, hz.WithClientSessionFromRequest(r))
	var obj hz.GenericObject
	if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
		http.Error(
			w,
			"decoding request body: "+err.Error(),
			http.StatusBadRequest,
		)
		return
	}
	if _, err := client.Apply(r.Context(), hz.WithApplyObject(obj), hz.WithApplyCreateOnly(true)); err != nil {
		httpError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (o *ObjectsHandler) get(w http.ResponseWriter, r *http.Request) {
	key := hz.ObjectKey{
		Group:     r.URL.Query().Get("group"),
		Version:   r.URL.Query().Get("version"),
		Kind:      r.URL.Query().Get("kind"),
		Name:      r.URL.Query().Get("name"),
		Namespace: r.URL.Query().Get("namespace"),
	}
	client := hz.NewClient(o.Conn, hz.WithClientSessionFromRequest(r))
	resp := bytes.Buffer{}
	if err := client.List(
		r.Context(),
		hz.WithListKey(key),
		hz.WithListResponseWriter(&resp),
	); err != nil {
		httpError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(resp.Bytes())
}

func (o *ObjectsHandler) apply(w http.ResponseWriter, r *http.Request) {
	manager := r.Header.Get(hz.HeaderApplyFieldManager)
	client := hz.NewClient(
		o.Conn,
		hz.WithClientSessionFromRequest(r),
		hz.WithClientManager(manager),
	)
	var obj hz.GenericObject
	if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
		http.Error(
			w,
			"decoding request body: "+err.Error(),
			http.StatusBadRequest,
		)
		return
	}
	if _, err := client.Apply(r.Context(), hz.WithApplyObject(obj)); err != nil {
		httpError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (o *ObjectsHandler) delete(w http.ResponseWriter, r *http.Request) {
	key := hz.ObjectKey{
		Group:     chi.URLParam(r, "group"),
		Version:   chi.URLParam(r, "version"),
		Kind:      chi.URLParam(r, "kind"),
		Namespace: chi.URLParam(r, "namespace"),
		Name:      chi.URLParam(r, "name"),
	}
	client := hz.NewClient(o.Conn, hz.WithClientSessionFromRequest(r))
	if err := client.Delete(r.Context(), hz.WithDeleteKey(key)); err != nil {
		httpError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
