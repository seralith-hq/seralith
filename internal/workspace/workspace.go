// Package workspace manages isolated git worktrees and the change segments observed in them.
package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ryan-tobin/new-startup/internal/fsutil"
	"github.com/ryan-tobin/new-startup/internal/gitx"
	"github.com/ryan-tobin/new-startup/internal/ledger"
	"github.com/ryan-tobin/new-startup/internal/project"
	"github.com/ryan-tobin/new-startup/pkg/provenance"
)

const BranchPrefix = "seralith/"

var (
	ErrNotFound    = errors.New("workspace: not found")
	ErrExists      = errors.New("workspace: already exists")
	ErrInvalidName = errors.New("workspace: name must be 1-63 lowercase letters, digits, '-', '_' or '.'")
	validName      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
)

type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

type Workspace struct {
	Name       string               `json:"name"`
	Path       string               `json:"path"`
	Branch     string               `json:"branch"`
	BaseCommit string               `json:"baseCommit"`
	Owner      string               `json:"owner"`
	CreatedAt  time.Time            `json:"createdAt"`
	UpdatedAt  time.Time            `json:"updatedAt"`
	Status     Status               `json:"status"`
	Head       string               `json:"head"`
	Checkpoint string               `json:"checkpoint"`
	Segments   []provenance.Segment `json:"segments"`
}

// Pending reports whether the workspace has segments not yet bound to a commit.
func (w *Workspace) Pending() bool { return len(w.Segments) > 0 }

func (w *Workspace) Repo() *gitx.Repo { return &gitx.Repo{Dir: w.Path} }

type db struct {
	Version    int                   `json:"version"`
	Workspaces map[string]*Workspace `json:"workspaces"`
}

// Manager owns the workspace database for a project.
type Manager struct {
	Project *project.Project
	Ledger  *ledger.Ledger
	Now     func() time.Time
}

func NewManager(p *project.Project, l *ledger.Ledger) *Manager {
	return &Manager{Project: p, Ledger: l, Now: func() time.Time { return time.Now().UTC() }}
}

func (m *Manager) lockPath() string { return m.Project.WorkspaceDB() + ".lock" }

