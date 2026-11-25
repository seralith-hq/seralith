// Package policy evaluates declarative rules over attributed changes.
package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/seralith-hq/seralith/pkg/provenance"
	"gopkg.in/yaml.v3"
)

var ruleID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type Threshold struct {
	GreaterThan *int `yaml:"gt,omitempty" json:"gt,omitempty"`
}

func (t *Threshold) exceeded(v int) bool {
	return t == nil || t.GreaterThan == nil || v > *t.GreaterThan
}

// Match selects the changes a rule applies to. Every populated field must match.
type Match struct {
	Actors     []provenance.ActorKind `yaml:"actors,omitempty" json:"actors,omitempty"`
	Tools      []string               `yaml:"tools,omitempty" json:"tools,omitempty"`
	ToolsNotIn []string               `yaml:"toolsNotIn,omitempty" json:"toolsNotIn,omitempty"`
	Models     []string               `yaml:"models,omitempty" json:"models,omitempty"`
	Paths      []string               `yaml:"paths,omitempty" json:"paths,omitempty"`
	Exclude    []string               `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	Files      *Threshold             `yaml:"files,omitempty" json:"files,omitempty"`
	LinesAdded *Threshold             `yaml:"linesAdded,omitempty" json:"linesAdded,omitempty"`
}

type Rule struct {
	ID          string                  `yaml:"id" json:"id"`
	Description string                  `yaml:"description,omitempty" json:"description,omitempty"`
	When        Match                   `yaml:"when" json:"when"`
	Effect      provenance.PolicyEffect `yaml:"effect" json:"effect"`
	Message     string                  `yaml:"message,omitempty" json:"message,omitempty"`
	Approvals   int                     `yaml:"approvals,omitempty" json:"approvals,omitempty"`
	Roles       []string                `yaml:"roles,omitempty" json:"roles,omitempty"`
	AllowSelf   bool                    `yaml:"allowSelfApproval,omitempty" json:"allowSelfApproval,omitempty"`
}

// Human label modes. The actor kind of a segment is recorded by the committer's own
// Seralith, so a "human" label is a claim by the party it benefits.
const (
	// HumanLabelsUnverified, the default, applies agent rules that protect paths to
	// human-labeled changes too, with deny relaxed to one independent approval.
	HumanLabelsUnverified = "unverified"
	// HumanLabelsTrusted takes human labels at face value.
	HumanLabelsTrusted = "trusted"
)

type Policy struct {
	Version     int    `yaml:"version" json:"version"`
	HumanLabels string `yaml:"humanLabels,omitempty" json:"humanLabels,omitempty"`
	Rules       []Rule `yaml:"rules" json:"rules"`

	digest string
}

// Default is the policy written by `seralith init`. It encodes the controls most
// regulated teams start from.
const Default = `version: 1
humanLabels: unverified
rules:
  - id: agents-no-pipeline-edits
    description: Coding agents may not modify CI/CD pipeline definitions.
    when:
      actors: [agent]
      paths: [".github/workflows/**", ".gitlab-ci.yml", "Jenkinsfile", ".circleci/**"]
    effect: deny
    message: Pipeline changes must be authored by a human.

  - id: no-secret-material
    description: Private keys and environment files never enter the repository.
    when:
      paths: ["**/*.pem", "**/*.key", "**/.env", "**/.env.*", "**/id_rsa", "**/id_ed25519"]
      exclude: ["**/.env.example", "**/testdata/**"]
    effect: deny
    message: Secret material detected in the change set.

  - id: security-critical-review
    description: Agent changes to security-critical code need independent human approval.
    when:
      actors: [agent]
      paths: ["**/auth/**", "**/crypto/**", "**/security/**", "**/*.tf", "**/Dockerfile"]
    effect: require-approval
    approvals: 1
    roles: [security]

  - id: agent-blast-radius
    description: Large agent change sets need a second pair of eyes.
    when:
      actors: [agent]
      linesAdded: { gt: 1500 }
    effect: require-approval
    approvals: 1

  - id: dependency-manifests
    description: Flag agent edits to dependency manifests for review.
    when:
      actors: [agent]
      paths: ["**/go.mod", "**/package.json", "**/requirements*.txt", "**/Cargo.toml", "**/pom.xml"]
    effect: warn
    message: Agent modified a dependency manifest; review new dependencies.
`

// Parse decodes and validates a policy document.
func Parse(data []byte) (*Policy, error) {
	var p Policy
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("policy: parse: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	p.digest = hex.EncodeToString(sum[:])
	return &p, nil
}

// Load reads a policy file, falling back to an empty policy when it does not exist.
func Load(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Policy{Version: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func (p *Policy) Digest() string { return p.digest }

func (p *Policy) Validate() error {
	if p.Version != 1 {
		return fmt.Errorf("policy: unsupported version %d", p.Version)
	}
	switch p.HumanLabels {
	case "", HumanLabelsUnverified, HumanLabelsTrusted:
	default:
		return fmt.Errorf("policy: humanLabels must be %q or %q", HumanLabelsUnverified, HumanLabelsTrusted)
	}
	seen := map[string]bool{}
	var errs []error
	for i, r := range p.Rules {
		where := fmt.Sprintf("rule %d (%s)", i, r.ID)
		if !ruleID.MatchString(r.ID) {
			errs = append(errs, fmt.Errorf("%s: id must match %s", where, ruleID))
		}
		if seen[r.ID] {
			errs = append(errs, fmt.Errorf("%s: duplicate id", where))
		}
		seen[r.ID] = true
		switch r.Effect {
		case provenance.EffectAllow, provenance.EffectWarn, provenance.EffectDeny:
		case provenance.EffectRequireApproval:
			if r.Approvals < 1 {
				errs = append(errs, fmt.Errorf("%s: require-approval needs approvals >= 1", where))
			}
		default:
			errs = append(errs, fmt.Errorf("%s: unknown effect %q", where, r.Effect))
		}
		for _, a := range r.When.Actors {
			if a != provenance.ActorHuman && a != provenance.ActorAgent {
				errs = append(errs, fmt.Errorf("%s: unknown actor kind %q", where, a))
			}
		}
		for _, g := range append(slices.Clone(r.When.Paths), r.When.Exclude...) {
			if !doublestar.ValidatePattern(g) {
				errs = append(errs, fmt.Errorf("%s: invalid glob %q", where, g))
			}
		}
	}
	return errors.Join(errs...)
}

// Change is one actor's modification to one file, the unit policy reasons about.
type Change struct {
	Actor      provenance.Actor
	Path       string
	LinesAdded int
}

// ChangesFromSegments flattens segments into policy changes.
func ChangesFromSegments(segments []provenance.Segment) []Change {
	var out []Change
	for _, s := range segments {
		for _, f := range s.Files {
			out = append(out, Change{Actor: s.Actor, Path: f.Path, LinesAdded: f.LinesAdded})
			if f.OldPath != "" {
				out = append(out, Change{Actor: s.Actor, Path: f.OldPath})
			}
		}
	}
	return out
}

func (m *Match) matchActor(a provenance.Actor) bool {
	if len(m.Actors) > 0 && !slices.Contains(m.Actors, a.Kind) {
		return false
	}
	if len(m.Tools) > 0 && !slices.Contains(m.Tools, a.Tool) {
		return false
	}
	if len(m.ToolsNotIn) > 0 && (a.Kind != provenance.ActorAgent || slices.Contains(m.ToolsNotIn, a.Tool)) {
		return false
	}
	if len(m.Models) > 0 && !slices.Contains(m.Models, a.Model) {
		return false
	}
	return true
}

func (m *Match) matchPath(path string) bool {
	for _, g := range m.Exclude {
		if ok, _ := doublestar.Match(g, path); ok {
			return false
		}
	}
	if len(m.Paths) == 0 {
		return true
	}
	for _, g := range m.Paths {
		if ok, _ := doublestar.Match(g, path); ok {
			return true
		}
	}
	return false
}

// unverifiedMessage explains a deny rule relaxed for a human-labeled change.
const unverifiedMessage = "Human-labeled change to a path this rule protects from agents; human labels are not verified, so it needs an independent approval."

// Evaluate applies every rule to changes and returns the triggered decisions.
func (p *Policy) Evaluate(changes []Change) *provenance.PolicyResult {
	res := &provenance.PolicyResult{Digest: provenance.DigestSet{"sha256": p.digest}, Decisions: []provenance.Decision{}}
	for _, r := range p.Rules {
		direct, extended := map[string]bool{}, map[string]bool{}
		lines := 0
		for _, c := range changes {
			if !r.When.matchPath(c.Path) {
				continue
			}
			switch {
			case r.When.matchActor(c.Actor):
				direct[c.Path] = true
			case p.extendsToHuman(&r, c.Actor):
				extended[c.Path] = true
			default:
				continue
			}
			lines += c.LinesAdded
		}
		for path := range direct {
			delete(extended, path)
		}
		matched := len(direct) + len(extended)
		if matched == 0 || !r.When.Files.exceeded(matched) || !r.When.LinesAdded.exceeded(lines) {
			continue
		}
		d := provenance.Decision{Rule: r.ID, Effect: r.Effect, Message: r.Message, Approvals: r.Approvals, Roles: r.Roles}
		if d.Message == "" {
			d.Message = r.Description
		}
		switch {
		case r.Effect == provenance.EffectDeny && len(direct) > 0:
			d.Paths = sorted(direct)
		case r.Effect == provenance.EffectDeny:
			d.Effect, d.Approvals, d.Message, d.Paths = provenance.EffectRequireApproval, 1, unverifiedMessage, sorted(extended)
		default:
			maps.Copy(direct, extended)
			d.Paths = sorted(direct)
		}
		res.Decisions = append(res.Decisions, d)
	}
	return res
}

// extendsToHuman reports whether a rule that targets agents on specific paths also
// applies to a human-labeled change because human labels are not trusted.
func (p *Policy) extendsToHuman(r *Rule, a provenance.Actor) bool {
	if p.HumanLabels == HumanLabelsTrusted || a.Kind != provenance.ActorHuman || len(r.When.Paths) == 0 {
		return false
	}
	m := r.When
	return slices.Contains(m.Actors, provenance.ActorAgent) ||
		(len(m.Actors) == 0 && (len(m.Tools) > 0 || len(m.ToolsNotIn) > 0 || len(m.Models) > 0))
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Rule returns the rule with id.
func (p *Policy) Rule(id string) (*Rule, bool) {
	for i := range p.Rules {
		if p.Rules[i].ID == id {
			return &p.Rules[i], true
		}
	}
	return nil, false
}

var severity = map[provenance.PolicyEffect]int{
	provenance.EffectAllow:           0,
	provenance.EffectWarn:            1,
	provenance.EffectRequireApproval: 2,
	provenance.EffectDeny:            3,
}

// Strongest returns the most restrictive effect among decisions.
func Strongest(decisions []provenance.Decision) provenance.PolicyEffect {
	out := provenance.EffectAllow
	for _, d := range decisions {
		if severity[d.Effect] > severity[out] {
			out = d.Effect
		}
	}
	return out
}
