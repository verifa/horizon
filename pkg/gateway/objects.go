package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/hz"
)

type ObjectHandler struct {
	Conn *nats.Conn
}

func (o *ObjectHandler) router() *chi.Mux {
	r := chi.NewRouter()
	r.Get("/", o.get)
	r.Post("/", o.create)
	r.Patch("/", o.apply)
	r.Delete("/", o.delete)
	return r
}

func (o *ObjectHandler) create(w http.ResponseWriter, r *http.Request) {
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
	if err := client.Create(r.Context(), hz.WithCreateObject(obj)); err != nil {
		httpError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (o *ObjectHandler) get(w http.ResponseWriter, r *http.Request) {
	key := hz.ObjectKey{
		Group:   r.URL.Query().Get("group"),
		Kind:    r.URL.Query().Get("kind"),
		Name:    r.URL.Query().Get("name"),
		Account: r.URL.Query().Get("account"),
	}
	client := hz.NewClient(o.Conn, hz.WithClientSessionFromRequest(r))
	resp := bytes.Buffer{}
	if err := client.List(
		r.Context(),
		hz.WithListKeyFromObject(key),
		hz.WithListResponseWriter(&resp),
	); err != nil {
		httpError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(resp.Bytes())
}

func (o *ObjectHandler) apply(w http.ResponseWriter, r *http.Request) {
	manager := r.Header.Get(hz.HeaderFieldManager)
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
	fmt.Println("GENERIC OBJECT: ", obj)
	if err := client.Apply(r.Context(), hz.WithApplyObject(obj)); err != nil {
		httpError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (o *ObjectHandler) delete(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
