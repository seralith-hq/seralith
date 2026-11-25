// Package session runs commands inside workspaces and turns each run into an observed,
// attributed change segment.
package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/seralith-hq/seralith/internal/ledger"
	"github.com/seralith-hq/seralith/internal/workspace"
	"github.com/seralith-hq/seralith/pkg/provenance"
)

type State string

const (
	StateRunning State = "running"
	StateExited  State = "exited"
	StateFailed  State = "failed"
)

const historyLimit = 256 * 1024

var ErrBusy = errors.New("session: workspace already has a running session")

type Spec struct {
	Workspace string
	Actor     provenance.Actor
	Command   []string
	Env       []string
	Cols      uint16
	Rows      uint16
	PTY       bool
	Stdin     io.Reader
}

type Info struct {
	ID           string           `json:"id"`
	Workspace    string           `json:"workspace"`
	State        State            `json:"state"`
	Actor        provenance.Actor `json:"actor"`
	Command      []string         `json:"command"`
	StartedAt    time.Time        `json:"startedAt"`
	EndedAt      *time.Time       `json:"endedAt,omitempty"`
	ExitCode     *int             `json:"exitCode,omitempty"`
	Segment      string           `json:"segment,omitempty"`
	FilesChanged int              `json:"filesChanged"`
	LinesAdded   int              `json:"linesAdded"`
	LinesDeleted int              `json:"linesDeleted"`
	Error        string           `json:"error,omitempty"`
}

type Session struct {
	mu         sync.Mutex
	info       Info
	cmd        *exec.Cmd
	ptmx       *os.File
	stdin      io.WriteCloser
	pipeW      io.Closer
	transcript *os.File
	digest     hash.Hash
	history    []byte
	subs       map[int]chan []byte
	nextSub    int
	done       chan struct{}
	baseTree   string
}

func (s *Session) Info() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info
}

func (s *Session) Done() <-chan struct{} { return s.done }

// Write sends input to the running process.
func (s *Session) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.info.State != StateRunning {
		return 0, errors.New("session: not running")
	}
	if s.ptmx != nil {
		return s.ptmx.Write(p)
	}
	if s.stdin != nil {
		return s.stdin.Write(p)
	}
	return 0, errors.New("session: input not attached")
}

// Resize updates the terminal window size of a PTY session.
func (s *Session) Resize(cols, rows uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptmx == nil {
		return nil
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

// Signal delivers sig to the process.
func (s *Session) Signal(sig os.Signal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd == nil || s.cmd.Process == nil || s.info.State != StateRunning {
		return nil
	}
	return s.cmd.Process.Signal(sig)
}

// Subscribe returns buffered output so far and a channel of subsequent output. The
// channel closes when the session ends or cancel is called.
func (s *Session) Subscribe() ([]byte, <-chan []byte, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hist := append([]byte(nil), s.history...)
	ch := make(chan []byte, 256)
	if s.info.State != StateRunning {
		close(ch)
		return hist, ch, func() {}
	}
	id := s.nextSub
	s.nextSub++
	s.subs[id] = ch
	return hist, ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if c, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(c)
		}
	}
}

func (s *Session) publish(chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.transcript.Write(chunk)
	s.digest.Write(chunk)
	s.history = append(s.history, chunk...)
	if over := len(s.history) - historyLimit; over > 0 {
		s.history = s.history[over:]
	}
	for _, ch := range s.subs {
		cp := append([]byte(nil), chunk...)
		select {
		case ch <- cp:
		default:
		}
	}
}

// Manager tracks live sessions for a project.
type Manager struct {
	Workspaces  *workspace.Manager
	Ledger      *ledger.Ledger
	SessionsDir string
	OnChange    func(Info)

	mu       sync.Mutex
	sessions map[string]*Session
}

func NewManager(ws *workspace.Manager, l *ledger.Ledger, dir string) *Manager {
	return &Manager{Workspaces: ws, Ledger: l, SessionsDir: dir, sessions: map[string]*Session{}}
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// List returns known sessions, newest first.
func (m *Manager) List() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Info, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s.Info())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

func (m *Manager) busy(workspace string) bool {
	for _, s := range m.sessions {
		info := s.Info()
		if info.Workspace == workspace && info.State == StateRunning {
			return true
		}
	}
	return false
}

