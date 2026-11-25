package verify_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/seralith-hq/seralith/internal/attest"
	"github.com/seralith-hq/seralith/internal/keys"
	"github.com/seralith-hq/seralith/internal/ledger"
	"github.com/seralith-hq/seralith/internal/notes"
	"github.com/seralith-hq/seralith/internal/policy"
	"github.com/seralith-hq/seralith/internal/project"
	"github.com/seralith-hq/seralith/internal/testutil"
	"github.com/seralith-hq/seralith/internal/verify"
	"github.com/seralith-hq/seralith/internal/workspace"
	"github.com/seralith-hq/seralith/pkg/provenance"
)

var ctx = context.Background()

type fixture struct {
	g     *testutil.GitRepo
	svc   *attest.Service
	dev   *keys.PrivateKey
	trust *keys.TrustStore
	pol   *policy.Policy
	base  string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("SERALITH_DATA_DIR", t.TempDir())
	g := testutil.NewGitRepo(t)
	g.Write(".seralith/config.yaml", "version: 1\nname: demo\n")
	g.Write("README.md", "demo\n")
	base := g.Commit("init")
	p, err := project.Load(ctx, g.Dir)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := ledger.Open(p.LedgerPath())
	pol, _ := policy.Parse([]byte(policy.Default))
	dev, _ := keys.Generate()
	ts := keys.NewTrustStore()
	if _, err := ts.Add(dev.Public(), "dev@example.com", keys.KindHuman, "security"); err != nil {
		t.Fatal(err)
	}
	svc := &attest.Service{Workspaces: workspace.NewManager(p, l), Ledger: l, Policy: pol, Signer: dev, Principal: "dev@example.com"}
	return &fixture{g: g, svc: svc, dev: dev, trust: ts, pol: pol, base: base}
}

func (f *fixture) commit(t *testing.T, ws, path, content string) string {
	t.Helper()
	w, err := f.svc.Workspaces.Get(ws)
	if err != nil {
		if w, err = f.svc.Workspaces.Create(ctx, workspace.CreateOptions{Name: ws, Owner: "dev@example.com"}); err != nil {
			t.Fatal(err)
		}
	}
	testutil.WriteFile(t, filepath.Join(w.Path, path), content)
	res, err := f.svc.Commit(ctx, attest.CommitOptions{Workspace: ws, Message: "change " + path})
	if err != nil {
		t.Fatal(err)
	}
	return res.Commit
}

func (f *fixture) opts() verify.Options {
	return verify.Options{Trust: f.trust, Policy: f.pol, SkipMerges: true}
}

func TestCopiedAttestationIsRejected(t *testing.T) {
	f := newFixture(t)
	good := f.commit(t, "a", "a.txt", "a\n")
	other := f.commit(t, "b", "b.txt", "b\n")

	envs, err := notes.Read(ctx, f.svc.Workspaces.Project.Repo, good)
	if err != nil || len(envs) != 1 {
		t.Fatalf("envs=%d err=%v", len(envs), err)
	}
	f.g.Git("notes", "--ref="+notes.Ref, "remove", other)
	if err := notes.Add(ctx, f.svc.Workspaces.Project.Repo, other, envs[0]); err != nil {
		t.Fatal(err)
	}
	rep, err := verify.Commit(ctx, f.svc.Workspaces.Project.Repo, other, f.opts())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != verify.StatusFail || !strings.Contains(strings.Join(rep.Problems, ";"), "does not match commit") {
		t.Fatalf("report = %+v", rep)
	}
}

func TestSignerMustBeCommitter(t *testing.T) {
	f := newFixture(t)
	c := f.commit(t, "a", "a.txt", "a\n")
	other, _ := keys.Generate()
	if _, err := f.trust.Add(other.Public(), "someone-else@example.com", keys.KindHuman); err != nil {
		t.Fatal(err)
	}
	repo := f.svc.Workspaces.Project.Repo
	atts, _ := notes.Statements(ctx, repo, c)
	resigned, err := atts[provenance.PredicateChange][0].Statement.Seal(other)
	if err != nil {
		t.Fatal(err)
	}
	f.g.Git("notes", "--ref="+notes.Ref, "remove", c)
	if err := notes.Add(ctx, repo, c, resigned); err != nil {
		t.Fatal(err)
	}
	rep, _ := verify.Commit(ctx, repo, c, f.opts())
	if rep.Status != verify.StatusFail || !strings.Contains(strings.Join(rep.Problems, ";"), "not signed by its committer") {
		t.Fatalf("report = %+v", rep)
	}
}

func TestSelfApprovalDoesNotSatisfyReview(t *testing.T) {
	f := newFixture(t)
	f.svc.Principal = "dev@example.com"
	w, _ := f.svc.Workspaces.Create(ctx, workspace.CreateOptions{Name: "sec", Owner: "dev@example.com"})
	sessionSeg := provenance.Segment{ID: "ses_x", Kind: provenance.SegmentSession, Actor: provenance.Actor{Kind: provenance.ActorAgent, ID: "codex", Tool: "codex"}}
	testutil.WriteFile(t, filepath.Join(w.Path, "internal/crypto/k.go"), "package crypto\n")
	repo := w.Repo()
	tree, _ := repo.Snapshot(ctx)
	sessionSeg.BaseTree, sessionSeg.ResultTree = w.Checkpoint, tree
	sessionSeg.Files, _ = repo.DiffTrees(ctx, w.Checkpoint, tree)
	if _, err := f.svc.Workspaces.AppendSegment("sec", sessionSeg); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.Commit(ctx, attest.CommitOptions{Workspace: "sec", Message: "feat: crypto"})
	if err != nil {
		t.Fatal(err)
	}
	main := f.svc.Workspaces.Project.Repo
	if _, _, err := f.svc.Approve(ctx, main, attest.ApproveOptions{Rev: res.Commit, Role: "security"}); err != nil {
		t.Fatal(err)
	}
	rep, _ := verify.Commit(ctx, main, res.Commit, f.opts())
	if rep.Status != verify.StatusFail {
		t.Fatalf("self-approval accepted: %+v", rep)
	}
	if !rep.Approvals[0].Valid {
		t.Fatalf("approval itself should be valid, only not independent: %+v", rep.Approvals)
	}
}

func TestGateRangeAndMergeSkipping(t *testing.T) {
	f := newFixture(t)
	f.commit(t, "a", "a.txt", "a\n")
	f.commit(t, "a", "a2.txt", "a2\n")
	f.g.Git("merge", "-q", "--no-ff", "-m", "merge a", "seralith/a")
	rep, err := verify.Gate(ctx, f.svc.Workspaces.Project.Repo, f.base+"..HEAD", f.opts())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != verify.StatusPass || rep.Totals[verify.StatusPass] != 2 || rep.Totals[verify.StatusSkipped] != 1 {
		t.Fatalf("gate = %+v", rep.Totals)
	}
	strict := f.opts()
	strict.SkipMerges = false
	rep, _ = verify.Gate(ctx, f.svc.Workspaces.Project.Repo, f.base+"..HEAD", strict)
	if rep.Status != verify.StatusFail {
		t.Fatal("unattested merge should fail when merges are included")
	}
}