func (m *Manager) read() (*db, error) {
	data, err := os.ReadFile(m.Project.WorkspaceDB())
	if errors.Is(err, os.ErrNotExist) {
		return &db{Version: 1, Workspaces: map[string]*Workspace{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var d db
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("workspace: corrupt database: %w", err)
	}
	if d.Workspaces == nil {
		d.Workspaces = map[string]*Workspace{}
	}
	return &d, nil
}

func (m *Manager) write(d *db) error {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteAtomic(m.Project.WorkspaceDB(), data, 0o600)
}

func (m *Manager) update(fn func(*db) error) error {
	return fsutil.WithLock(m.lockPath(), func() error {
		d, err := m.read()
		if err != nil {
			return err
		}
		if err := fn(d); err != nil {
			return err
		}
		return m.write(d)
	})
}

// List returns workspaces sorted by name.
func (m *Manager) List() ([]*Workspace, error) {
	d, err := m.read()
	if err != nil {
		return nil, err
	}
	out := make([]*Workspace, 0, len(d.Workspaces))
	for _, w := range d.Workspaces {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Manager) Get(name string) (*Workspace, error) {
	d, err := m.read()
	if err != nil {
		return nil, err
	}
	w, ok := d.Workspaces[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return w, nil
}

// ForPath returns the workspace whose worktree contains dir.
func (m *Manager) ForPath(dir string) (*Workspace, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	list, err := m.List()
	if err != nil {
		return nil, err
	}
	for _, w := range list {
		if abs == w.Path || strings.HasPrefix(abs, w.Path+string(os.PathSeparator)) {
			return w, nil
		}
	}
	return nil, fmt.Errorf("%w: no workspace contains %s", ErrNotFound, dir)
}

type CreateOptions struct {
	Name  string
	Base  string
	Owner string
	Log   io.Writer
}

// Create adds a worktree on a new seralith/<name> branch, copies configured untracked
// files, and runs setup commands.
func (m *Manager) Create(ctx context.Context, opts CreateOptions) (*Workspace, error) {
	if !validName.MatchString(opts.Name) {
		return nil, ErrInvalidName
	}
	if _, err := m.Get(opts.Name); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrExists, opts.Name)
	}
	repo := m.Project.Repo
	base := opts.Base
	if base == "" {
		base = "HEAD"
	}
	baseCommit, err := repo.ResolveCommit(ctx, base)
	if err != nil {
		return nil, fmt.Errorf("workspace: resolve base %q: %w", base, err)
	}
	path := filepath.Join(m.Project.WorktreeRoot(), opts.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	branch := BranchPrefix + opts.Name
	if err := repo.AddWorktree(ctx, path, branch, baseCommit); err != nil {
		return nil, fmt.Errorf("workspace: create worktree: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if err := m.bootstrap(ctx, path, opts.Log); err != nil {
		return nil, err
	}
	tree, err := (&gitx.Repo{Dir: path}).Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	now := m.Now()
	ws := &Workspace{
		Name:       opts.Name,
		Path:       path,
		Branch:     branch,
		BaseCommit: baseCommit,
		Owner:      opts.Owner,
		CreatedAt:  now,
		UpdatedAt:  now,
		Status:     StatusActive,
		Head:       baseCommit,
		Checkpoint: tree,
		Segments:   []provenance.Segment{},
	}
	baseTree, err := repo.CommitTree(ctx, baseCommit)
	if err != nil {
		return nil, err
	}
	if baseTree != tree {
		changes, err := ws.Repo().DiffTrees(ctx, baseTree, tree)
		if err != nil {
			return nil, err
		}
		ws.Segments = append(ws.Segments, provenance.Segment{
			ID:         NewID("seg"),
			Kind:       provenance.SegmentManual,
			Actor:      provenance.Actor{Kind: provenance.ActorHuman, ID: opts.Owner, Tool: "workspace-setup"},
			StartedAt:  now,
			EndedAt:    now,
			BaseTree:   baseTree,
			ResultTree: tree,
			Files:      changes,
		})
	}
	if err := m.update(func(d *db) error {
		if _, ok := d.Workspaces[ws.Name]; ok {
			return fmt.Errorf("%w: %s", ErrExists, ws.Name)
		}
		d.Workspaces[ws.Name] = ws
		return nil
	}); err != nil {
		return nil, err
	}
	m.log("workspace.created", "human:"+opts.Owner, ws.Name, map[string]string{"branch": branch, "base": baseCommit, "path": path})
	return ws, nil
}

func (m *Manager) bootstrap(ctx context.Context, path string, logw io.Writer) error {
	cfg := m.Project.Config
	if cfg == nil {
		return nil
	}
	for _, rel := range cfg.Workspaces.Copy {
		src := filepath.Join(m.Project.Root, rel)
		data, err := os.ReadFile(src)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		info, _ := os.Stat(src)
		if err := fsutil.WriteAtomic(filepath.Join(path, rel), data, info.Mode().Perm()); err != nil {
			return fmt.Errorf("workspace: copy %s: %w", rel, err)
		}
	}
	for _, line := range cfg.Workspaces.Setup {
		cmd := exec.CommandContext(ctx, "sh", "-c", line)
		cmd.Dir = path
		if logw != nil {
			cmd.Stdout, cmd.Stderr = logw, logw
		}
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("workspace: setup %q: %w", line, err)
		}
	}
	return nil
}

// Observe snapshots the workspace and, if it changed since the last checkpoint without
// a session running, records the difference as a manual segment by the owner.
func (m *Manager) Observe(ctx context.Context, name string) (*Workspace, string, error) {
	ws, err := m.Get(name)
	if err != nil {
		return nil, "", err
	}
	if ws, err = m.reconcileHead(ctx, ws); err != nil {
		return nil, "", err
	}
	tree, err := ws.Repo().Snapshot(ctx)
	if err != nil {
		return nil, "", err
	}
	if tree == ws.Checkpoint {
		return ws, tree, nil
	}
	changes, err := ws.Repo().DiffTrees(ctx, ws.Checkpoint, tree)
	if err != nil {
		return nil, "", err
	}
	now := m.Now()
	seg := provenance.Segment{
		ID:         NewID("seg"),
		Kind:       provenance.SegmentManual,
		Actor:      provenance.Actor{Kind: provenance.ActorHuman, ID: ws.Owner},
		StartedAt:  ws.UpdatedAt,
		EndedAt:    now,
		BaseTree:   ws.Checkpoint,
		ResultTree: tree,
		Files:      changes,
	}
	ws, err = m.AppendSegment(name, seg)
	if err != nil {
		return nil, "", err
	}
	m.log("segment.observed", "human:"+ws.Owner, ws.Name, map[string]any{"segment": seg.ID, "files": len(changes)})
	return ws, tree, nil
}

// reconcileHead handles commits made outside Seralith. Pending segments can no longer be
// bound to a commit, so they are discarded and the event is recorded in the ledger.
func (m *Manager) reconcileHead(ctx context.Context, ws *Workspace) (*Workspace, error) {
	head, err := ws.Repo().Head(ctx)
	if err != nil || head == ws.Head {
		return ws, err
	}
	tree, err := ws.Repo().CommitTree(ctx, head)
	if err != nil {
		return nil, err
	}
	discarded := len(ws.Segments)
	out, err := m.Settle(ws.Name, head, tree)
	if err != nil {
		return nil, err
	}
	m.log("workspace.external-commit", "human:"+ws.Owner, ws.Name, map[string]any{"from": ws.Head, "to": head, "discardedSegments": discarded})
	return out, nil
}

// AppendSegment records a segment and advances the checkpoint to its result tree.
func (m *Manager) AppendSegment(name string, seg provenance.Segment) (*Workspace, error) {
	var out *Workspace
	err := m.update(func(d *db) error {
		ws, ok := d.Workspaces[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		if seg.BaseTree != ws.Checkpoint {
			return fmt.Errorf("workspace: segment base %s does not match checkpoint %s", short(seg.BaseTree), short(ws.Checkpoint))
		}
		if seg.BaseTree != seg.ResultTree {
			ws.Segments = append(ws.Segments, seg)
		}
		ws.Checkpoint = seg.ResultTree
		ws.UpdatedAt = m.Now()
		out = ws
		return nil
	})
	return out, err
}

// Settle clears segments that are now bound to head and resets the checkpoint.
func (m *Manager) Settle(name, head, checkpoint string) (*Workspace, error) {
	var out *Workspace
	err := m.update(func(d *db) error {
		ws, ok := d.Workspaces[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		ws.Segments = []provenance.Segment{}
		ws.Head = head
		ws.Checkpoint = checkpoint
		ws.UpdatedAt = m.Now()
		out = ws
		return nil
	})
	return out, err
}

type RemoveOptions struct {
	Force        bool
	DeleteBranch bool
	Actor        string
}

// Remove deletes the worktree. Pending segments block removal unless forced.
func (m *Manager) Remove(ctx context.Context, name string, opts RemoveOptions) error {
	ws, err := m.Get(name)
	if err != nil {
		return err
	}
	if ws.Pending() && !opts.Force {
		return fmt.Errorf("workspace: %s has %d unattested segment(s); commit them or use --force", name, len(ws.Segments))
	}
	if _, err := os.Stat(ws.Path); err == nil {
		if err := m.Project.Repo.RemoveWorktree(ctx, ws.Path, opts.Force); err != nil {
			return err
		}
	}
	if opts.DeleteBranch {
		_ = m.Project.Repo.DeleteBranch(ctx, ws.Branch, opts.Force)
	}
	if err := m.update(func(d *db) error {
		delete(d.Workspaces, name)
		return nil
	}); err != nil {
		return err
	}
	m.log("workspace.removed", opts.Actor, name, map[string]any{"forced": opts.Force, "discardedSegments": len(ws.Segments)})
	return nil
}

func (m *Manager) log(typ, actor, subject string, data any) {
	if m.Ledger != nil {
		_, _ = m.Ledger.Append(typ, actor, subject, data)
	}
}

// NewID returns a sortable, prefixed random identifier.
func NewID(prefix string) string {
	var b [5]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s_%x%s", prefix, time.Now().UTC().UnixMilli(), hex.EncodeToString(b[:]))
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
