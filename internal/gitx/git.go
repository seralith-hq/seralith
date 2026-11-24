// Package gitx wraps the git command line for the operations Sigil relies on.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EmptyTree is the well-known hash of git's empty tree object.
const EmptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

var ErrNotRepository = errors.New("gitx: not a git repository")

// Error carries the stderr of a failed git invocation.
type Error struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.Args, " "), e.Err, strings.TrimSpace(e.Stderr))
}

func (e *Error) Unwrap() error { return e.Err }

// Repo is a git working tree rooted at Dir.
type Repo struct {
	Dir string
	Env []string
}

// Open resolves the top level of the working tree containing dir.
func Open(dir string) (*Repo, error) {
	r := &Repo{Dir: dir}
	top, err := r.Run(context.Background(), "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotRepository, dir)
	}
	return &Repo{Dir: top}, nil
}

// Run executes git with args and returns trimmed stdout.
func (r *Repo) Run(ctx context.Context, args ...string) (string, error) {
	out, err := r.RunRaw(ctx, nil, args...)
	return strings.TrimRight(string(out), "\n"), err
}

// RunRaw executes git with optional extra environment and returns raw stdout.
func (r *Repo) RunRaw(ctx context.Context, env []string, args ...string) ([]byte, error) {
	full := append([]string{"-c", "core.quotePath=false", "-c", "color.ui=never"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = r.Dir
	cmd.Env = append(append(os.Environ(), r.Env...), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), &Error{Args: args, Stderr: stderr.String(), Err: err}
	}
	return stdout.Bytes(), nil
}

// CommonDir returns the absolute path of the repository's shared git directory.
func (r *Repo) CommonDir(ctx context.Context) (string, error) {
	dir, err := r.Run(ctx, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(r.Dir, dir)
	}
	return filepath.Clean(dir), nil
}

func (r *Repo) Head(ctx context.Context) (string, error) {
	return r.Run(ctx, "rev-parse", "HEAD")
}

// HasCommits reports whether HEAD resolves to a commit.
func (r *Repo) HasCommits(ctx context.Context) bool {
	_, err := r.Run(ctx, "rev-parse", "--verify", "--quiet", "HEAD")
	return err == nil
}

func (r *Repo) Branch(ctx context.Context) (string, error) {
	return r.Run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
}

func (r *Repo) ResolveCommit(ctx context.Context, rev string) (string, error) {
	return r.Run(ctx, "rev-parse", "--verify", rev+"^{commit}")
}

// CommitTree returns the tree hash of a commit.
func (r *Repo) CommitTree(ctx context.Context, rev string) (string, error) {
	return r.Run(ctx, "rev-parse", rev+"^{tree}")
}

func (r *Repo) Parents(ctx context.Context, rev string) ([]string, error) {
	out, err := r.Run(ctx, "rev-list", "--parents", "-n", "1", rev)
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return nil, fmt.Errorf("gitx: no such commit %s", rev)
	}
	return fields[1:], nil
}

// RevList returns commits in range, oldest first.
func (r *Repo) RevList(ctx context.Context, rangeSpec string) ([]string, error) {
	out, err := r.Run(ctx, "rev-list", "--reverse", rangeSpec)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func (r *Repo) Config(ctx context.Context, key string) string {
	v, _ := r.Run(ctx, "config", "--get", key)
	return v
}

func (r *Repo) RemoteURL(ctx context.Context) string {
	return r.Config(ctx, "remote.origin.url")
}

// IsAncestor reports whether a is an ancestor of b.
func (r *Repo) IsAncestor(ctx context.Context, a, b string) bool {
	_, err := r.Run(ctx, "merge-base", "--is-ancestor", a, b)
	return err == nil
}

// UpdateRef points ref at an object, which may be a tree.
func (r *Repo) UpdateRef(ctx context.Context, ref, object string) error {
	_, err := r.Run(ctx, "update-ref", ref, object)
	return err
}

func (r *Repo) DeleteRef(ctx context.Context, ref string) error {
	_, err := r.Run(ctx, "update-ref", "-d", ref)
	return err
}

// ShowFile returns the contents of path at a tree-ish.
func (r *Repo) ShowFile(ctx context.Context, treeish, path string) ([]byte, error) {
	return r.RunRaw(ctx, nil, "cat-file", "blob", treeish+":"+path)
}
