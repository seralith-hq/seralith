// Package verify checks commits against attestations, trusted keys, and policy. It
// trusts nothing recorded in an attestation that it can recompute.
package verify

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/ryan-tobin/new-startup/internal/gitx"
	"github.com/ryan-tobin/new-startup/internal/keys"
	"github.com/ryan-tobin/new-startup/internal/notes"
	"github.com/ryan-tobin/new-startup/internal/policy"
	"github.com/ryan-tobin/new-startup/pkg/provenance"
)

type Status string

const (
	StatusPass    Status = "pass"
	StatusWarn    Status = "warn"
	StatusFail    Status = "fail"
	StatusSkipped Status = "skipped"
)

type Options struct {
	Trust  *keys.TrustStore
	Policy *policy.Policy
	// SkipMerges treats merge commits as covered by the attestations of their parents.
	SkipMerges bool
	// ExtraApprovals are approvals that apply to every commit, such as those on a range tip.
	ExtraApprovals []notes.Attestation
}

type Approval struct {
	Approver string                      `json:"approver"`
	Role     string                      `json:"role,omitempty"`
	Rules    []string                    `json:"rules,omitempty"`
	Decision provenance.ApprovalDecision `json:"decision"`
	Commit   string                      `json:"commit"`
	Valid    bool                        `json:"valid"`
	Problem  string                      `json:"problem,omitempty"`
}

type DecisionStatus struct {
	provenance.Decision
	Satisfied bool     `json:"satisfied"`
	Approvers []string `json:"approvers,omitempty"`
}

type CommitReport struct {
	Commit    string                      `json:"commit"`
	Subject   string                      `json:"subject,omitempty"`
	Status    Status                      `json:"status"`
	Attested  bool                        `json:"attested"`
	Signers   []string                    `json:"signers,omitempty"`
	Summary   *provenance.Summary         `json:"summary,omitempty"`
	Decisions []DecisionStatus            `json:"decisions,omitempty"`
	Approvals []Approval                  `json:"approvals,omitempty"`
	Problems  []string                    `json:"problems,omitempty"`
	Warnings  []string                    `json:"warnings,omitempty"`
	Predicate *provenance.ChangePredicate `json:"-"`
	Digest    string                      `json:"attestation,omitempty"`
}

func (r *CommitReport) fail(format string, args ...any) {
	r.Problems = append(r.Problems, fmt.Sprintf(format, args...))
}

// Commit verifies a single commit.
func Commit(ctx context.Context, repo *gitx.Repo, rev string, opts Options) (*CommitReport, error) {
	commit, err := repo.ResolveCommit(ctx, rev)
	if err != nil {
		return nil, err
	}
	rep := &CommitReport{Commit: commit}
	rep.Subject, _ = repo.Run(ctx, "log", "-1", "--format=%s", commit)
	parents, err := repo.Parents(ctx, commit)
	if err != nil {
		return nil, err
	}
	if opts.SkipMerges && len(parents) > 1 {
		rep.Status = StatusSkipped
		return rep, nil
	}
	atts, err := notes.Statements(ctx, repo, commit)
	if err != nil {
		return nil, err
	}
	change := pickChange(atts[provenance.PredicateChange], commit, opts.Trust, rep)
	if change == nil {
		if len(rep.Problems) == 0 {
			rep.fail("no change attestation")
		}
		rep.Status = StatusFail
		return rep, nil
	}
	rep.Attested = true
	pred, _ := change.Statement.Change()
	rep.Predicate = pred
	rep.Digest = provenance.Digest(change.Envelope)
	rep.Summary = &pred.Summary
	checkIntegrity(ctx, repo, commit, parents, pred, rep)

	res := opts.Policy.Evaluate(policy.ChangesFromSegments(pred.Segments))
	approvals := collectApprovals(append(slices.Clone(atts[provenance.PredicateApproval]), opts.ExtraApprovals...), opts.Trust, rep)
	for _, d := range res.Decisions {
		ds := DecisionStatus{Decision: d}
		switch d.Effect {
		case provenance.EffectDeny:
			rep.fail("policy %s denies change to %v", d.Rule, d.Paths)
		case provenance.EffectWarn:
			ds.Satisfied = true
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("policy %s: %s", d.Rule, d.Message))
		case provenance.EffectRequireApproval:
			rule, _ := opts.Policy.Rule(d.Rule)
			ds.Approvers, ds.Satisfied = satisfy(d, rule, approvals, pred.Committer.ID, rep)
			if !ds.Satisfied {
				rep.fail("policy %s requires %d approval(s)%s; have %d", d.Rule, d.Approvals, rolesSuffix(d.Roles), len(ds.Approvers))
			}
		default:
			ds.Satisfied = true
		}
		rep.Decisions = append(rep.Decisions, ds)
	}
	switch {
	case len(rep.Problems) > 0:
		rep.Status = StatusFail
	case len(rep.Warnings) > 0:
		rep.Status = StatusWarn
	default:
		rep.Status = StatusPass
	}
	return rep, nil
}

