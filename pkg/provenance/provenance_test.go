package provenance_test

import (
	"errors"
	"testing"
	"time"

	"github.com/seralith-hq/seralith/internal/keys"
	"github.com/seralith-hq/seralith/pkg/provenance"
)

func TestSealOpenChange(t *testing.T) {
	k, _ := keys.Generate()
	pred := provenance.ChangePredicate{
		Commit:    "abc123",
		Tree:      "def456",
		Committer: provenance.Actor{Kind: provenance.ActorHuman, ID: "alice@example.com"},
		Summary:   provenance.Summary{FilesChanged: 1, LinesByKind: map[provenance.ActorKind]int{provenance.ActorAgent: 10}},
		Builder:   provenance.Builder{ID: provenance.BuilderID, Version: "test"},
		CreatedAt: time.Unix(0, 0).UTC(),
	}
	st, err := provenance.NewStatement("abc123", provenance.PredicateChange, pred)
	if err != nil {
		t.Fatal(err)
	}
	env, err := st.Seal(k)
	if err != nil {
		t.Fatal(err)
	}
	ts := keys.NewTrustStore()
	if _, err := ts.Add(k.Public(), "alice@example.com", keys.KindHuman); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Verify(ts, 1); err != nil {
		t.Fatal(err)
	}
	opened, err := provenance.Open(env)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Commit() != "abc123" {
		t.Fatalf("commit = %q", opened.Commit())
	}
	got, err := opened.Change()
	if err != nil {
		t.Fatal(err)
	}
	if got.Summary.LinesByKind[provenance.ActorAgent] != 10 {
		t.Fatalf("summary = %+v", got.Summary)
	}
	if _, err := opened.Approval(); !errors.Is(err, provenance.ErrWrongPredicate) {
		t.Fatalf("Approval() err = %v", err)
	}
	if d := provenance.Digest(env); len(d) != 64 {
		t.Fatalf("digest = %q", d)
	}
}

func TestActorString(t *testing.T) {
	a := provenance.Actor{Kind: provenance.ActorAgent, ID: "codex", Tool: "codex"}
	if a.String() != "agent:codex(codex)" {
		t.Fatal(a.String())
	}
	h := provenance.Actor{Kind: provenance.ActorHuman, ID: "bob"}
	if h.String() != "human:bob" {
		t.Fatal(h.String())
	}
}
