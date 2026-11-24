// Package provenance defines the in-toto statements and predicates Sigil produces.
package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ryan-tobin/new-startup/pkg/dsse"
)

const (
	StatementType     = "https://in-toto.io/Statement/v1"
	PredicateChange   = "https://sigil.systems/attestation/change/v1"
	PredicateApproval = "https://sigil.systems/attestation/approval/v1"
	BuilderID         = "https://sigil.systems/builder/workbench"
)

type DigestSet map[string]string

type Subject struct {
	Name   string    `json:"name"`
	Digest DigestSet `json:"digest"`
}

type Statement struct {
	Type          string          `json:"_type"`
	Subject       []Subject       `json:"subject"`
	PredicateType string          `json:"predicateType"`
	Predicate     json.RawMessage `json:"predicate"`
}

type ActorKind string

const (
	ActorHuman ActorKind = "human"
	ActorAgent ActorKind = "agent"
)

// Actor identifies who or what produced a change. Model is declared by the operator,
// never inferred, and is recorded for evidence rather than trusted for policy.
type Actor struct {
	Kind        ActorKind `json:"kind"`
	ID          string    `json:"id"`
	Tool        string    `json:"tool,omitempty"`
	ToolVersion string    `json:"toolVersion,omitempty"`
	Model       string    `json:"model,omitempty"`
	Host        string    `json:"host,omitempty"`
}

func (a Actor) String() string {
	if a.Tool != "" {
		return fmt.Sprintf("%s:%s(%s)", a.Kind, a.ID, a.Tool)
	}
	return fmt.Sprintf("%s:%s", a.Kind, a.ID)
}

type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

func (r LineRange) Len() int { return r.End - r.Start + 1 }

type FileStatus string

const (
	FileAdded    FileStatus = "added"
	FileModified FileStatus = "modified"
	FileDeleted  FileStatus = "deleted"
	FileRenamed  FileStatus = "renamed"
)

type FileChange struct {
	Path         string      `json:"path"`
	OldPath      string      `json:"oldPath,omitempty"`
	Status       FileStatus  `json:"status"`
	LinesAdded   int         `json:"linesAdded"`
	LinesDeleted int         `json:"linesDeleted"`
	Binary       bool        `json:"binary,omitempty"`
	Hunks        []Hunk      `json:"hunks,omitempty"`
	Added        []LineRange `json:"added,omitempty"`
}

// Hunk is a zero-context diff hunk in unified-diff coordinates.
type Hunk struct {
	OldStart int `json:"oldStart"`
	OldLines int `json:"oldLines"`
	NewStart int `json:"newStart"`
	NewLines int `json:"newLines"`
}

type SegmentKind string

const (
	SegmentSession SegmentKind = "session"
	SegmentManual  SegmentKind = "manual"
)

// Segment is a contiguous, observed interval of change within a workspace.
type Segment struct {
	ID               string       `json:"id"`
	Kind             SegmentKind  `json:"kind"`
	Actor            Actor        `json:"actor"`
	Command          []string     `json:"command,omitempty"`
	StartedAt        time.Time    `json:"startedAt"`
	EndedAt          time.Time    `json:"endedAt"`
	BaseTree         string       `json:"baseTree"`
	ResultTree       string       `json:"resultTree"`
	ExitCode         *int         `json:"exitCode,omitempty"`
	TranscriptDigest DigestSet    `json:"transcriptDigest,omitempty"`
	Files            []FileChange `json:"files"`
}

type AttributedRange struct {
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Segment string `json:"segment"`
}

type FileAttribution struct {
	Path   string            `json:"path"`
	Ranges []AttributedRange `json:"ranges"`
}

type Summary struct {
	FilesChanged int               `json:"filesChanged"`
	LinesByKind  map[ActorKind]int `json:"linesByKind"`
	Segments     int               `json:"segments"`
}

type PolicyEffect string

const (
	EffectAllow           PolicyEffect = "allow"
	EffectWarn            PolicyEffect = "warn"
	EffectRequireApproval PolicyEffect = "require-approval"
	EffectDeny            PolicyEffect = "deny"
)

type Decision struct {
	Rule      string       `json:"rule"`
	Effect    PolicyEffect `json:"effect"`
	Message   string       `json:"message,omitempty"`
	Paths     []string     `json:"paths,omitempty"`
	Approvals int          `json:"approvals,omitempty"`
	Roles     []string     `json:"roles,omitempty"`
}

