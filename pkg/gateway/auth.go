package gateway

import (
	"context"
	"net/http"
)

type UserInfo struct {
	Sub     string   `json:"sub"`
	Iss     string   `json:"iss"`
	Name    string   `json:"name"`
	Email   string   `json:"email"`
	Groups  []string `json:"groups"`
	Picture string   `json:"picture"`
}

var dummyAuthDefault = UserInfo{
	Sub:    "123",
	Iss:    "http://localhost:9998/",
	Name:   "John Doe",
	Email:  "local@localhost",
	Groups: []string{"admin"},
}

func dummyAuthHandler(next http.Handler, userInfo UserInfo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = context.WithValue(ctx, authContext, userInfo)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
