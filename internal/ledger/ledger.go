// Package ledger implements Seralith's append-only, hash-chained audit log.
package ledger

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/ryan-tobin/new-startup/internal/fsutil"
	"github.com/ryan-tobin/new-startup/pkg/dsse"
)

const (
	GenesisHash    = "0000000000000000000000000000000000000000000000000000000000000000"
	TypeCheckpoint = "ledger.checkpoint"
)

type Entry struct {
	Seq     uint64          `json:"seq"`
	Time    time.Time       `json:"time"`
	Type    string          `json:"type"`
	Actor   string          `json:"actor,omitempty"`
	Subject string          `json:"subject,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Prev    string          `json:"prev"`
	Hash    string          `json:"hash"`
}

// ComputeHash returns the SHA-256 of the entry's canonical encoding with Hash cleared.
func (e Entry) ComputeHash() (string, error) {
	e.Hash = ""
	if len(e.Data) > 0 {
		var buf bytes.Buffer
		if err := json.Compact(&buf, e.Data); err != nil {
			return "", fmt.Errorf("ledger: compact data: %w", err)
		}
		e.Data = buf.Bytes()
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

type Checkpoint struct {
	Seq   uint64 `json:"seq"`
	Head  string `json:"head"`
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}

func checkpointMessage(seq uint64, head string) []byte {
	return []byte("seralith-ledger-checkpoint:v1:" + strconv.FormatUint(seq, 10) + ":" + head)
}

type Ledger struct {
	path string
	mu   sync.Mutex
	now  func() time.Time
}

// Open returns a ledger backed by the JSON Lines file at path, creating it if needed.
func Open(path string) (*Ledger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("ledger: open: %w", err)
	}
	_ = f.Close()
	return &Ledger{path: path, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (l *Ledger) Path() string { return l.path }

// Append adds an entry, holding an exclusive file lock so concurrent processes stay ordered.
func (l *Ledger) Append(typ, actor, subject string, data any) (*Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(l.path, os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("ledger: open for append: %w", err)
	}
	defer f.Close()
	if err := fsutil.Lock(f); err != nil {
		return nil, fmt.Errorf("ledger: lock: %w", err)
	}
	defer func() { _ = fsutil.Unlock(f) }()

	head, err := readLast(f)
	if err != nil {
		return nil, err
	}
	e := Entry{
		Seq:     1,
		Time:    l.now().Truncate(time.Microsecond),
		Type:    typ,
		Actor:   actor,
		Subject: subject,
		Prev:    GenesisHash,
	}
	if head != nil {
		e.Seq = head.Seq + 1
		e.Prev = head.Hash
	}
	if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("ledger: encode data: %w", err)
		}
		e.Data = raw
	}
	if e.Hash, err = e.ComputeHash(); err != nil {
		return nil, err
	}
	line, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return nil, fmt.Errorf("ledger: write: %w", err)
	}
	if err := f.Sync(); err != nil {
		return nil, fmt.Errorf("ledger: sync: %w", err)
	}
	return &e, nil
}

// Checkpoint signs the current head so that rewriting history requires the signing key.
func (l *Ledger) Checkpoint(signer dsse.Signer) (*Entry, error) {
	head, err := l.Head()
	if err != nil {
		return nil, err
	}
	if head == nil {
		return nil, errors.New("ledger: cannot checkpoint an empty ledger")
	}
	sig, err := signer.Sign(checkpointMessage(head.Seq, head.Hash))
	if err != nil {
		return nil, err
	}
	cp := Checkpoint{Seq: head.Seq, Head: head.Hash, KeyID: signer.KeyID(), Sig: base64.StdEncoding.EncodeToString(sig)}
	return l.Append(TypeCheckpoint, signer.KeyID(), head.Hash, cp)
}

// Head returns the last entry, or nil for an empty ledger.
func (l *Ledger) Head() (*Entry, error) {
	f, err := os.Open(l.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readLast(f)
}

// Filter selects entries. Zero values match everything.
type Filter struct {
	Type    string
	Subject string
	Since   uint64
	Limit   int
}

func (flt Filter) match(e *Entry) bool {
	if flt.Type != "" && e.Type != flt.Type {
		return false
	}
	if flt.Subject != "" && e.Subject != flt.Subject {
		return false
	}
	return e.Seq > flt.Since
}

// Entries returns matching entries in order. With a Limit, the most recent are returned.
func (l *Ledger) Entries(flt Filter) ([]Entry, error) {
	var out []Entry
	err := l.scan(func(e *Entry) error {
		if flt.match(e) {
			out = append(out, *e)
			if flt.Limit > 0 && len(out) > flt.Limit {
				out = out[1:]
			}
		}
		return nil
	})
	return out, err
}

func (l *Ledger) scan(fn func(*Entry) error) error {
	f, err := os.Open(l.path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return fmt.Errorf("ledger: line %d: %w", line, err)
		}
		if err := fn(&e); err != nil {
			return err
		}
	}
	return sc.Err()
}

type Issue struct {
	Seq     uint64 `json:"seq"`
	Problem string `json:"problem"`
}

type Report struct {
	Entries     int     `json:"entries"`
	Head        string  `json:"head"`
	Checkpoints int     `json:"checkpoints"`
	Verified    int     `json:"verifiedCheckpoints"`
	Issues      []Issue `json:"issues,omitempty"`
}

func (r *Report) OK() bool { return len(r.Issues) == 0 }

// Verify walks the whole chain, checking sequence, links, hashes, and checkpoint
// signatures. Checkpoints from keys unknown to resolver are counted but not verified.
func (l *Ledger) Verify(resolver dsse.KeyResolver) (*Report, error) {
	rep := &Report{}
	prevHash := GenesisHash
	var prevSeq uint64
	hashes := map[uint64]string{}
	err := l.scan(func(e *Entry) error {
		rep.Entries++
		if e.Seq != prevSeq+1 {
			rep.Issues = append(rep.Issues, Issue{e.Seq, fmt.Sprintf("sequence gap: expected %d", prevSeq+1)})
		}
		if e.Prev != prevHash {
			rep.Issues = append(rep.Issues, Issue{e.Seq, "broken link to previous entry"})
		}
		computed, err := e.ComputeHash()
		if err != nil {
			return err
		}
		if computed != e.Hash {
			rep.Issues = append(rep.Issues, Issue{e.Seq, "hash mismatch: entry was modified"})
		}
		if e.Type == TypeCheckpoint {
			rep.Checkpoints++
			verifyCheckpoint(e, hashes, resolver, rep)
		}
		hashes[e.Seq] = e.Hash
		prevHash = e.Hash
		prevSeq = e.Seq
		return nil
	})
	if err != nil {
		return nil, err
	}
	rep.Head = prevHash
	return rep, nil
}

func verifyCheckpoint(e *Entry, hashes map[uint64]string, resolver dsse.KeyResolver, rep *Report) {
	var cp Checkpoint
	if err := json.Unmarshal(e.Data, &cp); err != nil {
		rep.Issues = append(rep.Issues, Issue{e.Seq, "malformed checkpoint"})
		return
	}
	if hashes[cp.Seq] != cp.Head {
		rep.Issues = append(rep.Issues, Issue{e.Seq, fmt.Sprintf("checkpoint does not match entry %d", cp.Seq)})
		return
	}
	if resolver == nil {
		return
	}
	v, ok := resolver.Resolve(cp.KeyID)
	if !ok {
		return
	}
	sig, err := base64.StdEncoding.DecodeString(cp.Sig)
	if err != nil || v.Verify(checkpointMessage(cp.Seq, cp.Head), sig) != nil {
		rep.Issues = append(rep.Issues, Issue{e.Seq, "invalid checkpoint signature"})
		return
	}
	rep.Verified++
}

func readLast(f *os.File) (*Entry, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return nil, nil
	}
	const chunk = 8192
	var tail []byte
	offset := size
	for offset > 0 {
		n := int64(chunk)
		if offset < n {
			n = offset
		}
		offset -= n
		buf := make([]byte, n, n+int64(len(tail)))
		if _, err := f.ReadAt(buf, offset); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		tail = append(buf, tail...)
		trimmed := bytes.TrimRight(tail, "\n")
		if idx := bytes.LastIndexByte(trimmed, '\n'); idx >= 0 {
			tail = trimmed[idx+1:]
			break
		}
		if offset == 0 {
			tail = trimmed
		}
	}
	tail = bytes.TrimSpace(tail)
	if len(tail) == 0 {
		return nil, nil
	}
	var e Entry
	if err := json.Unmarshal(tail, &e); err != nil {
		return nil, fmt.Errorf("ledger: corrupt tail entry: %w", err)
	}
	return &e, nil
}
