package control_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/seralith-hq/seralith/ee/control"
	"github.com/seralith-hq/seralith/internal/keys"
	"github.com/seralith-hq/seralith/internal/policy"
	"github.com/seralith-hq/seralith/pkg/dsse"
	"github.com/seralith-hq/seralith/pkg/provenance"
)

type ingestFixture struct {
	a              *api
	owner, ci      string
	dev, other, ma *keys.PrivateKey
	commit         string
	legit          *dsse.Envelope
}

func newIngestFixture(t *testing.T) *ingestFixture {
	t.Helper()
	a, _ := newAPI(t)
	f := &ingestFixture{a: a, owner: a.bootstrapOrg(), commit: strings.Repeat("a", 40)}
	a.call("POST", "/v1/projects", f.owner, map[string]string{"name": "radar-fw"}, nil)
	a.call("PUT", "/v1/projects/radar-fw/policy", f.owner, map[string]string{"document": policy.Default}, nil)
	f.dev, _ = keys.Generate()
	f.other, _ = keys.Generate()
	f.ma, _ = keys.Generate()
	for _, k := range []struct {
		key             *keys.PrivateKey
		principal, kind string
		roles           []string
	}{
		{f.dev, "dev@acme.dev", "human", []string{"security"}},
		{f.other, "mallory@acme.dev", "human", []string{"security"}},
		{f.ma, "ci-bot", "machine", nil},
	} {
		pemText, _ := k.key.Public().PEM()
		if code := a.call("POST", "/v1/keys", f.owner, map[string]any{"principal": k.principal, "kind": k.kind, "roles": k.roles, "publicKey": pemText}, nil); code != http.StatusCreated {
			t.Fatalf("add key: %d", code)
		}
	}
	var m struct {
		Token string `json:"token"`
	}
	a.call("POST", "/v1/members", f.owner, map[string]string{"email": "ci@acme.dev", "role": "ci"}, &m)
	f.ci = m.Token

	pred := provenance.ChangePredicate{
		Commit:    f.commit,
		Committer: provenance.Actor{Kind: provenance.ActorHuman, ID: "dev@acme.dev"},
		Segments: []provenance.Segment{{
			ID: "s1", Kind: provenance.SegmentSession,
			Actor: provenance.Actor{Kind: provenance.ActorAgent, ID: "codex", Tool: "codex"},
			Files: []provenance.FileChange{{Path: "internal/crypto/aes.go", Status: provenance.FileAdded, LinesAdded: 30}},
		}},
		Summary: provenance.Summary{LinesByKind: map[provenance.ActorKind]int{provenance.ActorAgent: 30}},
	}
	st, _ := provenance.NewStatement(f.commit, provenance.PredicateChange, pred)
	f.legit, _ = st.Seal(f.dev)
	return f
}

func (f *ingestFixture) upload(t *testing.T, token string, envs ...*dsse.Envelope) {
	t.Helper()
	if code := f.a.call("POST", "/v1/projects/radar-fw/attestations", token, map[string]any{"envelopes": envs}, nil); code != http.StatusOK {
		t.Fatalf("ingest: %d", code)
	}
}

func (f *ingestFixture) records(t *testing.T) []control.AttestationRecord {
	t.Helper()
	var list []control.AttestationRecord
	f.a.call("GET", "/v1/projects/radar-fw/attestations?commit="+f.commit+"&envelope=true", f.owner, nil, &list)
	return list
}

func (f *ingestFixture) awaiting(t *testing.T) []string {
	t.Helper()
	var stats control.Stats
	f.a.call("GET", "/v1/projects/radar-fw/stats", f.owner, nil, &stats)
	return stats.AwaitingApproval
}

func (f *ingestFixture) approval(approver string, signer *keys.PrivateKey) *dsse.Envelope {
	st, _ := provenance.NewStatement(f.commit, provenance.PredicateApproval, provenance.ApprovalPredicate{
		Commit: f.commit, Approver: provenance.Actor{Kind: provenance.ActorHuman, ID: approver}, Role: "security", Decision: provenance.Approve,
	})
	env, _ := st.Seal(signer)
	return env
}

func TestIngestFrontRunCannotHideGenuineEnvelope(t *testing.T) {
	for name, forged := range map[string]func(f *ingestFixture) *dsse.Envelope{
		"re-signed by another member": func(f *ingestFixture) *dsse.Envelope {
			env := &dsse.Envelope{PayloadType: f.legit.PayloadType, Payload: f.legit.Payload}
			_ = env.AddSignature(f.other)
			return env
		},
		"signatures stripped": func(f *ingestFixture) *dsse.Envelope {
			return &dsse.Envelope{PayloadType: f.legit.PayloadType, Payload: f.legit.Payload}
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newIngestFixture(t)
			f.upload(t, f.ci, forged(f))
			if list := f.records(t); len(list) != 1 || list[0].Verified {
				t.Fatalf("forged upload verified: %+v", list)
			}
			f.upload(t, f.ci, f.legit)
			list := f.records(t)
			if len(list) != 1 || !list[0].Verified || list[0].Signer != "dev@acme.dev" || !strings.Contains(list[0].Envelope, f.dev.KeyID()) {
				t.Fatalf("genuine envelope not stored: %+v", list)
			}
		})
	}
}

func TestIngestCountsOnlyIndependentHumanApprovals(t *testing.T) {
	f := newIngestFixture(t)
	f.upload(t, f.ci, f.legit)
	if got := f.awaiting(t); len(got) != 1 {
		t.Fatalf("awaiting = %v", got)
	}
	f.upload(t, f.ci, f.approval("dev@acme.dev", f.ma), f.approval("dev@acme.dev", f.dev), f.approval("mallory@acme.dev", f.dev))
	if got := f.awaiting(t); len(got) != 1 {
		t.Fatalf("machine-key, self, or mismatched approval cleared the commit: %v", got)
	}
	f.upload(t, f.ci, f.approval("mallory@acme.dev", f.other))
	if got := f.awaiting(t); len(got) != 0 {
		t.Fatalf("independent approval not counted: %v", got)
	}
}
