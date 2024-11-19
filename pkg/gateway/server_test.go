package gateway_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/extensions/core"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestHandler(t *testing.T) {
	ctx := context.Background()
	ts := server.Test(t, ctx)
	handler := ts.Gateway.HTTPServer.Handler

	sess, err := ts.Auth.Sessions.New(ctx, auth.UserInfo{
		Sub:    "test",
		Iss:    "test",
		Groups: []string{"admin"},
	})
	tu.AssertNoError(t, err)

	form := url.Values{
		"namespace-name": {"another"},
	}
	createReq, err := http.NewRequest(
		http.MethodPost,
		"/namespaces",
		strings.NewReader(form.Encode()),
	)
	tu.AssertNoError(t, err)
	createReq.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	createReq.AddCookie(&http.Cookie{
		Name:  hz.CookieSession,
		Value: sess,
	})

	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	tu.AssertEqual(t, http.StatusCreated, createRec.Result().StatusCode)

	nsClient := hz.ObjectClient[core.Namespace]{
		Client: hz.NewClient(ts.Conn, hz.WithClientSession(sess)),
	}
	ns, err := nsClient.Get(
		ctx,
		hz.WithGetKey(hz.ObjectKeyFromObject(core.Namespace{
			ObjectMeta: hz.ObjectMeta{
				Namespace: hz.NamespaceRoot,
				Name:      "another",
			},
		})),
	)
	tu.AssertNoError(t, err)
	tu.AssertEqual(t, "another", ns.Name)
}
