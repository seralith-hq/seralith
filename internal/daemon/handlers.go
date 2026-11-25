package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/seralith-hq/seralith/internal/attest"
	"github.com/seralith-hq/seralith/internal/blame"
	"github.com/seralith-hq/seralith/internal/ledger"
	"github.com/seralith-hq/seralith/internal/session"
	"github.com/seralith-hq/seralith/internal/verify"
	"github.com/seralith-hq/seralith/internal/version"
	"github.com/seralith-hq/seralith/internal/workspace"
	"github.com/seralith-hq/seralith/pkg/provenance"
)

func (s *Server) handleProject(w http.ResponseWriter, _ *http.Request) {
	tk, trusted := s.cfg.Trust.Lookup(s.cfg.Key.KeyID())
	roles := []string{}
	if trusted {
		roles = tk.Roles
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":         s.cfg.Project.Config.Name,
		"root":         s.cfg.Project.Root,
		"principal":    s.cfg.Principal,
		"key":          s.cfg.Key.KeyID(),
		"trusted":      trusted && !tk.Revoked,
		"roles":        roles,
		"policyDigest": s.cfg.Policy.Digest(),
		"version":      version.Version,
	})
}

func (s *Server) handleAgents(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Agents.Detect())
}

type workspaceView struct {
	*workspace.Workspace
	Running bool `json:"running"`
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, _ *http.Request) {
	list, err := s.cfg.Workspaces.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	running := map[string]bool{}
	for _, info := range s.cfg.Sessions.List() {
		if info.State == session.StateRunning {
			running[info.Workspace] = true
		}
	}
	out := make([]workspaceView, 0, len(list))
	for _, ws := range list {
		out = append(out, workspaceView{Workspace: ws, Running: running[ws.Name]})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Base string `json:"base"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ws, err := s.cfg.Workspaces.Create(r.Context(), workspace.CreateOptions{Name: req.Name, Base: req.Base, Owner: s.cfg.Principal})
	switch {
	case errors.Is(err, workspace.ErrExists):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, workspace.ErrInvalidName):
		writeError(w, http.StatusBadRequest, err)
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
	default:
		s.events.publish(Event{Type: "workspace", Data: ws})
		writeJSON(w, http.StatusCreated, ws)
	}
}

func (s *Server) handleRemoveWorkspace(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.busy(name) {
		writeError(w, http.StatusConflict, session.ErrBusy)
		return
	}
	force := r.URL.Query().Get("force") == "true"
	err := s.cfg.Workspaces.Remove(r.Context(), name, workspace.RemoveOptions{Force: force, DeleteBranch: true, Actor: "human:" + s.cfg.Principal})
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	s.events.publish(Event{Type: "workspace.removed", Data: map[string]string{"name": name}})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) busy(name string) bool {
	for _, info := range s.cfg.Sessions.List() {
		if info.Workspace == name && info.State == session.StateRunning {
			return true
		}
	}
	return false
}

func (s *Server) handleWorkspaceStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.busy(name) {
		ws, err := s.cfg.Workspaces.Get(name)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"Workspace": ws, "Segments": ws.Segments, "Running": true})
		return
	}
	pv, err := s.attest.Preview(r.Context(), name)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, pv)
}

// handleWorkspaceDiff reports the uncommitted diff without recording a segment, so it is
// safe to call while a session is running.
func (s *Server) handleWorkspaceDiff(w http.ResponseWriter, r *http.Request) {
	ws, err := s.cfg.Workspaces.Get(r.PathValue("name"))
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	repo := ws.Repo()
	headTree, err := repo.CommitTree(r.Context(), "HEAD")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	snap, err := repo.Snapshot(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	files, err := repo.DiffTrees(r.Context(), headTree, snap)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	patch, err := repo.UnifiedDiff(r.Context(), headTree, snap)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	const limit = 2 << 20
	truncated := len(patch) > limit
	if truncated {
		patch = patch[:limit]
	}
	if files == nil {
		files = []provenance.FileChange{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "patch": patch, "truncated": truncated})
}

func (s *Server) handleCommit(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.busy(name) {
		writeError(w, http.StatusConflict, errors.New("stop the running session before committing"))
		return
	}
	var req struct {
		Message string `json:"message"`
		Force   bool   `json:"force"`
	}
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, errors.New("a commit message is required"))
		return
	}
	res, err := s.attest.Commit(r.Context(), attest.CommitOptions{Workspace: name, Message: req.Message, Force: req.Force})
	switch {
	case errors.Is(err, attest.ErrDenied):
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	case err != nil:
		writeError(w, statusFor(err), err)
		return
	}
	s.events.publish(Event{Type: "commit", Data: map[string]string{"commit": res.Commit, "workspace": name}})
	writeJSON(w, http.StatusCreated, map[string]any{"commit": res.Commit, "attestation": res.Digest, "predicate": res.Predicate})
}

func (s *Server) handleListSessions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Sessions.List())
}

func (s *Server) handleStartSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Workspace string   `json:"workspace"`
		Agent     string   `json:"agent"`
		Command   []string `json:"command"`
		Model     string   `json:"model"`
		Cols      uint16   `json:"cols"`
		Rows      uint16   `json:"rows"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Agent == "" {
		req.Agent = "shell"
	}
	profile, ok := s.cfg.Agents.Get(req.Agent)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unknown agent %q", req.Agent))
		return
	}
	command := req.Command
	if len(command) == 0 {
		command = profile.Command
	}
	actor := profile.Actor(r.Context(), s.cfg.Principal, command, os.Environ())
	if req.Model != "" {
		actor.Model = req.Model
	}
	sess, err := s.cfg.Sessions.Start(context.WithoutCancel(r.Context()), session.Spec{
		Workspace: req.Workspace, Actor: actor, Command: command, PTY: true, Cols: req.Cols, Rows: req.Rows,
	})
	switch {
	case errors.Is(err, session.ErrBusy):
		writeError(w, http.StatusConflict, err)
	case err != nil:
		writeError(w, statusFor(err), err)
	default:
		writeJSON(w, http.StatusCreated, sess.Info())
	}
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.cfg.Sessions.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("session not found"))
		return
	}
	writeJSON(w, http.StatusOK, sess.Info())
}

