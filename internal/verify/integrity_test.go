package verify_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/seralith-hq/seralith/internal/attest"
	"github.com/seralith-hq/seralith/internal/keys"
	"github.com/seralith-hq/seralith/internal/notes"
	"github.com/seralith-hq/seralith/internal/testutil"
	"github.com/seralith-hq/seralith/internal/verify"
	"github.com/seralith-hq/seralith/internal/workspace"
	"github.com/seralith-hq/seralith/pkg/provenance"
)

func (f *fixture) agentCommit(t *testing.T, ws, path, content string) string {
	t.Helper()
	w, err := f.svc.Workspaces.Get(ws)
	if err != nil {
		if w, err = f.svc.Workspaces.Create(ctx, workspace.CreateOptions{Name: ws, Owner: "dev@example.com"}); err != nil {
			t.Fatal(err)
		}
	}
	seg := provenance.Segment{ID: "ses_" + ws + "_" + filepath.Base(path), Kind: provenance.SegmentSession, Actor: provenance.Actor{Kind: provenance.ActorAgent, ID: "codex", Tool: "codex"}}
	testutil.WriteFile(t, filepath.Join(w.Path, path), content)
	repo := w.Repo()
	tree, err := repo.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seg.BaseTree, seg.ResultTree = w.Checkpoint, tree
	if seg.Files, err = repo.DiffTrees(ctx, w.Checkpoint, tree); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Workspaces.AppendSegment(ws, seg); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.Commit(ctx, attest.CommitOptions{Workspace: ws, Message: "change " + path})
	if err != nil {
		t.Fatal(err)
	}
	return res.Commit
}

func (f *fixture) reviewer(t *testing.T) *attest.Service {
	t.Helper()
	key, _ := keys.Generate()
	if _, err := f.trust.Add(key.Public(), "rev@example.com", keys.KindHuman, "security"); err != nil {
		t.Fatal(err)
	}
	return &attest.Service{Workspaces: f.svc.Workspaces, Policy: f.pol, Signer: key, Principal: "rev@example.com"}
}

func problems(rep *verify.CommitReport) string { return strings.Join(rep.Problems, "; ") }

func TestApprovalForAnotherCommitIsRejected(t *testing.T) {
	f := newFixture(t)
	repo := f.svc.Workspaces.Project.Repo
	rev := f.reviewer(t)
	x := f.agentCommit(t, "a", "internal/crypto/old.go", "package crypto\n")
	approval, _, err := rev.Approve(ctx, repo, attest.ApproveOptions{Rev: x, Role: "security"})
	if err != nil {
		t.Fatal(err)
	}
	if rep, _ := verify.Commit(ctx, repo, x, f.opts()); rep.Status != verify.StatusPass {
		t.Fatalf("approved commit should pass: %s", problems(rep))
	}

	y := f.agentCommit(t, "b", "internal/crypto/new.go", "package crypto\n")
	if err := notes.Add(ctx, repo, y, approval); err != nil {
		t.Fatal(err)
	}
	for name, run := range map[string]func() *verify.CommitReport{
		"commit": func() *verify.CommitReport { rep, _ := verify.Commit(ctx, repo, y, f.opts()); return rep },
		"gate": func() *verify.CommitReport {
			rep, _ := verify.Gate(ctx, repo, x+".."+y, f.opts())
			return rep.Commits[len(rep.Commits)-1]
		},
	} {
		rep := run()
		if rep.Status != verify.StatusFail || rep.Approvals[0].Valid || rep.Approvals[0].Problem != "approval is for a different commit" {
			t.Fatalf("%s: replayed approval accepted: status=%s approvals=%+v", name, rep.Status, rep.Approvals)
		}
	}
}

func TestApprovalOnRangeTipCoversRange(t *testing.T) {
	f := newFixture(t)
	repo := f.svc.Workspaces.Project.Repo
	rev := f.reviewer(t)
	f.agentCommit(t, "a", "internal/crypto/one.go", "package crypto\n")
	tip := f.agentCommit(t, "a", "internal/crypto/two.go", "package crypto\n")
	if _, _, err := rev.Approve(ctx, repo, attest.ApproveOptions{Rev: tip, Role: "security"}); err != nil {
		t.Fatal(err)
	}
	rep, err := verify.Gate(ctx, repo, f.base+".."+tip, f.opts())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != verify.StatusPass {
		t.Fatalf("tip approval should cover the range: %+v", rep.Totals)
	}
}

