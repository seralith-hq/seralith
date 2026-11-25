package gitx

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/seralith-hq/seralith/pkg/provenance"
)

// DiffTrees returns per-file changes between two tree-ish objects, including
// zero-context hunks and the line ranges added on the new side.
func (r *Repo) DiffTrees(ctx context.Context, from, to string) ([]provenance.FileChange, error) {
	if from == "" {
		from = EmptyTree
	}
	if from == to {
		return nil, nil
	}
	status, err := r.RunRaw(ctx, nil, "diff", "--no-ext-diff", "--name-status", "-z", "-M", from, to)
	if err != nil {
		return nil, err
	}
	changes, err := parseNameStatus(status)
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, nil
	}
	numstat, err := r.RunRaw(ctx, nil, "diff", "--no-ext-diff", "--numstat", "-z", "-M", from, to)
	if err != nil {
		return nil, err
	}
	counts := parseNumstat(numstat)
	patch, err := r.RunRaw(ctx, nil, "diff", "--no-ext-diff", "-U0", "-M", "--src-prefix=a/", "--dst-prefix=b/", from, to)
	if err != nil {
		return nil, err
	}
	hunks := parseHunks(patch)
	for i := range changes {
		c := &changes[i]
		if n, ok := counts[c.Path]; ok {
			c.LinesAdded, c.LinesDeleted, c.Binary = n.added, n.deleted, n.binary
		}
		c.Hunks = hunks[c.Path]
		for _, h := range c.Hunks {
			if h.NewLines > 0 {
				c.Added = append(c.Added, provenance.LineRange{Start: h.NewStart, End: h.NewStart + h.NewLines - 1})
			}
		}
	}
	return changes, nil
}

// ChangedPaths returns the paths that differ between two tree-ish objects.
func (r *Repo) ChangedPaths(ctx context.Context, from, to string) ([]string, error) {
	changes, err := r.DiffTrees(ctx, from, to)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(changes))
	for _, c := range changes {
		paths = append(paths, c.Path)
	}
	return paths, nil
}

// UnifiedDiff returns a conventional patch between two tree-ish objects.
func (r *Repo) UnifiedDiff(ctx context.Context, from, to string, paths ...string) (string, error) {
	if from == "" {
		from = EmptyTree
	}
	args := []string{"diff", "--no-ext-diff", "-M", from, to}
	if len(paths) > 0 {
		args = append(append(args, "--"), paths...)
	}
	out, err := r.RunRaw(ctx, nil, args...)
	return string(out), err
}

func parseNameStatus(data []byte) ([]provenance.FileChange, error) {
	fields := splitZ(data)
	var out []provenance.FileChange
	for i := 0; i < len(fields); {
		code := fields[i]
		i++
		if code == "" {
			continue
		}
		var c provenance.FileChange
		switch code[0] {
		case 'A':
			c.Status = provenance.FileAdded
		case 'D':
			c.Status = provenance.FileDeleted
		case 'R', 'C':
			c.Status = provenance.FileRenamed
			if i+1 >= len(fields) {
				return nil, fmt.Errorf("gitx: truncated rename record")
			}
			c.OldPath = fields[i]
			i++
		default:
			c.Status = provenance.FileModified
		}
		if i >= len(fields) {
			return nil, fmt.Errorf("gitx: truncated name-status record")
		}
		c.Path = fields[i]
		i++
		out = append(out, c)
	}
	return out, nil
}

type lineCounts struct {
	added, deleted int
	binary         bool
}

func parseNumstat(data []byte) map[string]lineCounts {
	out := map[string]lineCounts{}
	fields := splitZ(data)
	for i := 0; i < len(fields); i++ {
		parts := strings.SplitN(fields[i], "\t", 3)
		if len(parts) < 3 {
			continue
		}
		lc := lineCounts{}
		if parts[0] == "-" {
			lc.binary = true
		} else {
			lc.added, _ = strconv.Atoi(parts[0])
			lc.deleted, _ = strconv.Atoi(parts[1])
		}
		path := parts[2]
		if path == "" && i+2 < len(fields) {
			path = fields[i+2]
			i += 2
		}
		out[path] = lc
	}
	return out
}

func parseHunks(patch []byte) map[string][]provenance.Hunk {
	out := map[string][]provenance.Hunk{}
	sc := bufio.NewScanner(bytes.NewReader(patch))
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	var oldPath, current string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			oldPath, current = "", ""
		case strings.HasPrefix(line, "--- "):
			oldPath = strings.TrimPrefix(strings.TrimPrefix(line, "--- "), "a/")
		case strings.HasPrefix(line, "+++ "):
			p := strings.TrimPrefix(line, "+++ ")
			if p == "/dev/null" {
				current = oldPath
			} else {
				current = strings.TrimPrefix(p, "b/")
			}
		case strings.HasPrefix(line, "@@ ") && current != "":
			if h, ok := parseHunkHeader(line); ok {
				out[current] = append(out[current], h)
			}
		}
	}
	return out
}

func parseHunkHeader(line string) (provenance.Hunk, bool) {
	end := strings.Index(line[3:], " @@")
	if end < 0 {
		return provenance.Hunk{}, false
	}
	parts := strings.Fields(line[3 : 3+end])
	if len(parts) != 2 {
		return provenance.Hunk{}, false
	}
	os, ol, ok1 := parseRange(strings.TrimPrefix(parts[0], "-"))
	ns, nl, ok2 := parseRange(strings.TrimPrefix(parts[1], "+"))
	if !ok1 || !ok2 {
		return provenance.Hunk{}, false
	}
	return provenance.Hunk{OldStart: os, OldLines: ol, NewStart: ns, NewLines: nl}, true
}

func parseRange(s string) (start, lines int, ok bool) {
	lines = 1
	if comma := strings.IndexByte(s, ','); comma >= 0 {
		n, err := strconv.Atoi(s[comma+1:])
		if err != nil {
			return 0, 0, false
		}
		lines = n
		s = s[:comma]
	}
	start, err := strconv.Atoi(s)
	if err != nil {
		return 0, 0, false
	}
	return start, lines, true
}

func splitZ(data []byte) []string {
	data = bytes.TrimRight(data, "\x00")
	if len(data) == 0 {
		return nil
	}
	return strings.Split(string(data), "\x00")
}
