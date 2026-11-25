package workspace_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryan-tobin/new-startup/internal/ledger"
	"github.com/ryan-tobin/new-startup/internal/project"
	"github.com/ryan-tobin/new-startup/internal/testutil"
	"github.com/ryan-tobin/new-startup/internal/workspace"
	"github.com/ryan-tobin/new-startup/pkg/provenance"
)

var ctx = context.Background()

func setup(t *testing.T, cfg string) (*testutil.GitRepo, *workspace.Manager) {
	t.Helper()
	t.Setenv("SERALITH_DATA_DIR", t.TempDir())
	g := testutil.NewGitRepo(t)
	g.Write(".seralith/config.yaml", cfg)
	g.Write("main.go", testutil.Lines("package main", "", "func main() {}"))
	g.Commit("init")
	p, err := project.Load(ctx, g.Dir)
	if err != nil {
		t.Fatal(err)
	}
	l, err := ledger.Open(p.LedgerPath())
	if err != nil {
		t.Fatal(err)
	}
	return g, workspace.NewManager(p, l)
}

func TestCreateListRemove(t *testing.T) {
	g, m := setup(t, "version: 1\nname: demo\n")
	ws, err := m.Create(ctx, workspace.CreateOptions{Name: "feature-a", Owner: "test@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if ws.Branch != "seralith/feature-a" || ws.Pending() {
		t.Fatalf("workspace = %+v", ws)
	}
	if _, err := os.Stat(filepath.Join(ws.Path, "main.go")); err != nil {
		t.Fatal("worktree missing checkout")
	}
	if _, err := m.Create(ctx, workspace.CreateOptions{Name: "feature-a", Owner: "x"}); !errors.Is(err, workspace.ErrExists) {
		t.Fatalf("duplicate create err = %v", err)
	}
	if _, err := m.Create(ctx, workspace.CreateOptions{Name: "Bad Name"}); !errors.Is(err, workspace.ErrInvalidName) {
		t.Fatalf("invalid name err = %v", err)
	}
	found, err := m.ForPath(filepath.Join(ws.Path, "sub", "dir"))
	if err != nil || found.Name != "feature-a" {
		t.Fatalf("ForPath = %v, %v", found, err)
	}
	list, _ := m.List()
	if len(list) != 1 {
		t.Fatalf("list = %d", len(list))
	}
	if err := m.Remove(ctx, "feature-a", workspace.RemoveOptions{DeleteBranch: true}); err != nil {
		t.Fatal(err)
	}
	if out := g.Git("branch", "--list", "seralith/feature-a"); out != "" {
		t.Fatalf("branch not deleted: %q", out)
	}
	entries, _ := m.Ledger.Entries(ledger.Filter{})
	if len(entries) != 2 || entries[0].Type != "workspace.created" || entries[1].Type != "workspace.removed" {
		t.Fatalf("ledger = %+v", entries)
	}
}

func TestObserveManualEdits(t *testing.T) {
	_, m := setup(t, "version: 1\nname: demo\n")
	if _, err := m.Create(ctx, workspace.CreateOptions{Name: "w", Owner: "dev@example.com"}); err != nil {
		t.Fatal(err)
	}
	ws, tree, err := m.Observe(ctx, "w")
	if err != nil || ws.Pending() || tree != ws.Checkpoint {
		t.Fatalf("clean observe: pending=%v err=%v", ws.Pending(), err)
	}
	testutil.WriteFile(t, filepath.Join(ws.Path, "main.go"), testutil.Lines("package main", "", "// edited", "func main() {}"))
	ws, tree, err = m.Observe(ctx, "w")
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Segments) != 1 || ws.Checkpoint != tree {
		t.Fatalf("segments = %+v", ws.Segments)
	}
	seg := ws.Segments[0]
	if seg.Kind != provenance.SegmentManual || seg.Actor.ID != "dev@example.com" || seg.Files[0].LinesAdded != 1 {
		t.Fatalf("segment = %+v", seg)
	}
	if err := m.Remove(ctx, "w", workspace.RemoveOptions{}); err == nil || !strings.Contains(err.Error(), "unattested") {
		t.Fatalf("remove with pending err = %v", err)
	}
	if err := m.Remove(ctx, "w", workspace.RemoveOptions{Force: true, DeleteBranch: true}); err != nil {
		t.Fatal(err)
	}
}

func TestAppendSegmentRejectsStaleBase(t *testing.T) {
	_, m := setup(t, "version: 1\nname: demo\n")
	if _, err := m.Create(ctx, workspace.CreateOptions{Name: "w", Owner: "dev"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AppendSegment("w", provenance.Segment{ID: "s", BaseTree: "deadbeef", ResultTree: "cafe"}); err == nil {
		t.Fatal("accepted segment with stale base")
	}
}

func TestExternalCommitDiscardsPending(t *testing.T) {
	_, m := setup(t, "version: 1\nname: demo\n")
	ws, _ := m.Create(ctx, workspace.CreateOptions{Name: "w", Owner: "dev"})
	testutil.WriteFile(t, filepath.Join(ws.Path, "x.txt"), "x\n")
	if _, _, err := m.Observe(ctx, "w"); err != nil {
		t.Fatal(err)
	}
	repo := ws.Repo()
	if _, err := repo.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Run(ctx, "commit", "-q", "-m", "outside seralith"); err != nil {
		t.Fatal(err)
	}
	ws, _, err := m.Observe(ctx, "w")
	if err != nil {
		t.Fatal(err)
	}
	if ws.Pending() {
		t.Fatalf("pending after external commit: %+v", ws.Segments)
	}
	entries, _ := m.Ledger.Entries(ledger.Filter{Type: "workspace.external-commit"})
	if len(entries) != 1 {
		t.Fatal("external commit not recorded")
	}
}

func TestBootstrapCopiesFilesAndRunsSetup(t *testing.T) {
	g, m := setup(t, "version: 1\nname: demo\nworkspaces:\n  copy: [.env.local]\n  setup: [\"echo ready > .setup-done\"]\n")
	g.Write(".gitignore", ".env.local\n.setup-done\n")
	g.Commit("ignore")
	g.Write(".env.local", "TOKEN=abc\n")
	ws, err := m.Create(ctx, workspace.CreateOptions{Name: "boot", Owner: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(ws.Path, ".env.local"))
	if err != nil || string(data) != "TOKEN=abc\n" {
		t.Fatalf("copied file = %q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(ws.Path, ".setup-done")); err != nil {
		t.Fatal("setup command did not run")
	}
	if ws.Pending() {
		t.Fatal("ignored bootstrap output should not create segments")
	}
}
