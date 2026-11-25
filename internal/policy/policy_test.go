package policy

import (
	"strings"
	"testing"

	"github.com/seralith-hq/seralith/pkg/provenance"
)

var (
	agent = provenance.Actor{Kind: provenance.ActorAgent, ID: "codex", Tool: "codex"}
	human = provenance.Actor{Kind: provenance.ActorHuman, ID: "alice@example.com"}
)

func mustDefault(t *testing.T) *Policy {
	t.Helper()
	p, err := Parse([]byte(Default))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func effects(res *provenance.PolicyResult) map[string]provenance.PolicyEffect {
	out := map[string]provenance.PolicyEffect{}
	for _, d := range res.Decisions {
		out[d.Rule] = d.Effect
	}
	return out
}

func TestDefaultPolicyParses(t *testing.T) {
	p := mustDefault(t)
	if len(p.Rules) != 5 || len(p.Digest()) != 64 {
		t.Fatalf("rules=%d digest=%q", len(p.Rules), p.Digest())
	}
}

func TestAgentPipelineEditDenied(t *testing.T) {
	p := mustDefault(t)
	res := p.Evaluate([]Change{{Actor: agent, Path: ".github/workflows/ci.yml", LinesAdded: 3}})
	if effects(res)["agents-no-pipeline-edits"] != provenance.EffectDeny {
		t.Fatalf("decisions = %+v", res.Decisions)
	}
	if Strongest(res.Decisions) != provenance.EffectDeny {
		t.Fatal("strongest should be deny")
	}
}

func TestUnverifiedHumanLabels(t *testing.T) {
	p := mustDefault(t)
	res := p.Evaluate([]Change{{Actor: human, Path: ".github/workflows/ci.yml", LinesAdded: 3}})
	if len(res.Decisions) != 1 {
		t.Fatalf("decisions = %+v", res.Decisions)
	}
	if d := res.Decisions[0]; d.Rule != "agents-no-pipeline-edits" || d.Effect != provenance.EffectRequireApproval || d.Approvals != 1 {
		t.Fatalf("human pipeline edit should need one approval, got %+v", d)
	}
	res = p.Evaluate([]Change{{Actor: human, Path: "internal/crypto/aes.go", LinesAdded: 3}})
	if d := res.Decisions; len(d) != 1 || d[0].Effect != provenance.EffectRequireApproval || d[0].Roles[0] != "security" {
		t.Fatalf("human crypto edit should need security approval, got %+v", d)
	}
	res = p.Evaluate([]Change{{Actor: human, Path: "a.go", LinesAdded: 5000}})
	if len(res.Decisions) != 0 {
		t.Fatalf("rules without paths must not extend to humans: %+v", res.Decisions)
	}
	res = p.Evaluate([]Change{{Actor: agent, Path: ".github/workflows/ci.yml"}, {Actor: human, Path: ".github/workflows/release.yml"}})
	if d := res.Decisions; len(d) != 1 || d[0].Effect != provenance.EffectDeny || len(d[0].Paths) != 1 || d[0].Paths[0] != ".github/workflows/ci.yml" {
		t.Fatalf("agent edit should still be denied: %+v", d)
	}
}

func TestTrustedHumanLabels(t *testing.T) {
	p, err := Parse([]byte(strings.Replace(Default, "humanLabels: unverified", "humanLabels: trusted", 1)))
	if err != nil {
		t.Fatal(err)
	}
	res := p.Evaluate([]Change{{Actor: human, Path: ".github/workflows/ci.yml"}, {Actor: human, Path: "internal/crypto/aes.go"}})
	if len(res.Decisions) != 0 {
		t.Fatalf("trusted human edits triggered %+v", res.Decisions)
	}
}

func TestSecretsDeniedForEveryoneWithExclusions(t *testing.T) {
	p := mustDefault(t)
	res := p.Evaluate([]Change{{Actor: human, Path: "deploy/server.key"}, {Actor: human, Path: "app/.env.example"}})
	if len(res.Decisions) != 1 || res.Decisions[0].Paths[0] != "deploy/server.key" {
		t.Fatalf("decisions = %+v", res.Decisions)
	}
	res = p.Evaluate([]Change{{Actor: human, Path: ".env"}})
	if effects(res)["no-secret-material"] != provenance.EffectDeny {
		t.Fatalf("root .env not denied: %+v", res.Decisions)
	}
}

func TestSecurityCriticalRequiresApproval(t *testing.T) {
	p := mustDefault(t)
	res := p.Evaluate([]Change{{Actor: agent, Path: "internal/auth/token.go", LinesAdded: 10}})
	d := res.Decisions[0]
	if d.Effect != provenance.EffectRequireApproval || d.Approvals != 1 || d.Roles[0] != "security" {
		t.Fatalf("decision = %+v", d)
	}
}

func TestBlastRadiusThreshold(t *testing.T) {
	p := mustDefault(t)
	small := p.Evaluate([]Change{{Actor: agent, Path: "a.go", LinesAdded: 1000}, {Actor: agent, Path: "b.go", LinesAdded: 500}})
	if _, ok := effects(small)["agent-blast-radius"]; ok {
		t.Fatal("1500 lines should not exceed gt 1500")
	}
	big := p.Evaluate([]Change{{Actor: agent, Path: "a.go", LinesAdded: 1000}, {Actor: agent, Path: "b.go", LinesAdded: 501}})
	if effects(big)["agent-blast-radius"] != provenance.EffectRequireApproval {
		t.Fatal("1501 lines should exceed threshold")
	}
}

func TestToolAllowlist(t *testing.T) {
	p, err := Parse([]byte(`version: 1
rules:
  - id: approved-tools
    when:
      toolsNotIn: [codex, claude]
    effect: deny
`))
	if err != nil {
		t.Fatal(err)
	}
	rogue := provenance.Actor{Kind: provenance.ActorAgent, ID: "x", Tool: "unvetted-agent"}
	if res := p.Evaluate([]Change{{Actor: rogue, Path: "a"}}); len(res.Decisions) != 1 {
		t.Fatal("unapproved tool not denied")
	}
	if res := p.Evaluate([]Change{{Actor: agent, Path: "a"}, {Actor: human, Path: "b"}}); len(res.Decisions) != 0 {
		t.Fatalf("approved tool or human denied: %+v", res.Decisions)
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]string{
		"version":   "version: 2\nrules: []\n",
		"effect":    "version: 1\nrules:\n  - id: a\n    when: {}\n    effect: explode\n",
		"approvals": "version: 1\nrules:\n  - id: a\n    when: {}\n    effect: require-approval\n",
		"duplicate": "version: 1\nrules:\n  - id: a\n    when: {}\n    effect: warn\n  - id: a\n    when: {}\n    effect: warn\n",
		"id":        "version: 1\nrules:\n  - id: Bad ID\n    when: {}\n    effect: warn\n",
		"actor":     "version: 1\nrules:\n  - id: a\n    when: {actors: [robot]}\n    effect: warn\n",
		"glob":      "version: 1\nrules:\n  - id: a\n    when: {paths: ['[']}\n    effect: warn\n",
		"unknown":   "version: 1\nrules:\n  - id: a\n    when: {colour: red}\n    effect: warn\n",
		"labels":    "version: 1\nhumanLabels: maybe\nrules: []\n",
	}
	for name, doc := range cases {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestChangesFromSegmentsIncludesRenameSource(t *testing.T) {
	segs := []provenance.Segment{{Actor: agent, Files: []provenance.FileChange{{Path: "new.go", OldPath: ".github/workflows/x.yml", LinesAdded: 2}}}}
	changes := ChangesFromSegments(segs)
	if len(changes) != 2 {
		t.Fatalf("changes = %+v", changes)
	}
	res := mustDefault(t).Evaluate(changes)
	if !strings.Contains(strings.Join(res.Decisions[0].Paths, ","), ".github/workflows/x.yml") {
		t.Fatal("renaming a pipeline file away should still be caught")
	}
}
