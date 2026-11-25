package control

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/seralith-hq/seralith/internal/keys"
	"github.com/seralith-hq/seralith/internal/policy"
	"github.com/seralith-hq/seralith/pkg/dsse"
	"github.com/seralith-hq/seralith/pkg/provenance"
)

// TrustStore builds a verifiable trust store from the organization's key registry.
func (s *Store) TrustStore(ctx context.Context, orgID string) (*keys.TrustStore, error) {
	list, err := s.Keys(ctx, orgID)
	if err != nil {
		return nil, err
	}
	ts := keys.NewTrustStore()
	for _, k := range list {
		pub, err := keys.ParsePublicPEM([]byte(k.PublicKey))
		if err != nil {
			return nil, fmt.Errorf("control: key %s: %w", k.ID, err)
		}
		if _, err := ts.Add(pub, k.Principal, keys.PrincipalKind(k.Kind), k.Roles...); err != nil {
			return nil, err
		}
		if k.Revoked {
			ts.Revoke(k.ID)
		}
	}
	return ts, nil
}

type IngestResult struct {
	Digest   string `json:"digest"`
	Commit   string `json:"commit"`
	Stored   bool   `json:"stored"`
	Verified bool   `json:"verified"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

// Ingest verifies envelopes against the organization's registry and the project's
// current policy and stores them. An envelope counts as verified only under the
// identity rules the gate applies: a change must be signed by its committer, and an
// approval by its approver's human key holding the stated role. Other envelopes are
// recorded but flagged, and a later verified upload of the same statement replaces
// them.
func (s *Store) Ingest(ctx context.Context, project *Project, envs []*dsse.Envelope) ([]IngestResult, error) {
	trust, err := s.TrustStore(ctx, project.OrgID)
	if err != nil {
		return nil, err
	}
	pol := &policy.Policy{Version: 1}
	if pv, err := s.LatestPolicy(ctx, project.ID); err == nil {
		if parsed, perr := policy.Parse([]byte(pv.Document)); perr == nil {
			pol = parsed
		}
	}
	out := make([]IngestResult, 0, len(envs))
	for _, env := range envs {
		res := IngestResult{Digest: provenance.Digest(env)}
		st, err := provenance.Open(env)
		if err != nil {
			res.Error = err.Error()
			out = append(out, res)
			continue
		}
		res.Commit = st.Commit()
		rec := AttestationRecord{Digest: res.Digest, Commit: res.Commit, PredicateType: st.PredicateType}
		raw, _ := env.Marshal()
		rec.Envelope = string(raw)
		rec.Signer, rec.Verified = signedBy(st, env, trust)
		switch st.PredicateType {
		case provenance.PredicateChange:
			summarizeChange(st, pol, &rec)
		case provenance.PredicateApproval:
			rec.Status = "approval"
			if ap, err := st.Approval(); err == nil && ap.Decision == provenance.Reject {
				rec.Status = "rejection"
			}
		default:
			rec.Status = "unknown"
		}
		if !rec.Verified {
			rec.Status = "untrusted"
		}
		stored, err := s.PutAttestation(ctx, project.ID, rec)
		if err != nil {
			return nil, err
		}
		res.Stored, res.Verified, res.Status = stored, rec.Verified, rec.Status
		out = append(out, res)
	}
	return out, nil
}

// signedBy returns the principal an envelope is verified for: the committer of a
// change or the approver of an approval, provided a trusted key of theirs signed it.
func signedBy(st *provenance.Statement, env *dsse.Envelope, trust *keys.TrustStore) (string, bool) {
	vr, err := env.Verify(trust, 1)
	if err != nil {
		return "", false
	}
	accepts := func(want string, ok func(*keys.TrustedKey) bool) bool {
		for _, id := range vr.Accepted {
			if tk, found := trust.Lookup(id); found && tk.Principal == want && ok(tk) {
				return true
			}
		}
		return false
	}
	switch st.PredicateType {
	case provenance.PredicateChange:
		pred, err := st.Change()
		if err != nil || pred.Commit != st.Commit() {
			return "", false
		}
		return pred.Committer.ID, accepts(pred.Committer.ID, func(*keys.TrustedKey) bool { return true })
	case provenance.PredicateApproval:
		pred, err := st.Approval()
		if err != nil || pred.Commit != st.Commit() {
			return "", false
		}
		return pred.Approver.ID, accepts(pred.Approver.ID, func(tk *keys.TrustedKey) bool {
			return tk.Kind == keys.KindHuman && (pred.Role == "" || tk.HasRole(pred.Role))
		})
	default:
		return "", false
	}
}

func summarizeChange(st *provenance.Statement, pol *policy.Policy, rec *AttestationRecord) {
	pred, err := st.Change()
	if err != nil {
		rec.Status = "invalid"
		return
	}
	rec.HumanLines = pred.Summary.LinesByKind[provenance.ActorHuman]
	rec.AgentLines = pred.Summary.LinesByKind[provenance.ActorAgent]
	tools, models := map[string]bool{}, map[string]bool{}
	for _, seg := range pred.Segments {
		if seg.Actor.Kind == provenance.ActorAgent {
			if seg.Actor.Tool != "" {
				tools[seg.Actor.Tool] = true
			}
			if seg.Actor.Model != "" {
				models[seg.Actor.Model] = true
			}
		}
	}
	rec.Tools, rec.Models = sortedKeys(tools), sortedKeys(models)
	res := pol.Evaluate(policy.ChangesFromSegments(pred.Segments))
	for _, d := range res.Decisions {
		if d.Effect == provenance.EffectRequireApproval {
			rec.PendingRules = append(rec.PendingRules, d.Rule)
		}
	}
	rec.Status = string(policy.Strongest(res.Decisions))
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type Stats struct {
	Commits          int            `json:"commits"`
	HumanLines       int            `json:"humanLines"`
	AgentLines       int            `json:"agentLines"`
	AgentShare       float64        `json:"agentShare"`
	ByTool           map[string]int `json:"byTool"`
	ByModel          map[string]int `json:"byModel"`
	ByStatus         map[string]int `json:"byStatus"`
	Untrusted        int            `json:"untrusted"`
	AwaitingApproval []string       `json:"awaitingApproval"`
}

// Stats summarizes a project's attested activity for dashboards and reports. A
// commit stops awaiting approval only when verified approvals satisfy each pending
// rule of the project's current policy as the gate would count them.
func (s *Store) Stats(ctx context.Context, projectID string) (*Stats, error) {
	all, err := s.Attestations(ctx, projectID, AttestationQuery{Limit: 1000, WithEnvelope: true})
	if err != nil {
		return nil, err
	}
	pol := &policy.Policy{Version: 1}
	if pv, err := s.LatestPolicy(ctx, projectID); err == nil {
		if parsed, perr := policy.Parse([]byte(pv.Document)); perr == nil {
			pol = parsed
		}
	}
	st := &Stats{ByTool: map[string]int{}, ByModel: map[string]int{}, ByStatus: map[string]int{}, AwaitingApproval: []string{}}
	approvals := map[string][]*provenance.ApprovalPredicate{}
	for _, a := range all {
		if a.PredicateType != provenance.PredicateApproval || !a.Verified {
			continue
		}
		if ap := approvalOf(a); ap != nil {
			approvals[a.Commit] = append(approvals[a.Commit], ap)
		}
	}
	for _, a := range all {
		if a.PredicateType != provenance.PredicateChange {
			continue
		}
		st.Commits++
		st.ByStatus[a.Status]++
		if !a.Verified {
			st.Untrusted++
			continue
		}
		st.HumanLines += a.HumanLines
		st.AgentLines += a.AgentLines
		for _, t := range a.Tools {
			st.ByTool[t]++
		}
		for _, m := range a.Models {
			st.ByModel[m]++
		}
		if !rulesSatisfied(pol, a.PendingRules, a.Signer, approvals[a.Commit]) {
			st.AwaitingApproval = append(st.AwaitingApproval, a.Commit)
		}
	}
	if total := st.HumanLines + st.AgentLines; total > 0 {
		st.AgentShare = float64(st.AgentLines) / float64(total)
	}
	return st, nil
}

func approvalOf(a AttestationRecord) *provenance.ApprovalPredicate {
	env, err := dsse.Parse([]byte(a.Envelope))
	if err != nil {
		return nil
	}
	st, err := provenance.Open(env)
	if err != nil {
		return nil
	}
	ap, err := st.Approval()
	if err != nil {
		return nil
	}
	return ap
}

// rulesSatisfied reports whether approvals meet every pending rule, counting
// distinct approvers other than the committer, with a matching role and rule list.
// A rule no longer in the policy needs one independent approval.
func rulesSatisfied(pol *policy.Policy, pending []string, committer string, approvals []*provenance.ApprovalPredicate) bool {
	for _, id := range pending {
		need, roles, allowSelf := 1, []string(nil), false
		if r, ok := pol.Rule(id); ok {
			need, roles, allowSelf = r.Approvals, r.Roles, r.AllowSelf
		}
		approvers := map[string]bool{}
		for _, ap := range approvals {
			switch {
			case len(ap.Rules) > 0 && !slices.Contains(ap.Rules, id):
			case len(roles) > 0 && !slices.Contains(roles, ap.Role):
			case ap.Approver.ID == committer && !allowSelf:
			case ap.Decision == provenance.Reject:
				return false
			default:
				approvers[ap.Approver.ID] = true
			}
		}
		if len(approvers) < need {
			return false
		}
	}
	return true
}
