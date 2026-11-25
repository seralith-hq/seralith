package control

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type ServerConfig struct {
	Store          *Store
	BootstrapToken string
	Logger         *slog.Logger
	Version        string
}

type Server struct {
	cfg     ServerConfig
	mux     *http.ServeMux
	metrics *metrics
}

type ctxKey int

const userKey ctxKey = iota

func NewServer(cfg ServerConfig) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux(), metrics: newMetrics()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.observe(s.recover(s.mux))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	s.mux.HandleFunc("GET /readyz", s.handleReady)
	s.mux.HandleFunc("GET /metrics", s.metrics.serve)
	s.mux.HandleFunc("POST /v1/bootstrap", s.handleBootstrap)

	any := func(pattern string, h http.HandlerFunc) { s.mux.Handle(pattern, s.auth(nil, h)) }
	admin := func(pattern string, h http.HandlerFunc) { s.mux.Handle(pattern, s.auth(Role.CanAdminister, h)) }
	writer := func(pattern string, h http.HandlerFunc) { s.mux.Handle(pattern, s.auth(Role.CanWrite, h)) }
	auditor := func(pattern string, h http.HandlerFunc) {
		s.mux.Handle(pattern, s.auth(func(r Role) bool { return r.CanAdminister() || r == RoleAuditor }, h))
	}

	any("GET /v1/me", s.handleMe)
	any("GET /v1/tokens", s.handleListTokens)
	any("POST /v1/tokens", s.handleCreateToken)
	any("DELETE /v1/tokens/{id}", s.handleRevokeToken)
	admin("GET /v1/members", s.handleListMembers)
	admin("POST /v1/members", s.handleAddMember)
	any("GET /v1/projects", s.handleListProjects)
	admin("POST /v1/projects", s.handleCreateProject)
	any("GET /v1/keys", s.handleListKeys)
	admin("POST /v1/keys", s.handleAddKey)
	admin("POST /v1/keys/{id}/revoke", s.handleRevokeKey)
	any("GET /v1/trust.yaml", s.handleTrustYAML)
	any("GET /v1/projects/{project}/policy", s.handleGetPolicy)
	any("GET /v1/projects/{project}/policy.yaml", s.handlePolicyYAML)
	admin("PUT /v1/projects/{project}/policy", s.handlePutPolicy)
	writer("POST /v1/projects/{project}/attestations", s.handleIngest)
	any("GET /v1/projects/{project}/attestations", s.handleListAttestations)
	any("GET /v1/projects/{project}/stats", s.handleStats)
	auditor("GET /v1/audit", s.handleAudit)
	auditor("GET /v1/audit/verify", s.handleAuditVerify)
}

func (s *Server) auth(allowed func(Role) bool, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || secret == "" {
			writeErr(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		u, err := s.cfg.Store.Authenticate(r.Context(), secret)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid token")
			return
		}
		if allowed != nil && !allowed(u.Role) {
			writeErr(w, http.StatusForbidden, fmt.Sprintf("role %s may not perform this action", u.Role))
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func userFrom(r *http.Request) *User {
	u, _ := r.Context().Value(userKey).(*User)
	return u
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var id [8]byte
		_, _ = rand.Read(id[:])
		reqID := hex.EncodeToString(id[:])
		w.Header().Set("X-Request-ID", reqID)
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		pattern := r.Pattern
		if pattern == "" {
			pattern = "unmatched"
		}
		s.metrics.record(pattern, sw.status, time.Since(start))
		s.cfg.Logger.Info("request", "id", reqID, "method", r.Method, "path", r.URL.Path, "status", sw.status, "duration", time.Since(start))
	})
}

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.cfg.Logger.Error("panic", "value", v, "stack", string(debug.Stack()))
				writeErr(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, limit int64, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, limit))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func storeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrConflict):
		writeErr(w, http.StatusConflict, "already exists")
	default:
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.cfg.Store.db.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "version": s.cfg.Version})
}