// Start launches spec. Only one session may run per workspace at a time so that every
// change in the worktree has exactly one attributable author.
func (m *Manager) Start(ctx context.Context, spec Spec) (*Session, error) {
	if len(spec.Command) == 0 {
		return nil, errors.New("session: empty command")
	}
	m.mu.Lock()
	if m.busy(spec.Workspace) {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrBusy, spec.Workspace)
	}
	placeholder := &Session{info: Info{Workspace: spec.Workspace, State: StateRunning}}
	reserve := workspace.NewID("reserve")
	m.sessions[reserve] = placeholder
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.sessions, reserve)
		m.mu.Unlock()
	}()

	ws, base, err := m.Workspaces.Observe(ctx, spec.Workspace)
	if err != nil {
		return nil, err
	}
	id := workspace.NewID("ses")
	dir := filepath.Join(m.SessionsDir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	transcript, err := os.OpenFile(filepath.Join(dir, "transcript.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(spec.Command[0], spec.Command[1:]...)
	cmd.Dir = ws.Path
	cmd.Env = append(append(os.Environ(),
		"SERALITH_SESSION="+id,
		"SERALITH_WORKSPACE="+ws.Name,
		"SERALITH_ACTOR="+spec.Actor.String(),
	), spec.Env...)

	s := &Session{
		info: Info{
			ID:        id,
			Workspace: ws.Name,
			State:     StateRunning,
			Actor:     spec.Actor,
			Command:   spec.Command,
			StartedAt: time.Now().UTC(),
		},
		cmd:        cmd,
		transcript: transcript,
		digest:     sha256.New(),
		subs:       map[int]chan []byte{},
		done:       make(chan struct{}),
		baseTree:   base,
	}

	var output io.Reader
	if spec.PTY {
		size := &pty.Winsize{Cols: orDefault(spec.Cols, 120), Rows: orDefault(spec.Rows, 32)}
		ptmx, err := pty.StartWithSize(cmd, size)
		if err != nil {
			_ = transcript.Close()
			return nil, fmt.Errorf("session: start %s: %w", spec.Command[0], err)
		}
		s.ptmx, output = ptmx, ptmx
	} else {
		pr, pw := io.Pipe()
		cmd.Stdout, cmd.Stderr = pw, pw
		if spec.Stdin != nil {
			cmd.Stdin = spec.Stdin
		} else if stdin, err := cmd.StdinPipe(); err == nil {
			s.stdin = stdin
		}
		if err := cmd.Start(); err != nil {
			_ = transcript.Close()
			_ = pw.Close()
			return nil, fmt.Errorf("session: start %s: %w", spec.Command[0], err)
		}
		s.pipeW, output = pw, pr
	}

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	m.log("session.started", spec.Actor.String(), ws.Name, map[string]any{"session": id, "command": spec.Command, "baseTree": base})
	m.notify(s.Info())

	pumped := make(chan struct{})
	go func() {
		defer close(pumped)
		buf := make([]byte, 32*1024)
		for {
			n, err := output.Read(buf)
			if n > 0 {
				s.publish(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	go m.supervise(s, spec.PTY, pumped)
	return s, nil
}

func (m *Manager) supervise(s *Session, isPTY bool, pumped <-chan struct{}) {
	var waitErr error
	if isPTY {
		waitErr = s.cmd.Wait()
		select {
		case <-pumped:
		case <-time.After(2 * time.Second):
		}
		_ = s.ptmx.Close()
	} else {
		waitErr = s.cmd.Wait()
		_ = s.pipeW.Close()
	}
	<-pumped

	code := exitCode(s.cmd, waitErr)
	end := time.Now().UTC()
	seg, err := m.finalize(s, code, end)

	s.mu.Lock()
	_ = s.transcript.Close()
	s.info.EndedAt = &end
	s.info.ExitCode = &code
	s.info.State = StateExited
	if err != nil {
		s.info.State = StateFailed
		s.info.Error = err.Error()
	}
	if seg != nil {
		s.info.Segment = seg.ID
		s.info.FilesChanged = len(seg.Files)
		for _, f := range seg.Files {
			s.info.LinesAdded += f.LinesAdded
			s.info.LinesDeleted += f.LinesDeleted
		}
	}
	for id, ch := range s.subs {
		close(ch)
		delete(s.subs, id)
	}
	info := s.info
	s.mu.Unlock()

	m.log("session.ended", info.Actor.String(), info.Workspace, map[string]any{
		"session": info.ID, "exitCode": code, "segment": info.Segment, "files": info.FilesChanged, "error": info.Error,
	})
	close(s.done)
	m.notify(info)
}

func (m *Manager) finalize(s *Session, code int, end time.Time) (*provenance.Segment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ws, err := m.Workspaces.Get(s.info.Workspace)
	if err != nil {
		return nil, err
	}
	result, err := ws.Repo().Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	changes, err := ws.Repo().DiffTrees(ctx, s.baseTree, result)
	if err != nil {
		return nil, err
	}
	seg := provenance.Segment{
		ID:               s.info.ID,
		Kind:             provenance.SegmentSession,
		Actor:            s.info.Actor,
		Command:          s.info.Command,
		StartedAt:        s.info.StartedAt,
		EndedAt:          end,
		BaseTree:         s.baseTree,
		ResultTree:       result,
		ExitCode:         &code,
		TranscriptDigest: provenance.DigestSet{"sha256": hex.EncodeToString(s.digest.Sum(nil))},
		Files:            changes,
	}
	if _, err := m.Workspaces.AppendSegment(ws.Name, seg); err != nil {
		return nil, err
	}
	return &seg, nil
}

func (m *Manager) log(typ, actor, subject string, data any) {
	if m.Ledger != nil {
		_, _ = m.Ledger.Append(typ, actor, subject, data)
	}
}

func (m *Manager) notify(info Info) {
	if m.OnChange != nil {
		m.OnChange(info)
	}
}

// Transcript opens the recorded output of a session.
func (m *Manager) Transcript(id string) (*os.File, error) {
	return os.Open(filepath.Join(m.SessionsDir, filepath.Base(id), "transcript.log"))
}

func exitCode(cmd *exec.Cmd, err error) int {
	if cmd.ProcessState != nil {
		if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

func orDefault(v, d uint16) uint16 {
	if v == 0 {
		return d
	}
	return v
}
