package gateway

import (
	"net/http"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/hz"
)

type GatewayHandler interface {
	GetHome(w http.ResponseWriter, r *http.Request)
	GetAccounts(w http.ResponseWriter, r *http.Request)
	GetAccountsNew(w http.ResponseWriter, r *http.Request)
	PostAccounts(w http.ResponseWriter, r *http.Request)
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

// GetAccounts implements GatewayHandler.
func (d *DefaultHandler) GetAccounts(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := r.Context().Value(authContext).(auth.UserInfo)
	if !ok {
		http.Error(w, "no auth context", http.StatusUnauthorized)
		return
	}
	client := hz.Client{
		Conn:    d.Conn,
		Session: hz.SessionFromRequest(r),
	}
	accClient := hz.ObjectClient[accounts.Account]{Client: client}
	accounts, err := accClient.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body := accountsPage(accounts)
	_ = layout("Accounts", &userInfo, body).Render(r.Context(), w)
}

// GetAccountsNew implements GatewayHandler.
func (d *DefaultHandler) GetAccountsNew(
	w http.ResponseWriter,
	r *http.Request,
) {
	userInfo, ok := r.Context().Value(authContext).(auth.UserInfo)
	if !ok {
		http.Error(w, "no auth context", http.StatusUnauthorized)
		return
	}
	body := accountsNewPage()
	_ = layout("New Account", &userInfo, body).Render(r.Context(), w)
}

// PostAccounts implements GatewayHandler.
func (d *DefaultHandler) PostAccounts(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("account-name")
	client := hz.Client{
		Conn:    d.Conn,
		Session: hz.SessionFromRequest(r),
	}
	accClient := hz.ObjectClient[accounts.Account]{Client: client}
	account := accounts.Account{
		ObjectMeta: hz.ObjectMeta{
			Name:    name,
			Account: hz.RootAccount,
		},
	}
	err := accClient.Create(r.Context(), account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Add("HX-Redirect", "/accounts/"+name)
}
