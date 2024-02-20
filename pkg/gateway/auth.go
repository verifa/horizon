package gateway

import (
	"context"
	"net/http"

	"github.com/verifa/horizon/pkg/auth"
)

var dummyAuthDefault = auth.UserInfo{
	Sub:    "123",
	Iss:    "http://localhost:9998/",
	Name:   "John Doe",
	Email:  "local@localhost",
	Groups: []string{"admin"},
}

func dummyAuthHandler(
	next http.Handler,
	userInfo auth.UserInfo,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = context.WithValue(ctx, authContext, userInfo)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