func TestUndeclaredFilesAreDetected(t *testing.T) {
	f := newFixture(t)
	repo := f.svc.Workspaces.Project.Repo
	c := f.agentCommit(t, "a", "internal/crypto/evil.go", "package crypto\n")

	atts, _ := notes.Statements(ctx, repo, c)
	st := atts[provenance.PredicateChange][0].Statement
	pred, _ := st.Change()
	for i := range pred.Segments {
		pred.Segments[i].Files = []provenance.FileChange{{Path: "README.md", Status: provenance.FileModified, LinesAdded: 1}}
	}
	st.Predicate, _ = json.Marshal(pred)
	env, err := st.Seal(f.dev)
	if err != nil {
		t.Fatal(err)
	}
	f.g.Git("notes", "--ref="+notes.Ref, "remove", c)
	if err := notes.Add(ctx, repo, c, env); err != nil {
		t.Fatal(err)
	}
	rep, _ := verify.Commit(ctx, repo, c, f.opts())
	if rep.Status != verify.StatusFail || !strings.Contains(problems(rep), "change to internal/crypto/evil.go is not declared") {
		t.Fatalf("hidden change accepted: status=%s problems=%s", rep.Status, problems(rep))
	}
}

func TestUnderstatedLinesStillTriggerThresholds(t *testing.T) {
	f := newFixture(t)
	repo := f.svc.Workspaces.Project.Repo
	c := f.agentCommit(t, "a", "docs/big.txt", strings.Repeat("x\n", 1600))
	before, _ := verify.Commit(ctx, repo, c, f.opts())
	if len(before.Decisions) == 0 {
		t.Fatal("a 1600-line agent change should trigger agent-blast-radius")
	}

	atts, _ := notes.Statements(ctx, repo, c)
	st := atts[provenance.PredicateChange][0].Statement
	pred, _ := st.Change()
	for i := range pred.Segments {
		for j := range pred.Segments[i].Files {
			pred.Segments[i].Files[j].LinesAdded = 1
		}
	}
	st.Predicate, _ = json.Marshal(pred)
	env, _ := st.Seal(f.dev)
	f.g.Git("notes", "--ref="+notes.Ref, "remove", c)
	if err := notes.Add(ctx, repo, c, env); err != nil {
		t.Fatal(err)
	}
	after, _ := verify.Commit(ctx, repo, c, f.opts())
	if len(after.Decisions) != len(before.Decisions) || after.Status != before.Status {
		t.Fatalf("understated lines changed the outcome: before=%s %+v after=%s %+v", before.Status, before.Decisions, after.Status, after.Decisions)
	}
}

func TestMergeWithForeignContentIsNotSkipped(t *testing.T) {
	f := newFixture(t)
	repo := f.svc.Workspaces.Project.Repo
	f.g.Write("CHANGELOG", "x\n")
	second := f.g.Commit("second")
	f.g.Write("internal/crypto/backdoor.go", "package crypto\n")
	f.g.Git("add", "-A")
	tree := f.g.Git("write-tree")
	merge := f.g.Git("commit-tree", tree, "-p", second, "-p", f.base, "-m", "Merge branch 'main'")

	rep, err := verify.Gate(ctx, repo, second+".."+merge, f.opts())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != verify.StatusFail || rep.Totals[verify.StatusSkipped] != 0 {
		t.Fatalf("merge with foreign content passed: status=%s totals=%v", rep.Status, rep.Totals)
	}
	if !strings.Contains(problems(rep.Commits[0]), "not a clean merge") {
		t.Fatalf("problems = %s", problems(rep.Commits[0]))
	}
}

func TestHumanLabeledProtectedChangeNeedsIndependentApproval(t *testing.T) {
	f := newFixture(t)
	repo := f.svc.Workspaces.Project.Repo
	c := f.commit(t, "a", "internal/crypto/aes.go", "package crypto\n")
	rep, _ := verify.Commit(ctx, repo, c, f.opts())
	if rep.Status != verify.StatusFail || !strings.Contains(problems(rep), "security-critical-review") {
		t.Fatalf("human-labeled crypto change passed without review: %s", problems(rep))
	}
	if _, _, err := f.svc.Approve(ctx, repo, attest.ApproveOptions{Rev: c, Role: "security"}); err != nil {
		t.Fatal(err)
	}
	if rep, _ = verify.Commit(ctx, repo, c, f.opts()); rep.Status != verify.StatusFail {
		t.Fatal("self-approval satisfied review of a human-labeled change")
	}
	if _, _, err := f.reviewer(t).Approve(ctx, repo, attest.ApproveOptions{Rev: c, Role: "security"}); err != nil {
		t.Fatal(err)
	}
	if rep, _ = verify.Commit(ctx, repo, c, f.opts()); rep.Status != verify.StatusPass {
		t.Fatalf("independent approval not accepted: %s", problems(rep))
	}
}
