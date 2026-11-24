package notes_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ryan-tobin/new-startup/internal/gitx"
	"github.com/ryan-tobin/new-startup/internal/keys"
	"github.com/ryan-tobin/new-startup/internal/notes"
	"github.com/ryan-tobin/new-startup/internal/testutil"
	"github.com/ryan-tobin/new-startup/pkg/provenance"
)

var ctx = context.Background()

func envelope(t *testing.T, commit, predicateType string, pred any) *notes.Attestation {
	t.Helper()
	k, _ := keys.Generate()
	st, err := provenance.NewStatement(commit, predicateType, pred)
	if err != nil {
		t.Fatal(err)
	}
	env, err := st.Seal(k)
	if err != nil {
		t.Fatal(err)
	}
	return &notes.Attestation{Envelope: env, Statement: st}
}

func TestAddReadAndDedupe(t *testing.T) {
	g := testutil.NewGitRepo(t)
	g.Write("a", "a\n")
	commit := g.Commit("init")
	repo, _ := gitx.Open(g.Dir)

	if envs, err := notes.Read(ctx, repo, commit); err != nil || len(envs) != 0 {
		t.Fatalf("empty read: %v %v", envs, err)
	}
	change := envelope(t, commit, provenance.PredicateChange, provenance.ChangePredicate{Commit: commit})
	approval := envelope(t, commit, provenance.PredicateApproval, provenance.ApprovalPredicate{Commit: commit, Decision: provenance.Approve})
	for _, a := range []*notes.Attestation{change, approval, change} {
		if err := notes.Add(ctx, repo, commit, a.Envelope); err != nil {
			t.Fatal(err)
		}
	}
	grouped, err := notes.Statements(ctx, repo, commit)
	if err != nil {
		t.Fatal(err)
	}
	if len(grouped[provenance.PredicateChange]) != 1 || len(grouped[provenance.PredicateApproval]) != 1 {
		t.Fatalf("grouped = %v", grouped)
	}
}

func TestPushFetchMergesNotes(t *testing.T) {
	origin := testutil.NewGitRepo(t)
	origin.Write("a", "a\n")
	commit := origin.Commit("init")
	clone := filepath.Join(t.TempDir(), "clone")
	origin.Git("clone", "-q", origin.Dir, clone)
	upstream, _ := gitx.Open(origin.Dir)
	local, _ := gitx.Open(clone)

	a := envelope(t, commit, provenance.PredicateChange, provenance.ChangePredicate{Commit: commit})
	b := envelope(t, commit, provenance.PredicateApproval, provenance.ApprovalPredicate{Commit: commit})
	if err := notes.Add(ctx, upstream, commit, a.Envelope); err != nil {
		t.Fatal(err)
	}
	if err := notes.Add(ctx, local, commit, b.Envelope); err != nil {
		t.Fatal(err)
	}
	if err := notes.Fetch(ctx, local, "origin"); err != nil {
		t.Fatal(err)
	}
	envs, err := notes.Read(ctx, local, commit)
	if err != nil || len(envs) != 2 {
		t.Fatalf("merged envelopes = %d err=%v", len(envs), err)
	}
}
