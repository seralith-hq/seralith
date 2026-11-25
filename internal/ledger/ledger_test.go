package ledger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/seralith-hq/seralith/internal/keys"
)

func newLedger(t *testing.T) *Ledger {
	t.Helper()
	l, err := Open(filepath.Join(t.TempDir(), "ledger.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestAppendAndVerify(t *testing.T) {
	l := newLedger(t)
	for i := 0; i < 5; i++ {
		if _, err := l.Append("session.started", "human:alice", "s1", map[string]int{"n": i}); err != nil {
			t.Fatal(err)
		}
	}
	head, err := l.Head()
	if err != nil || head.Seq != 5 {
		t.Fatalf("head=%+v err=%v", head, err)
	}
	rep, err := l.Verify(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() || rep.Entries != 5 || rep.Head != head.Hash {
		t.Fatalf("report = %+v", rep)
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	l := newLedger(t)
	for i := 0; i < 3; i++ {
		if _, err := l.Append("x", "a", "", map[string]string{"v": "original"}); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(l.Path())
	data = bytes.Replace(data, []byte("original"), []byte("modified"), 1)
	if err := os.WriteFile(l.Path(), data, 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := l.Verify(nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK() || !strings.Contains(rep.Issues[0].Problem, "hash mismatch") {
		t.Fatalf("issues = %+v", rep.Issues)
	}
}

func TestVerifyDetectsDeletion(t *testing.T) {
	l := newLedger(t)
	for i := 0; i < 3; i++ {
		if _, err := l.Append("x", "a", "", nil); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(strings.TrimSpace(readFile(t, l.Path())), "\n")
	kept := lines[0] + "\n" + lines[2] + "\n"
	if err := os.WriteFile(l.Path(), []byte(kept), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, _ := l.Verify(nil)
	if len(rep.Issues) != 2 {
		t.Fatalf("issues = %+v", rep.Issues)
	}
}

func TestCheckpoint(t *testing.T) {
	l := newLedger(t)
	k, _ := keys.Generate()
	if _, err := l.Checkpoint(k); err == nil {
		t.Fatal("checkpointed empty ledger")
	}
	if _, err := l.Append("x", "a", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Checkpoint(k); err != nil {
		t.Fatal(err)
	}
	ts := keys.NewTrustStore()
	if _, err := ts.Add(k.Public(), "machine", keys.KindMachine); err != nil {
		t.Fatal(err)
	}
	rep, err := l.Verify(ts)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() || !rep.Anchored() || rep.Checkpoints != 1 || rep.Verified != 1 {
		t.Fatalf("report = %+v", rep)
	}
	other, _ := keys.Generate()
	ts2 := keys.NewTrustStore()
	if _, err := ts2.Add(other.Public(), "machine", keys.KindMachine); err != nil {
		t.Fatal(err)
	}
	rep2, _ := l.Verify(ts2)
	if rep2.Verified != 0 || !rep2.OK() || rep2.Anchored() || len(rep2.UntrustedKeys) != 1 || rep2.UntrustedKeys[0] != k.KeyID() {
		t.Fatalf("unknown checkpoint key report = %+v", rep2)
	}
	if rep3, _ := l.Verify(nil); rep3.Anchored() {
		t.Fatalf("ledger anchored without a trust store: %+v", rep3)
	}
}

func TestEntriesFilter(t *testing.T) {
	l := newLedger(t)
	for _, typ := range []string{"a", "b", "a", "a", "b"} {
		if _, err := l.Append(typ, "", "subj-"+typ, nil); err != nil {
			t.Fatal(err)
		}
	}
	got, err := l.Entries(Filter{Type: "a"})
	if err != nil || len(got) != 3 {
		t.Fatalf("got %d entries err=%v", len(got), err)
	}
	got, _ = l.Entries(Filter{Limit: 2})
	if len(got) != 2 || got[0].Seq != 4 || got[1].Seq != 5 {
		t.Fatalf("limit entries = %+v", got)
	}
	got, _ = l.Entries(Filter{Since: 3, Subject: "subj-b"})
	if len(got) != 1 || got[0].Seq != 5 {
		t.Fatalf("since entries = %+v", got)
	}
}

func TestConcurrentAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l, err := Open(path)
			if err != nil {
				t.Error(err)
				return
			}
			for i := 0; i < 25; i++ {
				if _, err := l.Append("x", "", "", strings.Repeat("y", 3000)); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	l, _ := Open(path)
	rep, err := l.Verify(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() || rep.Entries != 100 {
		t.Fatalf("report = %+v", rep)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