func (s *Server) handleSignalSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.cfg.Sessions.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("session not found"))
		return
	}
	var req struct {
		Signal string `json:"signal"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sig := map[string]os.Signal{"interrupt": os.Interrupt, "terminate": syscall.SIGTERM, "kill": os.Kill}[req.Signal]
	if sig == nil {
		writeError(w, http.StatusBadRequest, errors.New("signal must be interrupt, terminate, or kill"))
		return
	}
	if err := sess.Signal(sig); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleTranscript(w http.ResponseWriter, r *http.Request) {
	f, err := s.cfg.Sessions.Transcript(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("transcript not found"))
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = io.Copy(w, f)
}

type commitRow struct {
	Commit   string                       `json:"commit"`
	Subject  string                       `json:"subject"`
	Author   string                       `json:"author"`
	Date     time.Time                    `json:"date"`
	Branches []string                     `json:"branches"`
	Status   verify.Status                `json:"status"`
	Lines    map[provenance.ActorKind]int `json:"lines,omitempty"`
	Problems int                          `json:"problems"`
}

func (s *Server) handleListCommits(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 40
	}
	repo := s.cfg.Project.Repo
	out, err := repo.Run(r.Context(), "log", "--branches=seralith/*", "--not", "--exclude=seralith/*", "--branches",
		"-n", strconv.Itoa(limit), "--format=%H%x1f%s%x1f%ae%x1f%cI%x1f%D")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	rows := []commitRow{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, "\x1f")
		if len(parts) < 5 {
			continue
		}
		date, _ := time.Parse(time.RFC3339, parts[3])
		row := commitRow{Commit: parts[0], Subject: parts[1], Author: parts[2], Date: date, Branches: refs(parts[4])}
		if rep, err := verify.Commit(r.Context(), repo, parts[0], verify.Options{Trust: s.cfg.Trust, Policy: s.cfg.Policy, SkipMerges: true}); err == nil {
			row.Status, row.Problems = rep.Status, len(rep.Problems)
			if rep.Summary != nil {
				row.Lines = rep.Summary.LinesByKind
			}
		}
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, rows)
}

func refs(decoration string) []string {
	out := []string{}
	for _, d := range strings.Split(decoration, ",") {
		d = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(d), "HEAD -> "))
		if d != "" && !strings.HasPrefix(d, "tag:") {
			out = append(out, d)
		}
	}
	return out
}

func (s *Server) handleVerifyCommit(w http.ResponseWriter, r *http.Request) {
	rep, err := verify.Commit(r.Context(), s.cfg.Project.Repo, r.PathValue("sha"), verify.Options{Trust: s.cfg.Trust, Policy: s.cfg.Policy})
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"report": rep, "predicate": rep.Predicate})
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role    string   `json:"role"`
		Rules   []string `json:"rules"`
		Reject  bool     `json:"reject"`
		Comment string   `json:"comment"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	env, commit, err := s.attest.Approve(r.Context(), s.cfg.Project.Repo, attest.ApproveOptions{
		Rev: r.PathValue("sha"), Role: req.Role, Rules: req.Rules, Reject: req.Reject, Comment: req.Comment,
	})
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	s.events.publish(Event{Type: "approval", Data: map[string]string{"commit": commit}})
	writeJSON(w, http.StatusCreated, map[string]string{"commit": commit, "attestation": provenance.Digest(env)})
}

func (s *Server) handleGate(w http.ResponseWriter, r *http.Request) {
	rangeSpec := r.URL.Query().Get("range")
	if rangeSpec == "" {
		writeError(w, http.StatusBadRequest, errors.New("range is required"))
		return
	}
	rep, err := verify.Gate(r.Context(), s.cfg.Project.Repo, rangeSpec, verify.Options{Trust: s.cfg.Trust, Policy: s.cfg.Policy, SkipMerges: true})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleBlame(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rev := q.Get("rev")
	if rev == "" {
		rev = "HEAD"
	}
	lines, sum, err := blame.File(r.Context(), s.cfg.Project.Repo, rev, q.Get("path"), s.cfg.Trust)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines, "summary": sum})
}

func (s *Server) handleLedger(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 200
	}
	entries, err := s.cfg.Ledger.Entries(ledger.Filter{Type: q.Get("type"), Subject: q.Get("subject"), Limit: limit})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if entries == nil {
		entries = []ledger.Entry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleLedgerVerify(w http.ResponseWriter, _ *http.Request) {
	rep, err := s.cfg.Ledger.Verify(s.cfg.Trust)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handlePolicy(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Policy)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch, cancel := s.events.subscribe()
	defer cancel()
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case e, ok := <-ch:
			if !ok {
				return
			}
			data, _ := jsonString(e)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, data)
			flusher.Flush()
		}
	}
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, workspace.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, workspace.ErrExists), errors.Is(err, session.ErrBusy):
		return http.StatusConflict
	case strings.Contains(err.Error(), "nothing to commit"), strings.Contains(err.Error(), "unattested segment"):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
