// Package testutil provides fixtures shared by Sigil's tests.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// GitRepo is a throwaway repository for tests.
type GitRepo struct {
	t   testing.TB
	Dir string
}

// NewGitRepo initializes an empty repository on branch main with a test identity.
func NewGitRepo(t testing.TB) *GitRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &GitRepo{t: t, Dir: filepath.Join(dir, "repo")}
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r.Git("init", "-q", "-b", "main")
	r.Git("config", "user.name", "Test User")
	r.Git("config", "user.email", "test@example.com")
	r.Git("config", "commit.gpgsign", "false")
	return r
}

// Git runs a git command in the repository and returns trimmed output.
func (r *GitRepo) Git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// Write creates or replaces a file relative to the repository root.
func (r *GitRepo) Write(rel, content string) {
	r.t.Helper()
	WriteFile(r.t, filepath.Join(r.Dir, rel), content)
}

// Commit stages everything and commits, returning the new HEAD.
func (r *GitRepo) Commit(msg string) string {
	r.t.Helper()
	r.Git("add", "-A")
	r.Git("commit", "-q", "--allow-empty", "-m", msg)
	return r.Git("rev-parse", "HEAD")
}

// WriteFile writes content to path, creating parent directories.
func WriteFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Lines builds newline-terminated content from lines.
func Lines(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}