// handleBootstrap creates the first organization. It requires the operator-supplied
// bootstrap secret and is disabled once any organization exists.
func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	if s.cfg.BootstrapToken == "" {
		writeErr(w, http.StatusForbidden, "bootstrap is disabled")
		return
	}
	given := r.Header.Get("X-Bootstrap-Token")
	if subtle.ConstantTimeCompare([]byte(given), []byte(s.cfg.BootstrapToken)) != 1 {
		writeErr(w, http.StatusUnauthorized, "invalid bootstrap token")
		return
	}
	if n, err := s.cfg.Store.OrgCount(r.Context()); err != nil || n > 0 {
		writeErr(w, http.StatusConflict, "already bootstrapped")
		return
	}
	var req struct {
		Org   string `json:"org"`
		Slug  string `json:"slug"`
		Email string `json:"email"`
	}
	if err := readJSON(r, 1<<16, &req); err != nil || req.Org == "" || !slugPattern.MatchString(req.Slug) || !strings.Contains(req.Email, "@") {
		writeErr(w, http.StatusBadRequest, "org, slug (lowercase), and email are required")
		return
	}
	org, user, err := s.cfg.Store.CreateOrg(r.Context(), req.Org, req.Slug, req.Email)
	if err != nil {
		storeErr(w, err)
		return
	}
	secret, _, err := s.cfg.Store.IssueToken(r.Context(), user.ID, "bootstrap", 0)
	if err != nil {
		storeErr(w, err)
		return
	}
	_ = s.cfg.Store.Audit(r.Context(), org.ID, user.Email, "org.created", org.Slug, nil)
	writeJSON(w, http.StatusCreated, map[string]any{"org": org, "user": user, "token": secret})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	org, err := s.cfg.Store.Org(r.Context(), u.OrgID)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "org": org})
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	list, err := s.cfg.Store.Tokens(r.Context(), userFrom(r).ID)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		TTLHours int    `json:"ttlHours"`
	}
	if err := readJSON(r, 1<<16, &req); err != nil || req.Name == "" || req.TTLHours < 0 {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	u := userFrom(r)
	secret, tok, err := s.cfg.Store.IssueToken(r.Context(), u.ID, req.Name, time.Duration(req.TTLHours)*time.Hour)
	if err != nil {
		storeErr(w, err)
		return
	}
	_ = s.cfg.Store.Audit(r.Context(), u.OrgID, u.Email, "token.created", tok.ID, map[string]string{"name": req.Name})
	writeJSON(w, http.StatusCreated, map[string]any{"token": secret, "metadata": tok})
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if err := s.cfg.Store.RevokeToken(r.Context(), u.ID, r.PathValue("id")); err != nil {
		storeErr(w, err)
		return
	}
	_ = s.cfg.Store.Audit(r.Context(), u.OrgID, u.Email, "token.revoked", r.PathValue("id"), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	list, err := s.cfg.Store.Users(r.Context(), userFrom(r).OrgID)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Role  Role   `json:"role"`
	}
	if err := readJSON(r, 1<<16, &req); err != nil || !strings.Contains(req.Email, "@") || !req.Role.Valid() || req.Role == RoleOwner {
		writeErr(w, http.StatusBadRequest, "email and a role of admin, member, auditor, or ci are required")
		return
	}
	u := userFrom(r)
	member, err := s.cfg.Store.AddUser(r.Context(), u.OrgID, req.Email, req.Role)
	if err != nil {
		storeErr(w, err)
		return
	}
	_ = s.cfg.Store.Audit(r.Context(), u.OrgID, u.Email, "member.added", req.Email, map[string]Role{"role": req.Role})
	resp := map[string]any{"member": member}
	if req.Role == RoleCI {
		secret, _, err := s.cfg.Store.IssueToken(r.Context(), member.ID, "ci", 0)
		if err != nil {
			storeErr(w, err)
			return
		}
		resp["token"] = secret
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	list, err := s.cfg.Store.Projects(r.Context(), userFrom(r).OrgID)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Repository string `json:"repository"`
	}
	if err := readJSON(r, 1<<16, &req); err != nil || !slugPattern.MatchString(req.Name) {
		writeErr(w, http.StatusBadRequest, "name must be a lowercase slug")
		return
	}
	u := userFrom(r)
	p, err := s.cfg.Store.CreateProject(r.Context(), u.OrgID, req.Name, req.Repository)
	if err != nil {
		storeErr(w, err)
		return
	}
	_ = s.cfg.Store.Audit(r.Context(), u.OrgID, u.Email, "project.created", p.Name, nil)
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) project(w http.ResponseWriter, r *http.Request) (*Project, bool) {
	p, err := s.cfg.Store.ProjectByName(r.Context(), userFrom(r).OrgID, r.PathValue("project"))
	if err != nil {
		storeErr(w, err)
		return nil, false
	}
	return p, true
}

type metrics struct {
	mu        sync.Mutex
	requests  map[string]*atomic.Int64
	durations map[string]*atomic.Int64
	ingested  atomic.Int64
}

func newMetrics() *metrics {
	return &metrics{requests: map[string]*atomic.Int64{}, durations: map[string]*atomic.Int64{}}
}

func (m *metrics) record(pattern string, status int, d time.Duration) {
	key := fmt.Sprintf("route=%q,code=\"%d\"", pattern, status)
	m.mu.Lock()
	c, ok := m.requests[key]
	if !ok {
		c = &atomic.Int64{}
		m.requests[key] = c
	}
	dur, ok := m.durations[pattern]
	if !ok {
		dur = &atomic.Int64{}
		m.durations[pattern] = dur
	}
	m.mu.Unlock()
	c.Add(1)
	dur.Add(d.Microseconds())
}

func (m *metrics) serve(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	m.mu.Lock()
	defer m.mu.Unlock()
	fmt.Fprintln(w, "# HELP seralith_control_requests_total HTTP requests by route and status.")
	fmt.Fprintln(w, "# TYPE seralith_control_requests_total counter")
	keys := make([]string, 0, len(m.requests))
	for k := range m.requests {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "seralith_control_requests_total{%s} %d\n", k, m.requests[k].Load())
	}
	fmt.Fprintln(w, "# HELP seralith_control_request_duration_microseconds_total Cumulative request time by route.")
	fmt.Fprintln(w, "# TYPE seralith_control_request_duration_microseconds_total counter")
	routes := make([]string, 0, len(m.durations))
	for k := range m.durations {
		routes = append(routes, k)
	}
	sort.Strings(routes)
	for _, k := range routes {
		fmt.Fprintf(w, "seralith_control_request_duration_microseconds_total{route=%q} %d\n", k, m.durations[k].Load())
	}
	fmt.Fprintln(w, "# HELP seralith_control_attestations_ingested_total Attestations newly stored.")
	fmt.Fprintln(w, "# TYPE seralith_control_attestations_ingested_total counter")
	fmt.Fprintf(w, "seralith_control_attestations_ingested_total %d\n", m.ingested.Load())
}