type PolicyResult struct {
	Digest    DigestSet  `json:"digest"`
	Decisions []Decision `json:"decisions"`
}

type Builder struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// ChangePredicate records the observed history behind a single commit.
type ChangePredicate struct {
	Repository  string            `json:"repository,omitempty"`
	Commit      string            `json:"commit"`
	Tree        string            `json:"tree"`
	Parents     []string          `json:"parents"`
	Workspace   string            `json:"workspace,omitempty"`
	Branch      string            `json:"branch,omitempty"`
	Committer   Actor             `json:"committer"`
	Segments    []Segment         `json:"segments"`
	Attribution []FileAttribution `json:"attribution"`
	Summary     Summary           `json:"summary"`
	Policy      *PolicyResult     `json:"policy,omitempty"`
	Builder     Builder           `json:"builder"`
	CreatedAt   time.Time         `json:"createdAt"`
}

type ApprovalDecision string

const (
	Approve ApprovalDecision = "approve"
	Reject  ApprovalDecision = "reject"
)

// ApprovalPredicate records a human review decision over a commit.
type ApprovalPredicate struct {
	Commit    string           `json:"commit"`
	Approver  Actor            `json:"approver"`
	Role      string           `json:"role,omitempty"`
	Rules     []string         `json:"rules,omitempty"`
	Decision  ApprovalDecision `json:"decision"`
	Comment   string           `json:"comment,omitempty"`
	CreatedAt time.Time        `json:"createdAt"`
}

var ErrWrongPredicate = errors.New("provenance: unexpected predicate type")

// NewStatement builds a statement for a git commit subject.
func NewStatement(commit, predicateType string, predicate any) (*Statement, error) {
	raw, err := json.Marshal(predicate)
	if err != nil {
		return nil, fmt.Errorf("provenance: encode predicate: %w", err)
	}
	return &Statement{
		Type:          StatementType,
		Subject:       []Subject{{Name: "git+commit:" + commit, Digest: DigestSet{"gitCommit": commit}}},
		PredicateType: predicateType,
		Predicate:     raw,
	}, nil
}

// Commit returns the gitCommit digest of the first subject.
func (s *Statement) Commit() string {
	if len(s.Subject) == 0 {
		return ""
	}
	return s.Subject[0].Digest["gitCommit"]
}

func (s *Statement) Change() (*ChangePredicate, error) {
	if s.PredicateType != PredicateChange {
		return nil, fmt.Errorf("%w: %s", ErrWrongPredicate, s.PredicateType)
	}
	var p ChangePredicate
	if err := json.Unmarshal(s.Predicate, &p); err != nil {
		return nil, fmt.Errorf("provenance: decode change predicate: %w", err)
	}
	return &p, nil
}

func (s *Statement) Approval() (*ApprovalPredicate, error) {
	if s.PredicateType != PredicateApproval {
		return nil, fmt.Errorf("%w: %s", ErrWrongPredicate, s.PredicateType)
	}
	var p ApprovalPredicate
	if err := json.Unmarshal(s.Predicate, &p); err != nil {
		return nil, fmt.Errorf("provenance: decode approval predicate: %w", err)
	}
	return &p, nil
}

// Seal encodes the statement and signs it into a DSSE envelope.
func (s *Statement) Seal(signers ...dsse.Signer) (*dsse.Envelope, error) {
	payload, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return dsse.Sign(dsse.PayloadTypeInToto, payload, signers...)
}

// Open decodes the statement inside an envelope without verifying it.
func Open(env *dsse.Envelope) (*Statement, error) {
	if env.PayloadType != dsse.PayloadTypeInToto {
		return nil, fmt.Errorf("provenance: unexpected payload type %q", env.PayloadType)
	}
	payload, err := env.DecodePayload()
	if err != nil {
		return nil, err
	}
	var s Statement
	if err := json.Unmarshal(payload, &s); err != nil {
		return nil, fmt.Errorf("provenance: decode statement: %w", err)
	}
	if s.Type != StatementType {
		return nil, fmt.Errorf("provenance: unexpected statement type %q", s.Type)
	}
	return &s, nil
}

// Digest returns the SHA-256 of the envelope payload, used as a stable attestation ID.
func Digest(env *dsse.Envelope) string {
	payload, err := env.DecodePayload()
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
