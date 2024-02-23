package login

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/verifa/horizon/pkg/hz"
)

type LoginRequest struct {
	URL string `json:"url"`
}

type LoginResponse struct {
	Session     string
	Credentials string
}

func Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	baseURL, err := url.Parse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing horizon url: %w", err)
	}
	loginURL := baseURL.JoinPath("login")
	form, _ := url.ParseQuery(loginURL.RawQuery)

	list, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	port := list.Addr().(*net.TCPAddr).Port
	returnURL := fmt.Sprintf("http://localhost:%d/", port)
	// Add return_url to login request.
	form.Add("return_url", returnURL)
	loginURL.RawQuery = form.Encode()
	fmt.Println("loginURL: ", loginURL.String())

	if err := openBrowser(loginURL.String()); err != nil {
		return nil, fmt.Errorf("opening browser: %w", err)
	}

	resp := make(chan LoginResponse)
	lh := &loginHandler{
		baseURL: baseURL,
		resp:    resp,
	}

	mux := http.NewServeMux()
	mux.Handle(
		"/",
		http.HandlerFunc(lh.handleLogin),
	)
	server := http.Server{
		Addr:    list.Addr().String(),
		Handler: mux,
	}
	go func() {
		_ = server.Serve(list)
	}()
	defer server.Shutdown(ctx)

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("context done: %w", ctx.Err())
	case r := <-resp:
		return &r, nil
	case <-time.After(5 * time.Minute):
		return nil, errors.New("login timeout")
	}
}

type loginHandler struct {
	baseURL *url.URL
	resp    chan LoginResponse
}

func (l *loginHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	sessionCookie, err := r.Cookie(hz.CookieSession)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	userConfig, err := postNewUser(l.baseURL, sessionCookie, "test")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Write response to channel.
	resp := LoginResponse{
		Session:     sessionCookie.Value,
		Credentials: string(userConfig),
	}
	l.resp <- resp
	_ = layout(
		"login",
		pageStatusOK(resp),
	).Render(r.Context(), w)
}

func postNewUser(
	baseURL *url.URL,
	cookie *http.Cookie,
	account string,
) ([]byte, error) {
	loginURL := baseURL.JoinPath("auth", "login")
	req, err := http.NewRequest(
		"POST",
		loginURL.String(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.AddCookie(cookie)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if err := hz.ErrorFromHTTP(resp); err != nil {
		return nil, err
	}

	userConfig, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return userConfig, nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "openbsd":
		fallthrough
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		r := strings.NewReplacer("&", "^&")
		cmd = exec.Command("cmd", "/c", "start", r.Replace(url))
	}
	if cmd != nil {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Start()
		if err != nil {
			log.Printf("Failed to open browser due to error %v", err)
			return fmt.Errorf("Failed to open browser: " + err.Error())
		}
		err = cmd.Wait()
		if err != nil {
			log.Printf(
				"Failed to wait for open browser command to finish due to error %v",
				err,
			)
			return fmt.Errorf(
				"Failed to wait for open browser command to finish: " + err.Error(),
			)
		}
		return nil
	} else {
		return errors.New("unsupported platform")
	}
}
