// Package notes stores attestations alongside commits in git notes.
package notes

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ryan-tobin/new-startup/internal/gitx"
	"github.com/ryan-tobin/new-startup/pkg/dsse"
	"github.com/ryan-tobin/new-startup/pkg/provenance"
)

const Ref = "refs/notes/sigil"

// Read returns every envelope attached to commit. Malformed lines are skipped.
func Read(ctx context.Context, repo *gitx.Repo, commit string) ([]*dsse.Envelope, error) {
	out, err := repo.RunRaw(ctx, nil, "notes", "--ref="+Ref, "show", commit)
	if err != nil {
		var gerr *gitx.Error
		if errors.As(err, &gerr) && strings.Contains(gerr.Stderr, "no note found") {
			return nil, nil
		}
		return nil, err
	}
	var envs []*dsse.Envelope
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		env, err := dsse.Parse(line)
		if err != nil {
			continue
		}
		envs = append(envs, env)
	}
	return envs, sc.Err()
}

// Add attaches env to commit, replacing any envelope with the same payload digest.
func Add(ctx context.Context, repo *gitx.Repo, commit string, env *dsse.Envelope) error {
	existing, err := Read(ctx, repo, commit)
	if err != nil {
		return err
	}
	digest := provenance.Digest(env)
	var buf bytes.Buffer
	for _, e := range existing {
		if provenance.Digest(e) == digest {
			continue
		}
		raw, err := e.Marshal()
		if err != nil {
			return err
		}
		buf.Write(raw)
		buf.WriteByte('\n')
	}
	raw, err := env.Marshal()
	if err != nil {
		return err
	}
	buf.Write(raw)
	buf.WriteByte('\n')

	tmp, err := os.MkdirTemp("", "sigil-note-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	file := filepath.Join(tmp, "note")
	if err := os.WriteFile(file, buf.Bytes(), 0o600); err != nil {
		return err
	}
	_, err = repo.Run(ctx, "notes", "--ref="+Ref, "add", "-f", "-F", file, commit)
	return err
}

// Statements decodes envelopes on commit, grouped by predicate type.
func Statements(ctx context.Context, repo *gitx.Repo, commit string) (map[string][]Attestation, error) {
	envs, err := Read(ctx, repo, commit)
	if err != nil {
		return nil, err
	}
	out := map[string][]Attestation{}
	for _, env := range envs {
		st, err := provenance.Open(env)
		if err != nil {
			continue
		}
		out[st.PredicateType] = append(out[st.PredicateType], Attestation{Envelope: env, Statement: st})
	}
	return out, nil
}

type Attestation struct {
	Envelope  *dsse.Envelope
	Statement *provenance.Statement
}

// Push publishes the notes ref to remote.
func Push(ctx context.Context, repo *gitx.Repo, remote string) error {
	_, err := repo.Run(ctx, "push", remote, Ref+":"+Ref)
	return err
}

// Fetch merges the remote notes ref into the local one.
func Fetch(ctx context.Context, repo *gitx.Repo, remote string) error {
	if _, err := repo.Run(ctx, "fetch", remote, "+"+Ref+":refs/notes/sigil-remote"); err != nil {
		return err
	}
	defer func() { _ = repo.DeleteRef(ctx, "refs/notes/sigil-remote") }()
	if _, err := repo.Run(ctx, "rev-parse", "--verify", "--quiet", Ref); err != nil {
		return repo.UpdateRef(ctx, Ref, "refs/notes/sigil-remote")
	}
	_, err := repo.Run(ctx, "notes", "--ref="+Ref, "merge", "-s", "cat_sort_uniq", "refs/notes/sigil-remote")
	return err
}
