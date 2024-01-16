package natsutil

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type ServerOption func(*serverOption)

type serverOption struct {
	FindAvailablePort bool
	Port              int

	Dir string

	ConfigureLogger bool

	JWTAuth    ServerJWTAuth
	UseJWTAuth bool
}

var serverOptionDefaults = serverOption{
	FindAvailablePort: false,
	Port:              4222,
	Dir:               "",
}

func WithFindAvailablePort(enabled bool) ServerOption {
	return func(so *serverOption) {
		so.FindAvailablePort = enabled
	}
}

func WithPort(port int) ServerOption {
	return func(so *serverOption) {
		so.Port = port
	}
}

func WithDir(dir string) ServerOption {
	return func(so *serverOption) {
		so.Dir = dir
	}
}

func WithJWTAuth(jwtAuth ServerJWTAuth) ServerOption {
	return func(so *serverOption) {
		so.JWTAuth = jwtAuth
		so.UseJWTAuth = true
	}
}

func WithConfigureLogger(logger bool) ServerOption {
	return func(so *serverOption) {
		so.ConfigureLogger = true
	}
}

func NewServer(opts ...ServerOption) (Server, error) {
	sOpt := serverOptionDefaults
	for _, opt := range opts {
		opt(&sOpt)
	}
	if sOpt.Dir == "" {
		sOpt.Dir = filepath.Join(os.TempDir(), "horizon")
	}
	if sOpt.FindAvailablePort {
		port, err := findAvailablePort()
		if err != nil {
			return Server{}, fmt.Errorf("finding available port: %w", err)
		}
		sOpt.Port = port
	}

	var jwtAuth ServerJWTAuth
	if sOpt.UseJWTAuth {
		jwtAuth = sOpt.JWTAuth
	} else {
		var err error
		jwtAuth, err = BootstrapServerJWTAuth()
		if err != nil {
			return Server{}, fmt.Errorf("bootstrapping server JWT auth: %w", err)
		}
	}

	natsConf, err := generateConfigFile(ServerConfig{
		Dir:                 sOpt.Dir,
		Debug:               true,
		Host:                "", // Is this needed?
		Port:                sOpt.Port,
		OperatorJWT:         jwtAuth.Operator.JWT,
		SysAccountPublicKey: jwtAuth.SysAccount.PublicKey,
		SysAccountJWT:       jwtAuth.SysAccount.JWT,
	})
	if err != nil {
		return Server{}, fmt.Errorf("generating nats conf: %w", err)
	}
	natsConfFile := filepath.Join(sOpt.Dir, "resolver.conf")
	if err := os.MkdirAll(filepath.Dir(natsConfFile), os.ModePerm); err != nil {
		return Server{}, fmt.Errorf(
			"creating dir for nats conf file: %w",
			err,
		)
	}
	if err := os.WriteFile(natsConfFile, []byte(natsConf), os.ModePerm); err != nil {
		return Server{}, fmt.Errorf("writing nats conf: %w", err)
	}

	nsOpts := &server.Options{}
	if err := nsOpts.ProcessConfigFile(natsConfFile); err != nil {
		return Server{}, fmt.Errorf("processing config file: %w", err)
	}

	if err := os.RemoveAll(natsConfFile); err != nil {
		return Server{}, fmt.Errorf(
			"removing NATS conf file %s: %w",
			natsConfFile,
			err,
		)
	}
	// Make sure NATS is not using its own signal handler as it interferes with
	// any signal handling that we do.
	nsOpts.NoSigs = true

	ns, err := server.NewServer(nsOpts)
	if err != nil {
		return Server{}, fmt.Errorf("new server: %w", err)
	}
	if sOpt.ConfigureLogger {
		ns.ConfigureLogger()
	}

	return Server{
		NS:   ns,
		Auth: jwtAuth,
	}, nil
}

type Server struct {
	NS   *server.Server
	Auth ServerJWTAuth
}

