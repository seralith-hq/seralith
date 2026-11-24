// Package attest binds observed workspace segments to commits as signed attestations.
package attest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ryan-tobin/new-startup/internal/attribution"
	"github.com/ryan-tobin/new-startup/internal/gitx"
	"github.com/ryan-tobin/new-startup/internal/ledger"
	"github.com/ryan-tobin/new-startup/internal/notes"
	"github.com/ryan-tobin/new-startup/internal/policy"
	"github.com/ryan-tobin/new-startup/internal/version"
	"github.com/ryan-tobin/new-startup/internal/workspace"
	"github.com/ryan-tobin/new-startup/pkg/dsse"
	"github.com/ryan-tobin/new-startup/pkg/provenance"
)

var ErrDenied = errors.New("attest: policy denies this change")

type Service struct {
	Workspaces *workspace.Manager
	Ledger     *ledger.Ledger
	Policy     *policy.Policy
	Signer     dsse.Signer
	Principal  string
	Now        func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

// Preview is the evaluation of a workspace's pending segments before committing.
type Preview struct {
	Workspace   *workspace.Workspace
	Tree        string
	Segments    []provenance.Segment
	Attribution []provenance.FileAttribution
	Summary     provenance.Summary
	Policy      *provenance.PolicyResult
	Effect      provenance.PolicyEffect
}

// Preview observes the workspace and evaluates policy over its pending segments.
func (s *Service) Preview(ctx context.Context, name string) (*Preview, error) {
	ws, tree, err := s.Workspaces.Observe(ctx, name)
	if err != nil {
		return nil, err
	}
	tr := attribution.NewTracker()
	for _, seg := range ws.Segments {
		tr.Apply(seg.ID, seg.Files)
	}
	attr := tr.Result()
	res := s.Policy.Evaluate(policy.ChangesFromSegments(ws.Segments))
	return &Preview{
		Workspace:   ws,
		Tree:        tree,
		Segments:    ws.Segments,
		Attribution: attr,
		Summary:     attribution.Summarize(ws.Segments, attr),
		Policy:      res,
		Effect:      policy.Strongest(res.Decisions),
	}, nil
}

type CommitOptions struct {
	Workspace string
	Message   string
	Force     bool
}

type CommitResult struct {
	Commit    string
	Envelope  *dsse.Envelope
	Predicate *provenance.ChangePredicate
	Digest    string
}

// Commit stages the whole workspace, commits it, and attaches a signed change
// attestation describing every segment since the previous commit.
func (s *Service) Commit(ctx context.Context, opts CommitOptions) (*CommitResult, error) {
	pv, err := s.Preview(ctx, opts.Workspace)
	if err != nil {
		return nil, err
	}
	if len(pv.Segments) == 0 {
		return nil, errors.New("attest: nothing to commit")
	}
	if pv.Effect == provenance.EffectDeny {
		if !opts.Force {
			return nil, ErrDenied
		}
		s.log("policy.override", "human:"+s.Principal, pv.Workspace.Name, pv.Policy)
	}
	repo := pv.Workspace.Repo()
	if _, err := repo.Run(ctx, "add", "-A"); err != nil {
		return nil, err
	}
	args := []string{"commit", "-q", "-m", opts.Message, "--trailer", "Sigil-Workspace: " + pv.Workspace.Name}
	if _, err := repo.Run(ctx, args...); err != nil {
		return nil, fmt.Errorf("attest: git commit: %w", err)
	}
	commit, err := repo.Head(ctx)
	if err != nil {
		return nil, err
	}
	tree, err := repo.CommitTree(ctx, commit)
	if err != nil {
		return nil, err
	}
	if tree != pv.Tree {
		return nil, fmt.Errorf("attest: committed tree %s differs from observed tree %s", tree, pv.Tree)
	}
	parents, err := repo.Parents(ctx, commit)
	if err != nil {
		return nil, err
	}
	branch, _ := repo.Branch(ctx)
	pred := &provenance.ChangePredicate{
		Repository:  s.Workspaces.Project.Repo.RemoteURL(ctx),
		Commit:      commit,
		Tree:        tree,
		Parents:     parents,
		Workspace:   pv.Workspace.Name,
		Branch:      branch,
		Committer:   provenance.Actor{Kind: provenance.ActorHuman, ID: s.Principal},
		Segments:    pv.Segments,
		Attribution: pv.Attribution,
		Summary:     pv.Summary,
		Policy:      pv.Policy,
		Builder:     provenance.Builder{ID: provenance.BuilderID, Version: version.Version},
		CreatedAt:   s.now(),
	}
	env, err := s.seal(commit, provenance.PredicateChange, pred)
	if err != nil {
		return nil, err
	}
	if err := notes.Add(ctx, repo, commit, env); err != nil {
		return nil, fmt.Errorf("attest: store attestation: %w", err)
	}
	if _, err := s.Workspaces.Settle(pv.Workspace.Name, commit, tree); err != nil {
		return nil, err
	}
	digest := provenance.Digest(env)
	s.log("commit.attested", "human:"+s.Principal, commit, map[string]any{
		"workspace": pv.Workspace.Name, "attestation": digest, "segments": len(pv.Segments),
		"linesByKind": pv.Summary.LinesByKind, "effect": pv.Effect,
	})
	return &CommitResult{Commit: commit, Envelope: env, Predicate: pred, Digest: digest}, nil
}

type ApproveOptions struct {
	Rev     string
	Role    string
	Rules   []string
	Reject  bool
	Comment string
}

// Approve records a signed human review decision on a commit.
func (s *Service) Approve(ctx context.Context, repo *gitx.Repo, opts ApproveOptions) (*dsse.Envelope, string, error) {
	commit, err := repo.ResolveCommit(ctx, opts.Rev)
	if err != nil {
		return nil, "", err
	}
	decision := provenance.Approve
	if opts.Reject {
		decision = provenance.Reject
	}
	pred := &provenance.ApprovalPredicate{
		Commit:    commit,
		Approver:  provenance.Actor{Kind: provenance.ActorHuman, ID: s.Principal},
		Role:      opts.Role,
		Rules:     opts.Rules,
		Decision:  decision,
		Comment:   opts.Comment,
		CreatedAt: s.now(),
	}
	env, err := s.seal(commit, provenance.PredicateApproval, pred)
	if err != nil {
		return nil, "", err
	}
	if err := notes.Add(ctx, repo, commit, env); err != nil {
		return nil, "", err
	}
	s.log("approval.recorded", "human:"+s.Principal, commit, map[string]any{
		"decision": decision, "role": opts.Role, "rules": opts.Rules, "attestation": provenance.Digest(env),
	})
	return env, commit, nil
}

func (s *Service) seal(commit, predicateType string, pred any) (*dsse.Envelope, error) {
	st, err := provenance.NewStatement(commit, predicateType, pred)
	if err != nil {
		return nil, err
	}
	return st.Seal(s.Signer)
}

func (s *Service) log(typ, actor, subject string, data any) {
	if s.Ledger != nil {
		_, _ = s.Ledger.Append(typ, actor, subject, data)
	}
}
