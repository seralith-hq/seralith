// Package evidence assembles signed evidence packages for assessors and auditors.
package evidence

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ryan-tobin/new-startup/internal/gitx"
	"github.com/ryan-tobin/new-startup/internal/ledger"
	"github.com/ryan-tobin/new-startup/internal/notes"
	"github.com/ryan-tobin/new-startup/internal/verify"
	"github.com/ryan-tobin/new-startup/internal/version"
	"github.com/ryan-tobin/new-startup/pkg/dsse"
	"github.com/ryan-tobin/new-startup/pkg/provenance"
)

const ManifestPayloadType = "application/vnd.sigil.evidence-manifest+json"

type Input struct {
	Repo       *gitx.Repo
	Range      string
	Gate       *verify.GateReport
	PolicyFile []byte
	TrustFile  []byte
	Ledger     *ledger.Ledger
	Signer     dsse.Signer
	Project    string
	Now        time.Time
}

type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
}

type Manifest struct {
	Type      string         `json:"type"`
	Project   string         `json:"project"`
	Range     string         `json:"range"`
	Generated time.Time      `json:"generated"`
	Generator string         `json:"generator"`
	Status    verify.Status  `json:"gateStatus"`
	Files     []ManifestFile `json:"files"`
}

type ToolUsage struct {
	Tool       string   `json:"tool"`
	Versions   []string `json:"versions"`
	Models     []string `json:"models"`
	Operators  []string `json:"operators"`
	Segments   int      `json:"segments"`
	LinesAdded int      `json:"linesAdded"`
	Commits    int      `json:"commits"`
}

// Build writes a zip evidence package to w.
func Build(ctx context.Context, in Input, w io.Writer) (*Manifest, error) {
	files := map[string][]byte{}
	add := func(name string, v any) error {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		files[name] = append(data, '\n')
		return nil
	}
	if err := add("gate-report.json", in.Gate); err != nil {
		return nil, err
	}
	if err := add("controls.json", Controls); err != nil {
		return nil, err
	}
	files["policy.yaml"] = in.PolicyFile
	files["trust.yaml"] = in.TrustFile

	usage := map[string]*ToolUsage{}
	for _, c := range in.Gate.Commits {
		envs, err := notes.Read(ctx, in.Repo, c.Commit)
		if err != nil {
			return nil, err
		}
		if len(envs) > 0 {
			var buf bytes.Buffer
			for _, e := range envs {
				raw, _ := e.Marshal()
				buf.Write(raw)
				buf.WriteByte('\n')
			}
			files["attestations/"+c.Commit+".jsonl"] = buf.Bytes()
		}
		if c.Predicate != nil {
			collectUsage(usage, c.Predicate)
		}
	}
	if err := add("ai-usage.json", sortedUsage(usage)); err != nil {
		return nil, err
	}
	if in.Ledger != nil {
		data, err := os.ReadFile(in.Ledger.Path())
		if err != nil {
			return nil, err
		}
		files["ledger.jsonl"] = data
		rep, err := in.Ledger.Verify(nil)
		if err != nil {
			return nil, err
		}
		if err := add("ledger-verify.json", rep); err != nil {
			return nil, err
		}
	}
	files["report.md"] = []byte(renderReport(in, sortedUsage(usage)))

	man := &Manifest{
		Type:      "https://sigil.systems/evidence/v1",
		Project:   in.Project,
		Range:     in.Range,
		Generated: in.Now,
		Generator: "sigil " + version.Version,
		Status:    in.Gate.Status,
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		sum := sha256.Sum256(files[n])
		man.Files = append(man.Files, ManifestFile{Path: n, SHA256: hex.EncodeToString(sum[:]), Size: len(files[n])})
	}
	manJSON, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return nil, err
	}
	files["manifest.json"] = manJSON
	if in.Signer != nil {
		env, err := dsse.Sign(ManifestPayloadType, manJSON, in.Signer)
		if err != nil {
			return nil, err
		}
		sig, _ := json.MarshalIndent(env, "", "  ")
		files["manifest.sig.json"] = sig
	}

	zw := zip.NewWriter(w)
	all := make([]string, 0, len(files))
	for n := range files {
		all = append(all, n)
	}
	sort.Strings(all)
	for _, n := range all {
		hdr := &zip.FileHeader{Name: n, Method: zip.Deflate, Modified: in.Now}
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			return nil, err
		}
		if _, err := fw.Write(files[n]); err != nil {
			return nil, err
		}
	}
	return man, zw.Close()
}

func collectUsage(usage map[string]*ToolUsage, pred *provenance.ChangePredicate) {
	seen := map[string]bool{}
	for _, s := range pred.Segments {
		if s.Actor.Kind != provenance.ActorAgent {
			continue
		}
		u, ok := usage[s.Actor.Tool]
		if !ok {
			u = &ToolUsage{Tool: s.Actor.Tool}
			usage[s.Actor.Tool] = u
		}
		u.Segments++
		u.Versions = appendUnique(u.Versions, s.Actor.ToolVersion)
		u.Models = appendUnique(u.Models, s.Actor.Model)
		u.Operators = appendUnique(u.Operators, s.Actor.Operator)
		for _, f := range s.Files {
			u.LinesAdded += f.LinesAdded
		}
		if !seen[s.Actor.Tool] {
			seen[s.Actor.Tool] = true
			u.Commits++
		}
	}
}

