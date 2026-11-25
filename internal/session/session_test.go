package session_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/seralith-hq/seralith/internal/ledger"
	"github.com/seralith-hq/seralith/internal/project"
	"github.com/seralith-hq/seralith/internal/session"
	"github.com/seralith-hq/seralith/internal/testutil"
	"github.com/seralith-hq/seralith/internal/workspace"
	"github.com/seralith-hq/seralith/pkg/provenance"
)

var (
	ctx   = context.Background()
	agent = provenance.Actor{Kind: provenance.ActorAgent, ID: "test-agent", Tool: "test-agent", Operator: "dev@example.com"}
)

func setup(t *testing.T) (*session.Manager, *workspace.Workspace) {
	t.Helper()
	t.Setenv("SERALITH_DATA_DIR", t.TempDir())
	g := testutil.NewGitRepo(t)
	g.Write(".seralith/config.yaml", "version: 1\nname: demo\n")
	g.Write("app.txt", testutil.Lines("one", "two"))
	g.Commit("init")
	p, err := project.Load(ctx, g.Dir)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := ledger.Open(p.LedgerPath())
	wm := workspace.NewManager(p, l)
	ws, err := wm.Create(ctx, workspace.CreateOptions{Name: "w", Owner: "dev@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return session.NewManager(wm, l, p.SessionsDir()), ws
}

func wait(t *testing.T, s *session.Session) session.Info {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(20 * time.Second):
		t.Fatal("session did not finish")
	}
	return s.Info()
}

func TestPipeSessionProducesSegment(t *testing.T) {
	m, _ := setup(t)
	s, err := m.Start(ctx, session.Spec{
		Workspace: "w",
		Actor:     agent,
		Command:   []string{"sh", "-c", "echo working; printf 'one\\ntwo\\nthree\\n' > app.txt; printf 'x\\n' > new.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	info := wait(t, s)
	if info.State != session.StateExited || *info.ExitCode != 0 || info.FilesChanged != 2 || info.LinesAdded != 2 {
		t.Fatalf("info = %+v", info)
	}
	updated, _ := m.Workspaces.Get("w")
	if len(updated.Segments) != 1 {
		t.Fatalf("segments = %d", len(updated.Segments))
	}
	seg := updated.Segments[0]
	if seg.Kind != provenance.SegmentSession || seg.Actor.ID != "test-agent" || seg.TranscriptDigest["sha256"] == "" {
		t.Fatalf("segment = %+v", seg)
	}
	f, err := m.Transcript(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "working") {
		t.Fatalf("transcript = %q", data)
	}
	entries, _ := m.Ledger.Entries(ledger.Filter{Subject: "w"})
	types := []string{}
	for _, e := range entries {
		types = append(types, e.Type)
	}
	if strings.Join(types, ",") != "workspace.created,session.started,session.ended" {
		t.Fatalf("ledger types = %v", types)
	}
}

func TestPTYSessionAndExitCode(t *testing.T) {
	m, _ := setup(t)
	s, err := m.Start(ctx, session.Spec{
		Workspace: "w",
		Actor:     agent,
		Command:   []string{"sh", "-c", "echo from-pty; exit 3"},
		PTY:       true,
		Cols:      80,
		Rows:      24,
	})
	if err != nil {
		t.Fatal(err)
	}
	hist, ch, cancel := s.Subscribe()
	defer cancel()
	var out strings.Builder
	out.Write(hist)
	for chunk := range ch {
		out.Write(chunk)
	}
	info := wait(t, s)
	if *info.ExitCode != 3 || info.FilesChanged != 0 {
		t.Fatalf("info = %+v", info)
	}
	if !strings.Contains(out.String(), "from-pty") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestInteractiveInputAndBusy(t *testing.T) {
	m, _ := setup(t)
	s, err := m.Start(ctx, session.Spec{Workspace: "w", Actor: agent, Command: []string{"sh", "-c", "read line; echo \"$line\" > typed.txt"}, PTY: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(ctx, session.Spec{Workspace: "w", Actor: agent, Command: []string{"true"}}); !errors.Is(err, session.ErrBusy) {
		t.Fatalf("second session err = %v", err)
	}
	if err := s.Resize(100, 40); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := s.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	info := wait(t, s)
	if info.FilesChanged != 1 {
		t.Fatalf("info = %+v", info)
	}
	if len(m.List()) != 1 {
		t.Fatal("list should contain one session")
	}
}

func TestManualEditsBeforeSessionAreSeparate(t *testing.T) {
	m, ws := setup(t)
	testutil.WriteFile(t, filepath.Join(ws.Path, "human.txt"), "by hand\n")
	s, err := m.Start(ctx, session.Spec{Workspace: "w", Actor: agent, Command: []string{"sh", "-c", "echo agent > agent.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	wait(t, s)
	updated, _ := m.Workspaces.Get("w")
	if len(updated.Segments) != 2 {
		t.Fatalf("segments = %+v", updated.Segments)
	}
	if updated.Segments[0].Actor.Kind != provenance.ActorHuman || updated.Segments[0].Files[0].Path != "human.txt" {
		t.Fatalf("manual segment = %+v", updated.Segments[0])
	}
	if updated.Segments[1].Actor.Kind != provenance.ActorAgent || updated.Segments[1].Files[0].Path != "agent.txt" {
		t.Fatalf("session segment = %+v", updated.Segments[1])
	}
}