func pickChange(list []notes.Attestation, commit string, trust *keys.TrustStore, rep *CommitReport) *notes.Attestation {
	for i := range list {
		a := &list[i]
		if a.Statement.Commit() != commit {
			rep.fail("attestation subject %s does not match commit", a.Statement.Commit())
			continue
		}
		res, err := a.Envelope.Verify(trust, 1)
		if err != nil {
			rep.fail("change attestation signature not trusted: %v", err)
			continue
		}
		pred, err := a.Statement.Change()
		if err != nil {
			rep.fail("%v", err)
			continue
		}
		committerSigned := false
		for _, id := range res.Accepted {
			tk, _ := trust.Lookup(id)
			rep.Signers = append(rep.Signers, tk.Principal)
			if tk.Principal == pred.Committer.ID {
				committerSigned = true
			}
		}
		if !committerSigned {
			rep.fail("attestation is not signed by its committer %s", pred.Committer.ID)
			continue
		}
		rep.Problems = nil
		return a
	}
	return nil
}

func checkIntegrity(ctx context.Context, repo *gitx.Repo, commit string, parents []string, pred *provenance.ChangePredicate, rep *CommitReport) {
	if pred.Commit != commit {
		rep.fail("predicate commit %s does not match subject", pred.Commit)
	}
	tree, err := repo.CommitTree(ctx, commit)
	if err != nil {
		rep.fail("cannot read commit tree: %v", err)
		return
	}
	if pred.Tree != tree {
		rep.fail("attested tree %s does not match commit tree %s", pred.Tree, tree)
	}
	if !slices.Equal(pred.Parents, parents) {
		rep.fail("attested parents do not match commit parents")
	}
	expected := gitx.EmptyTree
	if len(parents) > 0 {
		if expected, err = repo.CommitTree(ctx, parents[0]); err != nil {
			rep.fail("cannot read parent tree: %v", err)
			return
		}
	}
	for i, seg := range pred.Segments {
		if seg.BaseTree != expected {
			rep.fail("segment %d (%s) does not continue from the previous state", i, seg.ID)
		}
		expected = seg.ResultTree
	}
	if expected != tree {
		rep.fail("segments end at %s but commit tree is %s; the commit contains unobserved changes", short(expected), short(tree))
	}
}