func sortedUsage(m map[string]*ToolUsage) []*ToolUsage {
	out := make([]*ToolUsage, 0, len(m))
	for _, u := range m {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tool < out[j].Tool })
	return out
}

func appendUnique(list []string, v string) []string {
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	list = append(list, v)
	sort.Strings(list)
	return list
}

func renderReport(in Input, usage []*ToolUsage) string {
	var b strings.Builder
	g := in.Gate
	fmt.Fprintf(&b, "# Software Provenance Evidence\n\n")
	fmt.Fprintf(&b, "| Field | Value |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| Project | %s |\n| Range | `%s` |\n| Generated | %s |\n| Generator | sigil %s |\n| Gate status | **%s** |\n\n",
		in.Project, in.Range, in.Now.Format(time.RFC3339), version.Version, strings.ToUpper(string(g.Status)))

	fmt.Fprintf(&b, "## Attribution\n\n| Actor kind | Surviving lines |\n| --- | --- |\n")
	kinds := make([]string, 0, len(g.Lines))
	for k := range g.Lines {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(&b, "| %s | %d |\n", k, g.Lines[k])
	}

	fmt.Fprintf(&b, "\n## Commits\n\n| Commit | Subject | Status | Signers | Notes |\n| --- | --- | --- | --- | --- |\n")
	for _, c := range g.Commits {
		notes := append(append([]string{}, c.Problems...), c.Warnings...)
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s |\n", c.Commit[:12], escape(c.Subject), c.Status, strings.Join(c.Signers, ", "), escape(strings.Join(notes, "; ")))
	}

	fmt.Fprintf(&b, "\n## Coding tools used\n\n")
	if len(usage) == 0 {
		fmt.Fprintf(&b, "No coding agents contributed to this range.\n")
	} else {
		fmt.Fprintf(&b, "| Tool | Versions | Declared models | Operators | Segments | Lines added |\n| --- | --- | --- | --- | --- | --- |\n")
		for _, u := range usage {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %d | %d |\n", u.Tool, list(u.Versions), list(u.Models), list(u.Operators), u.Segments, u.LinesAdded)
		}
	}

	fmt.Fprintf(&b, "\n## Control mapping\n\nThe mapping below describes what each artifact demonstrates. It supports, but does not replace, an assessor's determination.\n\n| Framework | Control | Evidence | Artifacts |\n| --- | --- | --- | --- |\n")
	for _, c := range Controls {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", c.Framework, c.ID, c.Evidence, strings.Join(c.Artifacts, ", "))
	}
	fmt.Fprintf(&b, "\n## Verifying this package\n\n```sh\nsigil evidence verify <package.zip> --trust trust.yaml\n```\n")
	return b.String()
}

func list(v []string) string {
	if len(v) == 0 {
		return "-"
	}
	return strings.Join(v, ", ")
}

func escape(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

// VerifyResult describes the outcome of checking an evidence package.
type VerifyResult struct {
	Manifest *Manifest `json:"manifest"`
	Signed   bool      `json:"signed"`
	SignedBy []string  `json:"signedBy,omitempty"`
	Mismatch []string  `json:"mismatch,omitempty"`
	Missing  []string  `json:"missing,omitempty"`
}

func (r *VerifyResult) OK() bool { return r.Signed && len(r.Mismatch) == 0 && len(r.Missing) == 0 }

// Verify checks file hashes against the manifest and the manifest signature against resolver.
func Verify(path string, resolver dsse.KeyResolver) (*VerifyResult, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	contents := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, 512<<20))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		contents[f.Name] = data
	}
	manJSON, ok := contents["manifest.json"]
	if !ok {
		return nil, fmt.Errorf("evidence: manifest.json missing")
	}
	var man Manifest
	if err := json.Unmarshal(manJSON, &man); err != nil {
		return nil, fmt.Errorf("evidence: parse manifest: %w", err)
	}
	res := &VerifyResult{Manifest: &man}
	for _, f := range man.Files {
		data, ok := contents[f.Path]
		if !ok {
			res.Missing = append(res.Missing, f.Path)
			continue
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != f.SHA256 {
			res.Mismatch = append(res.Mismatch, f.Path)
		}
	}
	if sigJSON, ok := contents["manifest.sig.json"]; ok && resolver != nil {
		env, err := dsse.Parse(sigJSON)
		if err == nil {
			payload, _ := env.DecodePayload()
			if bytes.Equal(payload, manJSON) && env.PayloadType == ManifestPayloadType {
				if vr, err := env.Verify(resolver, 1); err == nil {
					res.Signed, res.SignedBy = true, vr.Accepted
				}
			}
		}
	}
	return res, nil
}
