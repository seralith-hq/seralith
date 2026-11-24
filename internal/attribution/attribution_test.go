package attribution_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ryan-tobin/new-startup/internal/attribution"
	"github.com/ryan-tobin/new-startup/internal/gitx"
	"github.com/ryan-tobin/new-startup/internal/testutil"
	"github.com/ryan-tobin/new-startup/pkg/provenance"
)

type step struct {
	segment string
	files   map[string]string
	remove  []string
}

func replay(t *testing.T, initial map[string]string, steps []step) []provenance.FileAttribution {
	t.Helper()
	ctx := context.Background()
	g := testutil.NewGitRepo(t)
	for p, c := range initial {
		g.Write(p, c)
	}
	g.Commit("base")
	repo, _ := gitx.Open(g.Dir)
	prev, err := repo.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tr := attribution.NewTracker()
	for _, s := range steps {
		for p, c := range s.files {
			g.Write(p, c)
		}
		for _, p := range s.remove {
			if err := os.Remove(filepath.Join(g.Dir, p)); err != nil {
				t.Fatal(err)
			}
		}
		next, err := repo.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		changes, err := repo.DiffTrees(ctx, prev, next)
		if err != nil {
			t.Fatal(err)
		}
		tr.Apply(s.segment, changes)
		prev = next
	}
	return tr.Result()
}

func TestAttributionAcrossSegments(t *testing.T) {
	L := testutil.Lines
	got := replay(t,
		map[string]string{"a.go": L("1", "2", "3", "4", "5")},
		[]step{
			{segment: "agent", files: map[string]string{"a.go": L("1", "2", "A1", "A2", "3", "4", "5")}},
			{segment: "human", files: map[string]string{"a.go": L("H0", "1", "2", "A1", "A2-edited", "3", "4", "5")}},
			{segment: "agent2", files: map[string]string{"b.go": L("x", "y"), "a.go": L("H0", "1", "2", "A1", "A2-edited", "3", "5")}},
		})
	want := []provenance.FileAttribution{
		{Path: "a.go", Ranges: []provenance.AttributedRange{
			{Start: 1, End: 1, Segment: "human"},
			{Start: 4, End: 4, Segment: "agent"},
			{Start: 5, End: 5, Segment: "human"},
		}},
		{Path: "b.go", Ranges: []provenance.AttributedRange{{Start: 1, End: 2, Segment: "agent2"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestAttributionNewFileRewrittenAndDeleted(t *testing.T) {
	L := testutil.Lines
	got := replay(t,
		map[string]string{"keep.txt": "k\n"},
		[]step{
			{segment: "s1", files: map[string]string{"n.txt": L("a", "b", "c"), "tmp.txt": "t\n"}},
			{segment: "s2", files: map[string]string{"n.txt": L("a", "B", "c")}, remove: []string{"tmp.txt"}},
		})
	want := []provenance.FileAttribution{
		{Path: "n.txt", Ranges: []provenance.AttributedRange{
			{Start: 1, End: 1, Segment: "s1"},
			{Start: 2, End: 2, Segment: "s2"},
			{Start: 3, End: 3, Segment: "s1"},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestAttributionAppendAtEnd(t *testing.T) {
	L := testutil.Lines
	got := replay(t,
		map[string]string{"f": L("1", "2")},
		[]step{{segment: "s", files: map[string]string{"f": L("1", "2", "3", "4")}}})
	want := []provenance.FileAttribution{{Path: "f", Ranges: []provenance.AttributedRange{{Start: 3, End: 4, Segment: "s"}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestSummarize(t *testing.T) {
	segs := []provenance.Segment{
		{ID: "a", Actor: provenance.Actor{Kind: provenance.ActorAgent}, Files: []provenance.FileChange{{Path: "x"}, {Path: "y"}}},
		{ID: "h", Actor: provenance.Actor{Kind: provenance.ActorHuman}, Files: []provenance.FileChange{{Path: "x"}}},
	}
	attr := []provenance.FileAttribution{{Path: "x", Ranges: []provenance.AttributedRange{{Start: 1, End: 10, Segment: "a"}, {Start: 11, End: 12, Segment: "h"}}}}
	s := attribution.Summarize(segs, attr)
	if s.FilesChanged != 2 || s.Segments != 2 || s.LinesByKind[provenance.ActorAgent] != 10 || s.LinesByKind[provenance.ActorHuman] != 2 {
		t.Fatalf("summary = %+v", s)
	}
}
