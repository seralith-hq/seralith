package control

import (
	"context"
	"fmt"
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
// current policy and stores them. Unverifiable envelopes are recorded but flagged.
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
		if vr, err := env.Verify(trust, 1); err == nil {
			rec.Verified = true
			if tk, ok := trust.Lookup(vr.Accepted[0]); ok {
				rec.Signer = tk.Principal
			}
		}
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

// Stats summarizes a project's attested activity for dashboards and reports.
func (s *Store) Stats(ctx context.Context, projectID string) (*Stats, error) {
	all, err := s.Attestations(ctx, projectID, AttestationQuery{Limit: 1000})
	if err != nil {
		return nil, err
	}
	st := &Stats{ByTool: map[string]int{}, ByModel: map[string]int{}, ByStatus: map[string]int{}, AwaitingApproval: []string{}}
	approved := map[string]bool{}
	for _, a := range all {
		if a.PredicateType == provenance.PredicateApproval && a.Verified && a.Status == "approval" {
			approved[a.Commit] = true
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
		if len(a.PendingRules) > 0 && !approved[a.Commit] {
			st.AwaitingApproval = append(st.AwaitingApproval, a.Commit)
		}
	}
	if total := st.HumanLines + st.AgentLines; total > 0 {
		st.AgentShare = float64(st.AgentLines) / float64(total)
	}
	return st, nil
}