func (s Server) StartUntilReady() error {
	timeout := 4 * time.Second
	s.NS.Start()
	if !s.NS.ReadyForConnections(timeout) {
		return fmt.Errorf("server not ready after %s", timeout.String())
	}
	return nil
}

// sysUserConn uses the system user JWT to connect to the server.
// This is required to be able to create the root account.
// It is not expected that a user will need to use this.
func (s Server) sysUserConn() (*nats.Conn, error) {
	nc, err := nats.Connect(
		s.NS.ClientURL(),
		nats.UserJWTAndSeed(s.Auth.SysUser.JWT, string(s.Auth.SysUser.Seed)),
		// UserJWTOption(t.Auth.SysUser.JWT, t.Auth.SysUser.KeyPair),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to nats: %w", err)
	}
	return nc, nil
}

// RootUserConn uses the root user JWT to connect to the server.
func (s Server) RootUserConn() (*nats.Conn, error) {
	nc, err := nats.Connect(
		s.NS.ClientURL(),
		nats.UserJWTAndSeed(
			s.Auth.RootUser.JWT,
			string(s.Auth.RootUser.Seed),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to nats: %w", err)
	}
	return nc, nil
}

// PublishRootAccount publishes the root account JWT to the server.
// This is required to be able to connect to the server with the root account
// and therefore needs to happen once the server has been started.
func (s Server) PublishRootAccount() error {
	nc, err := s.sysUserConn()
	if err != nil {
		return fmt.Errorf("connecting to system_account nats: %w", err)
	}
	defer nc.Close()
	// Check if the account already exists
	lookupMessage, err := nc.Request(
		fmt.Sprintf(
			"$SYS.REQ.ACCOUNT.%s.CLAIMS.LOOKUP",
			s.Auth.RootAccount.PublicKey,
		),
		nil,
		time.Second,
	)
	if err != nil {
		return fmt.Errorf(
			"looking up account with ID %s: %w",
			s.Auth.RootAccount.PublicKey,
			err,
		)
	}
	// If the account exists, do not re-create it because it contains imports
	// from the accounts that will get overwritten.
	//
	// Please find a more elegant way of doing this.
	if string(lookupMessage.Data) != "" {
		return nil
	}
	if _, err := nc.Request(
		"$SYS.REQ.CLAIMS.UPDATE",
		[]byte(s.Auth.RootAccount.JWT),
		time.Second,
	); err != nil {
		return fmt.Errorf("requesting new account: %w", err)
	}
	return nil
}

type ServerConfig struct {
	Dir                 string
	Debug               bool
	Host                string
	Port                int
	OperatorJWT         string
	SysAccountPublicKey string
	SysAccountJWT       string
}

func generateConfigFile(config ServerConfig) (string, error) {
	const natsConf = `
host: "{{ .Host }}"
port: {{ .Port }}
operator: "{{ .OperatorJWT }}"
system_account: "{{ .SysAccountPublicKey }}"

debug: {{ .Debug }}

# JetStream configuration
jetstream {
	store_dir: "{{ .Dir }}/jetstream"

}

# Configuration of the nats based resolver
resolver {
    type: "full"
    dir: "{{ .Dir }}/resolver"
    allow_delete: true
}

# Prepopulate the resolver with the SYS account.
# All other accounts will be added dynamically.
resolver_preload: {
	"{{ .SysAccountPublicKey }}": "{{ .SysAccountJWT }}",
}
`
	t, err := template.New("nats.conf").Parse(natsConf)
	if err != nil {
		return "", fmt.Errorf("parsing nats conf template: %w", err)
	}
	var buf strings.Builder
	if err := t.Execute(&buf, config); err != nil {
		return "", fmt.Errorf("executing nats conf template: %w", err)
	}
	return buf.String(), nil
}

func findAvailablePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return -1, fmt.Errorf("listen: %w", err)
	}
	l.Close()

	return l.Addr().(*net.TCPAddr).Port, nil
}
