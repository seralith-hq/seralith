// Package attribution replays observed change segments to compute which segment
// authored each line of each file.
package attribution

import (
	"sort"

	"github.com/seralith-hq/seralith/pkg/provenance"
)

const baseline = ""

// unbounded stands in for the unknown length of content that predates tracking.
const unbounded = 1 << 40

type run struct {
	segment string
	length  int
}

type file struct {
	runs []run
}

func (f *file) delete(pos, n int) {
	if n <= 0 {
		return
	}
	out := make([]run, 0, len(f.runs)+1)
	offset := 0
	for _, r := range f.runs {
		start, end := offset, offset+r.length
		offset = end
		cutStart, cutEnd := max(start, pos), min(end, pos+n)
		if cutStart >= cutEnd {
			out = append(out, r)
			continue
		}
		if before := cutStart - start; before > 0 {
			out = append(out, run{r.segment, before})
		}
		if after := end - cutEnd; after > 0 {
			out = append(out, run{r.segment, after})
		}
	}
	f.runs = merge(out)
}

func (f *file) insert(pos int, segment string, n int) {
	if n <= 0 {
		return
	}
	out := make([]run, 0, len(f.runs)+2)
	offset := 0
	inserted := false
	for _, r := range f.runs {
		start, end := offset, offset+r.length
		offset = end
		if !inserted && pos >= start && pos < end {
			if pos > start {
				out = append(out, run{r.segment, pos - start})
			}
			out = append(out, run{segment, n})
			if end > pos {
				out = append(out, run{r.segment, end - pos})
			}
			inserted = true
			continue
		}
		out = append(out, r)
	}
	if !inserted {
		out = append(out, run{segment, n})
	}
	f.runs = merge(out)
}

func merge(runs []run) []run {
	out := runs[:0:0]
	for _, r := range runs {
		if r.length <= 0 {
			continue
		}
		if n := len(out); n > 0 && out[n-1].segment == r.segment {
			out[n-1].length += r.length
			continue
		}
		out = append(out, r)
	}
	return out
}

// Tracker accumulates per-line authorship across segments applied in order.
type Tracker struct {
	files map[string]*file
}

func NewTracker() *Tracker {
	return &Tracker{files: map[string]*file{}}
}

func (t *Tracker) get(path string, status provenance.FileStatus) *file {
	if f, ok := t.files[path]; ok {
		return f
	}
	f := &file{}
	if status != provenance.FileAdded {
		f.runs = []run{{baseline, unbounded}}
	}
	t.files[path] = f
	return f
}

// Apply records the changes made by segment. Hunks are applied bottom-up so that
// old-side coordinates stay valid throughout.
func (t *Tracker) Apply(segment string, changes []provenance.FileChange) {
	for _, c := range changes {
		switch c.Status {
		case provenance.FileDeleted:
			delete(t.files, c.Path)
			continue
		case provenance.FileRenamed:
			if f, ok := t.files[c.OldPath]; ok {
				delete(t.files, c.OldPath)
				t.files[c.Path] = f
			}
		case provenance.FileAdded:
			delete(t.files, c.Path)
		}
		f := t.get(c.Path, c.Status)
		if c.Binary {
			f.runs = []run{{segment, 1}}
			continue
		}
		hunks := append([]provenance.Hunk(nil), c.Hunks...)
		sort.Slice(hunks, func(i, j int) bool { return hunks[i].OldStart > hunks[j].OldStart })
		for _, h := range hunks {
			pos := h.OldStart
			if h.OldLines > 0 {
				pos = h.OldStart - 1
				f.delete(pos, h.OldLines)
			}
			f.insert(pos, segment, h.NewLines)
		}
	}
}

// Result returns attributed ranges for every line not present before tracking began.
func (t *Tracker) Result() []provenance.FileAttribution {
	paths := make([]string, 0, len(t.files))
	for p := range t.files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var out []provenance.FileAttribution
	for _, p := range paths {
		var ranges []provenance.AttributedRange
		line := 1
		for _, r := range t.files[p].runs {
			if r.segment != baseline && r.length < unbounded/2 {
				ranges = append(ranges, provenance.AttributedRange{Start: line, End: line + r.length - 1, Segment: r.segment})
			}
			line += r.length
		}
		if len(ranges) > 0 {
			out = append(out, provenance.FileAttribution{Path: p, Ranges: ranges})
		}
	}
	return out
}

// Summarize counts surviving attributed lines per actor kind.
func Summarize(segments []provenance.Segment, attribution []provenance.FileAttribution) provenance.Summary {
	kinds := map[string]provenance.ActorKind{}
	files := map[string]bool{}
	for _, s := range segments {
		kinds[s.ID] = s.Actor.Kind
		for _, f := range s.Files {
			files[f.Path] = true
		}
	}
	sum := provenance.Summary{
		FilesChanged: len(files),
		LinesByKind:  map[provenance.ActorKind]int{},
		Segments:     len(segments),
	}
	for _, fa := range attribution {
		for _, r := range fa.Ranges {
			sum.LinesByKind[kinds[r.Segment]] += r.End - r.Start + 1
		}
	}
	return sum
}
