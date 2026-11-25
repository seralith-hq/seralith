package gitx_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/seralith-hq/seralith/internal/gitx"
	"github.com/seralith-hq/seralith/internal/testutil"
	"github.com/seralith-hq/seralith/pkg/provenance"
)

var ctx = context.Background()

func TestOpenNotRepository(t *testing.T) {
	if _, err := gitx.Open(t.TempDir()); err == nil {
		t.Fatal("opened a non-repository")
	}
}

func TestSnapshotDoesNotTouchIndex(t *testing.T) {
	g := testutil.NewGitRepo(t)
	g.Write("a.txt", "one\n")
	base := g.Commit("init")
	repo, err := gitx.Open(filepath.Join(g.Dir))
	if err != nil {
		t.Fatal(err)
	}
	baseTree, _ := repo.CommitTree(ctx, base)
	snap, err := repo.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap != baseTree {
		t.Fatalf("clean snapshot %s != commit tree %s", snap, baseTree)
	}
	g.Write("b.txt", "untracked\n")
	g.Write(".gitignore", "ignored.log\n")
	g.Write("ignored.log", "noise\n")
	snap2, err := repo.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := repo.ChangedPaths(ctx, snap, snap2)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != ".gitignore" || paths[1] != "b.txt" {
		t.Fatalf("changed paths = %v", paths)
	}
	if status := g.Git("status", "--porcelain"); status != "?? .gitignore\n?? b.txt" {
		t.Fatalf("snapshot modified index; status = %q", status)
	}
}

func TestSnapshotEmptyRepository(t *testing.T) {
	g := testutil.NewGitRepo(t)
	repo, _ := gitx.Open(g.Dir)
	snap, err := repo.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap != gitx.EmptyTree {
		t.Fatalf("empty snapshot = %s", snap)
	}
}

func TestDiffTrees(t *testing.T) {
	g := testutil.NewGitRepo(t)
	g.Write("keep.go", testutil.Lines("package a", "", "func A() {}", "func B() {}", "func C() {}"))
	g.Write("gone.txt", "bye\n")
	g.Write("move.txt", testutil.Lines("l1", "l2", "l3", "l4", "l5", "l6"))
	base := g.Commit("init")

	g.Write("keep.go", testutil.Lines("package a", "", "func A() {}", "func X() {}", "func Y() {}", "func C() {}"))
	if err := os.Remove(filepath.Join(g.Dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	g.Git("mv", "move.txt", "moved.txt")
	g.Write("new/file.txt", testutil.Lines("n1", "n2"))
	g.Write("bin.dat", "\x00\x01\x02")
	head := g.Commit("change")

	repo, _ := gitx.Open(g.Dir)
	changes, err := repo.DiffTrees(ctx, base, head)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]provenance.FileChange{}
	for _, c := range changes {
		byPath[c.Path] = c
	}
	keep := byPath["keep.go"]
	if keep.Status != provenance.FileModified || keep.LinesAdded != 2 || keep.LinesDeleted != 1 {
		t.Fatalf("keep.go = %+v", keep)
	}
	if len(keep.Added) != 1 || keep.Added[0] != (provenance.LineRange{Start: 4, End: 5}) {
		t.Fatalf("keep.go added = %+v", keep.Added)
	}
	if byPath["gone.txt"].Status != provenance.FileDeleted {
		t.Fatalf("gone.txt = %+v", byPath["gone.txt"])
	}
	if m := byPath["moved.txt"]; m.Status != provenance.FileRenamed || m.OldPath != "move.txt" {
		t.Fatalf("moved.txt = %+v", m)
	}
	if n := byPath["new/file.txt"]; n.Status != provenance.FileAdded || n.LinesAdded != 2 {
		t.Fatalf("new/file.txt = %+v", n)
	}
	if b := byPath["bin.dat"]; !b.Binary {
		t.Fatalf("bin.dat = %+v", b)
	}
	if none, _ := repo.DiffTrees(ctx, head, head); none != nil {
		t.Fatal("expected no changes for identical trees")
	}
}

func TestWorktrees(t *testing.T) {
	g := testutil.NewGitRepo(t)
	g.Write("a", "a\n")
	head := g.Commit("init")
	repo, _ := gitx.Open(g.Dir)
	wtPath := filepath.Join(filepath.Dir(g.Dir), "wt-feature")
	if err := repo.AddWorktree(ctx, wtPath, "feature", head); err != nil {
		t.Fatal(err)
	}
	list, err := repo.Worktrees(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[1].Branch != "feature" || list[1].Head != head {
		t.Fatalf("worktrees = %+v", list)
	}
	wt, err := gitx.Open(wtPath)
	if err != nil {
		t.Fatal(err)
	}
	common, _ := wt.CommonDir(ctx)
	if common != filepath.Join(g.Dir, ".git") {
		t.Fatalf("common dir = %s", common)
	}
	if err := repo.RemoveWorktree(ctx, wtPath, false); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteBranch(ctx, "feature", true); err != nil {
		t.Fatal(err)
	}
}

func TestCommitHelpers(t *testing.T) {
	g := testutil.NewGitRepo(t)
	g.Write("a", "1\n")
	c1 := g.Commit("one")
	g.Write("a", "2\n")
	c2 := g.Commit("two")
	repo, _ := gitx.Open(g.Dir)
	parents, err := repo.Parents(ctx, c2)
	if err != nil || len(parents) != 1 || parents[0] != c1 {
		t.Fatalf("parents = %v err=%v", parents, err)
	}
	list, err := repo.RevList(ctx, c1+".."+c2)
	if err != nil || len(list) != 1 || list[0] != c2 {
		t.Fatalf("revlist = %v err=%v", list, err)
	}
	if !repo.IsAncestor(ctx, c1, c2) || repo.IsAncestor(ctx, c2, c1) {
		t.Fatal("IsAncestor wrong")
	}
	data, err := repo.ShowFile(ctx, c1, "a")
	if err != nil || string(data) != "1\n" {
		t.Fatalf("ShowFile = %q err=%v", data, err)
	}
	tree, _ := repo.CommitTree(ctx, c2)
	if err := repo.UpdateRef(ctx, "refs/seralith/test", tree); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteRef(ctx, "refs/seralith/test"); err != nil {
		t.Fatal(err)
	}
	if repo.Config(ctx, "user.email") != "test@example.com" {
		t.Fatal("config lookup failed")
	}
}
