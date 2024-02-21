package gateway

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/hz"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/jwt/v2"
)

func (s *Server) serveAccount(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := r.Context().Value(authContext).(auth.UserInfo)
	if !ok {
		http.Error(w, "no auth context", http.StatusUnauthorized)
		return
	}
	account := chi.URLParam(r, "account")
	body := accountLayout(account, s.portals, accountPage())
	layout("Account", &userInfo, body).Render(r.Context(), w)
}

func (s *Server) serveAccountUsers(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := r.Context().Value(authContext).(auth.UserInfo)
	if !ok {
		http.Error(w, "no auth context", http.StatusUnauthorized)
		return
	}
	account := chi.URLParam(r, "account")
	body := accountLayout(account, s.portals, accountUsersPage(account))
	layout("Users", &userInfo, body).Render(r.Context(), w)
}

func (s *Server) postAccountUsers(
	w http.ResponseWriter,
	r *http.Request,
) {
	// TODO: should we use accounts.User here?
	// Would be easy, what about double account??
	// This starts to lean heavily on RBAC implementation.
	account := chi.URLParam(r, "account")
	user := accounts.User{
		ObjectMeta: hz.ObjectMeta{
			Name:    "TODO",
			Account: account,
		},
	}
	client := hz.Client{
		Conn:    s.Conn,
		Session: hz.SessionFromRequest(r),
	}
	userClient := hz.ObjectClient[accounts.User]{Client: client}
	reply, err := userClient.Run(
		r.Context(),
		&accounts.UserCreateAction{},
		user,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	b, err := json.Marshal(reply)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.Write(b)
}

func (s *Server) postAccountUserConfig(
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
	fmt.Println("")
	fmt.Println("")
	fmt.Println("user: ", user)
	fmt.Println("")
	fmt.Println("")
	client := hz.Client{
		Conn:    s.Conn,
		Session: hz.SessionFromRequest(r),
	}
	userClient := hz.ObjectClient[accounts.User]{Client: client}
	reply, err := userClient.Run(
		r.Context(),
		&accounts.UserCreateAction{},
		user,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
