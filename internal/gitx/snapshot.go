package gitx

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Snapshot records the full state of the working tree, including untracked files that
// are not ignored, as a tree object without touching the real index. The real index is
// copied first so git can reuse its stat cache instead of rehashing every file.
func (r *Repo) Snapshot(ctx context.Context) (string, error) {
	tmp, err := os.MkdirTemp("", "seralith-index-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	tmpIndex := filepath.Join(tmp, "index")
	env := []string{"GIT_INDEX_FILE=" + tmpIndex}

	seeded := false
	if real, err := r.Run(ctx, "rev-parse", "--path-format=absolute", "--git-path", "index"); err == nil {
		seeded = copyFile(real, tmpIndex) == nil
	}
	if !seeded && r.HasCommits(ctx) {
		if _, err := r.RunRaw(ctx, env, "read-tree", "HEAD"); err != nil {
			return "", fmt.Errorf("gitx: snapshot read-tree: %w", err)
		}
	}
	if _, err := r.RunRaw(ctx, env, "add", "-A", "--", "."); err != nil {
		return "", fmt.Errorf("gitx: snapshot add: %w", err)
	}
	out, err := r.RunRaw(ctx, env, "write-tree")
	if err != nil {
		return "", fmt.Errorf("gitx: snapshot write-tree: %w", err)
	}
	return string(trimNL(out)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func trimNL(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
