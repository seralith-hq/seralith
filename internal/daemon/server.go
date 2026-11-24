// Package daemon serves the Sigil workbench API and UI on the loopback interface.
package daemon

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ryan-tobin/new-startup/internal/agent"
	"github.com/ryan-tobin/new-startup/internal/attest"
	"github.com/ryan-tobin/new-startup/internal/keys"
	"github.com/ryan-tobin/new-startup/internal/ledger"
	"github.com/ryan-tobin/new-startup/internal/policy"
	"github.com/ryan-tobin/new-startup/internal/project"
	"github.com/ryan-tobin/new-startup/internal/session"
	"github.com/ryan-tobin/new-startup/internal/workspace"
)

type Config struct {
	Project    *project.Project
	Ledger     *ledger.Ledger
	Workspaces *workspace.Manager
	Sessions   *session.Manager
	Agents     *agent.Registry
	Policy     *policy.Policy
	Trust      *keys.TrustStore
	Key        *keys.PrivateKey
	Principal  string
	UI         fs.FS
	Token      string
	DevOrigins []string
	Logger     *slog.Logger
}

type Server struct {
	cfg    Config
	attest *attest.Service
	events *broker
	mux    *http.ServeMux
}

// NewToken returns a random bearer token for a daemon run.
func NewToken() string {
	var b [24]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &Server{
		cfg: cfg,
		attest: &attest.Service{
			Workspaces: cfg.Workspaces, Ledger: cfg.Ledger, Policy: cfg.Policy,
			Signer: cfg.Key, Principal: cfg.Principal,
		},
		events: newBroker(),
		mux:    http.NewServeMux(),
	}
	prev := cfg.Sessions.OnChange
	cfg.Sessions.OnChange = func(info session.Info) {
		if prev != nil {
			prev(info)
		}
		s.events.publish(Event{Type: "session", Data: info})
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	api := func(pattern string, h http.HandlerFunc) {
		s.mux.Handle(pattern, s.guard(h))
	}
	api("GET /api/v1/project", s.handleProject)
	api("GET /api/v1/agents", s.handleAgents)
	api("GET /api/v1/workspaces", s.handleListWorkspaces)
	api("POST /api/v1/workspaces", s.handleCreateWorkspace)
	api("DELETE /api/v1/workspaces/{name}", s.handleRemoveWorkspace)
	api("GET /api/v1/workspaces/{name}/status", s.handleWorkspaceStatus)
	api("GET /api/v1/workspaces/{name}/diff", s.handleWorkspaceDiff)
	api("POST /api/v1/workspaces/{name}/commit", s.handleCommit)
	api("GET /api/v1/sessions", s.handleListSessions)
	api("POST /api/v1/sessions", s.handleStartSession)
	api("GET /api/v1/sessions/{id}", s.handleGetSession)
	api("POST /api/v1/sessions/{id}/signal", s.handleSignalSession)
	api("GET /api/v1/sessions/{id}/transcript", s.handleTranscript)
	api("GET /api/v1/sessions/{id}/pty", s.handlePTY)
	api("GET /api/v1/commits", s.handleListCommits)
	api("GET /api/v1/commits/{sha}", s.handleVerifyCommit)
	api("POST /api/v1/commits/{sha}/approve", s.handleApprove)
	api("GET /api/v1/gate", s.handleGate)
	api("GET /api/v1/blame", s.handleBlame)
	api("GET /api/v1/ledger", s.handleLedger)
	api("GET /api/v1/ledger/verify", s.handleLedgerVerify)
	api("GET /api/v1/policy", s.handlePolicy)
	api("GET /api/v1/events", s.handleEvents)
	s.mux.Handle("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	s.mux.Handle("/", s.hostGuard(s.uiHandler()))
}

func (s *Server) Handler() http.Handler {
	return securityHeaders(s.mux)
}

// Serve listens on addr until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		s.events.close()
		_ = srv.Shutdown(shutdown)
	}()
	err := srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// guard rejects requests that could come from another site or a rebinding attack and
// requires the per-run bearer token.
func (s *Server) guard(next http.HandlerFunc) http.Handler {
	return s.hostGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && !s.allowedOrigin(origin) {
			writeError(w, http.StatusForbidden, errors.New("cross-origin request refused"))
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.Token)) != 1 {
			writeError(w, http.StatusUnauthorized, errors.New("missing or invalid token"))
			return
		}
		next(w, r)
	}))
}

func (s *Server) allowedOrigin(origin string) bool {
	for _, o := range s.cfg.DevOrigins {
		if origin == o {
			return true
		}
	}
	host := strings.TrimPrefix(strings.TrimPrefix(origin, "http://"), "https://")
	return loopbackHost(host)
}

func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; connect-src 'self' ws://localhost:* ws://127.0.0.1:*; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) uiHandler() http.Handler {
	if s.cfg.UI == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(fallbackPage))
		})
	}
	files := http.FileServer(http.FS(s.cfg.UI))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(s.cfg.UI, path); err != nil {
			r.URL.Path = "/"
		}
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

const fallbackPage = `<!doctype html><html><head><meta charset="utf-8"><title>Sigil</title>
<style>body{font:15px/1.6 system-ui;background:#0E1116;color:#E9E6DF;display:grid;place-items:center;height:100vh;margin:0}code{color:#FF5A1F}</style>
</head><body><div><h1>Sigil daemon is running</h1><p>The workbench UI is not embedded in this build. Run <code>make web build</code>, or use the API at <code>/api/v1</code>.</p></div></body></html>`

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type apiError struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, apiError{Error: err.Error()})
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// Event is pushed to subscribers of /api/v1/events.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type broker struct {
	mu     sync.Mutex
	subs   map[chan Event]struct{}
	closed bool
}

func newBroker() *broker { return &broker{subs: map[chan Event]struct{}{}} }

func (b *broker) subscribe() (chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan Event, 64)
	if b.closed {
		close(ch)
		return ch, func() {}
	}
	b.subs[ch] = struct{}{}
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
}

func (b *broker) publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

func (b *broker) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for ch := range b.subs {
		close(ch)
		delete(b.subs, ch)
	}
}