func collectApprovals(list []notes.Attestation, trust *keys.TrustStore, rep *CommitReport) []Approval {
	out := make([]Approval, 0, len(list))
	for _, a := range list {
		pred, err := a.Statement.Approval()
		if err != nil {
			continue
		}
		ap := Approval{Approver: pred.Approver.ID, Role: pred.Role, Rules: pred.Rules, Decision: pred.Decision, Commit: pred.Commit}
		res, err := a.Envelope.Verify(trust, 1)
		switch {
		case a.Statement.Commit() != pred.Commit:
			ap.Problem = "subject does not match predicate"
		case err != nil:
			ap.Problem = "signature not trusted"
		default:
			for _, id := range res.Accepted {
				tk, _ := trust.Lookup(id)
				switch {
				case tk.Principal != pred.Approver.ID:
					ap.Problem = "signed by a different principal"
				case tk.Kind != keys.KindHuman:
					ap.Problem = "approvals must be signed by a human key"
				case pred.Role != "" && !tk.HasRole(pred.Role):
					ap.Problem = "approver does not hold role " + pred.Role
				default:
					ap.Valid, ap.Problem = true, ""
				}
				if ap.Valid {
					break
				}
			}
		}
		out = append(out, ap)
	}
	rep.Approvals = out
	return out
}

func satisfy(d provenance.Decision, rule *policy.Rule, approvals []Approval, committer string, rep *CommitReport) ([]string, bool) {
	allowSelf := rule != nil && rule.AllowSelf
	approvers := map[string]bool{}
	for _, ap := range approvals {
		if !ap.Valid {
			continue
		}
		if len(ap.Rules) > 0 && !slices.Contains(ap.Rules, d.Rule) {
			continue
		}
		if len(d.Roles) > 0 && !slices.Contains(d.Roles, ap.Role) {
			continue
		}
		if ap.Approver == committer && !allowSelf {
			continue
		}
		if ap.Decision == provenance.Reject {
			rep.fail("policy %s: change rejected by %s", d.Rule, ap.Approver)
			return nil, false
		}
		approvers[ap.Approver] = true
	}
	list := make([]string, 0, len(approvers))
	for a := range approvers {
		list = append(list, a)
	}
	sort.Strings(list)
	return list, len(list) >= d.Approvals
}

type GateReport struct {
	Range   string          `json:"range"`
	Status  Status          `json:"status"`
	Commits []*CommitReport `json:"commits"`
	Totals  map[Status]int  `json:"totals"`
	Lines   map[string]int  `json:"lines"`
}

// Gate verifies every commit in rangeSpec. Approvals on the range tip apply to every
// commit in the range, matching how reviews are given on a pull request head.
func Gate(ctx context.Context, repo *gitx.Repo, rangeSpec string, opts Options) (*GateReport, error) {
	commits, err := repo.RevList(ctx, rangeSpec)
	if err != nil {
		return nil, err
	}
	rep := &GateReport{Range: rangeSpec, Status: StatusPass, Totals: map[Status]int{}, Lines: map[string]int{}}
	if len(commits) > 0 {
		tip := commits[len(commits)-1]
		atts, err := notes.Statements(ctx, repo, tip)
		if err != nil {
			return nil, err
		}
		opts.ExtraApprovals = atts[provenance.PredicateApproval]
	}
	for _, c := range commits {
		cr, err := Commit(ctx, repo, c, opts)
		if err != nil {
			return nil, err
		}
		if cr.Commit == commits[len(commits)-1] {
			cr.Approvals = dedupeApprovals(cr.Approvals)
		}
		rep.Commits = append(rep.Commits, cr)
		rep.Totals[cr.Status]++
		if cr.Summary != nil {
			for k, v := range cr.Summary.LinesByKind {
				rep.Lines[string(k)] += v
			}
		}
		switch {
		case cr.Status == StatusFail:
			rep.Status = StatusFail
		case cr.Status == StatusWarn && rep.Status == StatusPass:
			rep.Status = StatusWarn
		}
	}
	return rep, nil
}

func dedupeApprovals(list []Approval) []Approval {
	seen := map[string]bool{}
	var out []Approval
	for _, a := range list {
		key := a.Approver + "|" + a.Commit + "|" + string(a.Decision) + "|" + a.Role
		if !seen[key] {
			seen[key] = true
			out = append(out, a)
		}
	}
	return out
}

func rolesSuffix(roles []string) string {
	if len(roles) == 0 {
		return ""
	}
	return fmt.Sprintf(" from role %v", roles)
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
