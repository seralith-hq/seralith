// Package blame combines git blame with change attestations to report which actor
// authored each surviving line of a file.
package blame

import (
	"bufio"
	"bytes"
	"context"
	"strconv"
	"strings"

	"github.com/ryan-tobin/new-startup/internal/gitx"
	"github.com/ryan-tobin/new-startup/internal/keys"
	"github.com/ryan-tobin/new-startup/internal/notes"
	"github.com/ryan-tobin/new-startup/pkg/provenance"
)

type Line struct {
	Number   int              `json:"line"`
	Commit   string           `json:"commit"`
	Original int              `json:"originalLine"`
	Actor    provenance.Actor `json:"actor"`
	Attested bool             `json:"attested"`
	Verified bool             `json:"verified"`
	Text     string           `json:"text"`
}

type Summary struct {
	Lines      int                          `json:"lines"`
	ByKind     map[provenance.ActorKind]int `json:"byKind"`
	ByTool     map[string]int               `json:"byTool"`
	Unattested int                          `json:"unattested"`
}

type commitInfo struct {
	pred     *provenance.ChangePredicate
	verified bool
	segments map[string]provenance.Actor
}

// File attributes every line of path at rev.
func File(ctx context.Context, repo *gitx.Repo, rev, path string, trust *keys.TrustStore) ([]Line, *Summary, error) {
	out, err := repo.RunRaw(ctx, nil, "blame", "--porcelain", rev, "--", path)
	if err != nil {
		return nil, nil, err
	}
	raw := parsePorcelain(out)
	cache := map[string]*commitInfo{}
	lines := make([]Line, 0, len(raw))
	sum := &Summary{ByKind: map[provenance.ActorKind]int{}, ByTool: map[string]int{}}
	for _, r := range raw {
		info, ok := cache[r.commit]
		if !ok {
			info = load(ctx, repo, r.commit, trust)
			cache[r.commit] = info
		}
		l := Line{Number: r.final, Commit: r.commit, Original: r.orig, Text: r.text}
		if info.pred != nil {
			l.Attested, l.Verified = true, info.verified
			l.Actor = actorFor(info, path, r.orig)
		}
		if !l.Attested {
			sum.Unattested++
		} else {
			sum.ByKind[l.Actor.Kind]++
			if l.Actor.Tool != "" {
				sum.ByTool[l.Actor.Tool]++
			}
		}
		lines = append(lines, l)
	}
	sum.Lines = len(lines)
	return lines, sum, nil
}

func load(ctx context.Context, repo *gitx.Repo, commit string, trust *keys.TrustStore) *commitInfo {
	info := &commitInfo{segments: map[string]provenance.Actor{}}
	atts, err := notes.Statements(ctx, repo, commit)
	if err != nil {
		return info
	}
	for _, a := range atts[provenance.PredicateChange] {
		if a.Statement.Commit() != commit {
			continue
		}
		pred, err := a.Statement.Change()
		if err != nil {
			continue
		}
		info.pred = pred
		if trust != nil {
			_, verr := a.Envelope.Verify(trust, 1)
			info.verified = verr == nil
		}
		for _, s := range pred.Segments {
			info.segments[s.ID] = s.Actor
		}
		break
	}
	return info
}

func actorFor(info *commitInfo, path string, line int) provenance.Actor {
	for _, fa := range info.pred.Attribution {
		if fa.Path != path {
			continue
		}
		for _, r := range fa.Ranges {
			if line >= r.Start && line <= r.End {
				return info.segments[r.Segment]
			}
		}
	}
	return info.pred.Committer
}

type rawLine struct {
	commit      string
	orig, final int
	text        string
}

func parsePorcelain(data []byte) []rawLine {
	var out []rawLine
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var cur rawLine
	expectHeader := true
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "\t") {
			cur.text = line[1:]
			out = append(out, cur)
			expectHeader = true
			continue
		}
		if expectHeader {
			fields := strings.Fields(line)
			if len(fields) >= 3 && len(fields[0]) >= 40 {
				cur = rawLine{commit: fields[0]}
				cur.orig, _ = strconv.Atoi(fields[1])
				cur.final, _ = strconv.Atoi(fields[2])
				expectHeader = false
			}
		}
	}
	return out
}
