// Package agent describes the terminal tools Sigil can launch and attribute work to.
// Sigil only starts these programs; it never proxies their network traffic or credentials.
package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ryan-tobin/new-startup/internal/project"
	"github.com/ryan-tobin/new-startup/pkg/provenance"
)

type Profile struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Kind       provenance.ActorKind `json:"kind"`
	Command    []string             `json:"command"`
	ModelFlags []string             `json:"modelFlags,omitempty"`
	ModelEnv   []string             `json:"modelEnv,omitempty"`
	Custom     bool                 `json:"custom,omitempty"`
}

// Builtins are the profiles Sigil recognizes without configuration.
var Builtins = []Profile{
	{ID: "shell", Name: "Shell", Kind: provenance.ActorHuman, Command: []string{defaultShell()}},
	{ID: "claude", Name: "Claude Code", Kind: provenance.ActorAgent, Command: []string{"claude"}, ModelFlags: []string{"--model"}, ModelEnv: []string{"ANTHROPIC_MODEL"}},
	{ID: "codex", Name: "Codex CLI", Kind: provenance.ActorAgent, Command: []string{"codex"}, ModelFlags: []string{"-m", "--model"}},
	{ID: "gemini", Name: "Gemini CLI", Kind: provenance.ActorAgent, Command: []string{"gemini"}, ModelFlags: []string{"-m", "--model"}, ModelEnv: []string{"GEMINI_MODEL"}},
	{ID: "aider", Name: "Aider", Kind: provenance.ActorAgent, Command: []string{"aider"}, ModelFlags: []string{"--model"}, ModelEnv: []string{"AIDER_MODEL"}},
	{ID: "opencode", Name: "OpenCode", Kind: provenance.ActorAgent, Command: []string{"opencode"}, ModelFlags: []string{"-m", "--model"}},
	{ID: "cursor-agent", Name: "Cursor Agent", Kind: provenance.ActorAgent, Command: []string{"cursor-agent"}, ModelFlags: []string{"-m", "--model"}},
	{ID: "copilot", Name: "GitHub Copilot CLI", Kind: provenance.ActorAgent, Command: []string{"copilot"}, ModelFlags: []string{"--model"}},
	{ID: "goose", Name: "Goose", Kind: provenance.ActorAgent, Command: []string{"goose"}, ModelEnv: []string{"GOOSE_MODEL"}},
	{ID: "amp", Name: "Amp", Kind: provenance.ActorAgent, Command: []string{"amp"}},
	{ID: "qwen", Name: "Qwen Code", Kind: provenance.ActorAgent, Command: []string{"qwen"}, ModelFlags: []string{"-m", "--model"}},
	{ID: "ollama", Name: "Ollama (local)", Kind: provenance.ActorAgent, Command: []string{"ollama", "run"}},
}

func defaultShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

// Registry merges built-in and project-defined profiles.
type Registry struct {
	profiles map[string]Profile
}

func NewRegistry(custom []project.AgentProfile) *Registry {
	r := &Registry{profiles: map[string]Profile{}}
	for _, p := range Builtins {
		r.profiles[p.ID] = p
	}
	for _, c := range custom {
		kind := provenance.ActorAgent
		if c.Kind == string(provenance.ActorHuman) {
			kind = provenance.ActorHuman
		}
		name := c.Name
		if name == "" {
			name = c.ID
		}
		r.profiles[c.ID] = Profile{ID: c.ID, Name: name, Kind: kind, Command: c.Command, ModelFlags: c.ModelFlags, ModelEnv: c.ModelEnv, Custom: true}
	}
	return r
}

func (r *Registry) Get(id string) (Profile, bool) {
	p, ok := r.profiles[id]
	return p, ok
}

func (r *Registry) All() []Profile {
	out := make([]Profile, 0, len(r.profiles))
	for _, p := range r.profiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Guess finds the profile whose executable matches command[0].
func (r *Registry) Guess(command []string) (Profile, bool) {
	if len(command) == 0 {
		return Profile{}, false
	}
	exe := filepath.Base(command[0])
	for _, p := range r.All() {
		if p.ID == "shell" {
			continue
		}
		if len(p.Command) > 0 && filepath.Base(p.Command[0]) == exe {
			return p, true
		}
	}
	if slices.Contains([]string{"bash", "zsh", "fish", "sh", "nu", "pwsh"}, exe) {
		return r.profiles["shell"], true
	}
	return Profile{}, false
}

// DeclaredModel extracts the model a profile was asked to use from its arguments or
// environment. It is a declaration for the record, not a verified fact.
func (p Profile) DeclaredModel(args, env []string) string {
	for i, a := range args {
		for _, f := range p.ModelFlags {
			if a == f && i+1 < len(args) {
				return args[i+1]
			}
			if strings.HasPrefix(a, f+"=") {
				return strings.TrimPrefix(a, f+"=")
			}
		}
	}
	for _, name := range p.ModelEnv {
		for _, kv := range slices.Backward(env) {
			if v, ok := strings.CutPrefix(kv, name+"="); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

var versionPattern = regexp.MustCompile(`v?\d+\.\d+(\.\d+)?([-+.][0-9A-Za-z.-]+)?`)

// ToolVersion asks the executable for its version, returning "" when unavailable.
func (p Profile) ToolVersion(ctx context.Context) string {
	if p.Kind == provenance.ActorHuman || len(p.Command) == 0 {
		return ""
	}
	path, err := exec.LookPath(p.Command[0])
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return ""
	}
	return versionPattern.FindString(string(out))
}

type Detection struct {
	Profile   Profile `json:"profile"`
	Installed bool    `json:"installed"`
	Path      string  `json:"path,omitempty"`
}

// Detect reports which profiles have their executable on PATH.
func (r *Registry) Detect() []Detection {
	all := r.All()
	out := make([]Detection, 0, len(all))
	for _, p := range all {
		d := Detection{Profile: p}
		if len(p.Command) > 0 {
			if path, err := exec.LookPath(p.Command[0]); err == nil {
				d.Installed, d.Path = true, path
			}
		}
		out = append(out, d)
	}
	return out
}

// Actor builds the attributed actor for running profile on behalf of operator.
func (p Profile) Actor(ctx context.Context, operator string, args, env []string) provenance.Actor {
	host, _ := os.Hostname()
	if p.Kind == provenance.ActorHuman {
		return provenance.Actor{Kind: provenance.ActorHuman, ID: operator, Tool: p.ID, Host: host}
	}
	return provenance.Actor{
		Kind:        provenance.ActorAgent,
		ID:          p.ID,
		Tool:        p.ID,
		ToolVersion: p.ToolVersion(ctx),
		Model:       p.DeclaredModel(args, env),
		Operator:    operator,
		Host:        host,
	}
}
