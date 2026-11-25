package agent

import (
	"context"
	"testing"

	"github.com/seralith-hq/seralith/internal/project"
	"github.com/seralith-hq/seralith/pkg/provenance"
)

func TestDeclaredModel(t *testing.T) {
	r := NewRegistry(nil)
	codex, _ := r.Get("codex")
	cases := []struct {
		args []string
		env  []string
		want string
	}{
		{[]string{"codex", "-m", "gpt-x"}, nil, "gpt-x"},
		{[]string{"codex", "--model=local-7b"}, nil, "local-7b"},
		{[]string{"codex"}, nil, ""},
	}
	for _, c := range cases {
		if got := codex.DeclaredModel(c.args, c.env); got != c.want {
			t.Errorf("DeclaredModel(%v) = %q, want %q", c.args, got, c.want)
		}
	}
	claude, _ := r.Get("claude")
	if got := claude.DeclaredModel(nil, []string{"ANTHROPIC_MODEL=a", "ANTHROPIC_MODEL=b"}); got != "b" {
		t.Errorf("env model = %q, want last value", got)
	}
}

func TestRegistryCustomAndGuess(t *testing.T) {
	r := NewRegistry([]project.AgentProfile{
		{ID: "inhouse", Command: []string{"/opt/tools/inhouse-agent"}, ModelFlags: []string{"--llm"}},
		{ID: "pairing", Kind: "human", Command: []string{"tmate"}},
	})
	p, ok := r.Guess([]string{"/usr/local/bin/inhouse-agent", "--llm", "mistral"})
	if !ok || p.ID != "inhouse" || !p.Custom || p.Kind != provenance.ActorAgent {
		t.Fatalf("guess = %+v ok=%v", p, ok)
	}
	if p.DeclaredModel([]string{"x", "--llm", "mistral"}, nil) != "mistral" {
		t.Fatal("custom model flag not honored")
	}
	if h, _ := r.Get("pairing"); h.Kind != provenance.ActorHuman {
		t.Fatal("custom human profile has wrong kind")
	}
	if s, ok := r.Guess([]string{"/bin/zsh"}); !ok || s.ID != "shell" {
		t.Fatalf("shell guess = %+v", s)
	}
	if _, ok := r.Guess([]string{"make"}); ok {
		t.Fatal("unknown command matched a profile")
	}
	if len(r.Detect()) != len(r.All()) {
		t.Fatal("detect should report every profile")
	}
}

func TestActor(t *testing.T) {
	r := NewRegistry(nil)
	shell, _ := r.Get("shell")
	a := shell.Actor(context.Background(), "dev@example.com", nil, nil)
	if a.Kind != provenance.ActorHuman || a.ID != "dev@example.com" {
		t.Fatalf("human actor = %+v", a)
	}
	aider, _ := r.Get("aider")
	b := aider.Actor(context.Background(), "dev@example.com", []string{"aider", "--model", "m"}, nil)
	if b.Kind != provenance.ActorAgent || b.Operator != "dev@example.com" || b.Model != "m" || b.ID != "aider" {
		t.Fatalf("agent actor = %+v", b)
	}
}
