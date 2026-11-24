package control

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

const genesis = "0000000000000000000000000000000000000000000000000000000000000000"

type AuditEvent struct {
	Seq     int64           `json:"seq"`
	At      string          `json:"at"`
	Actor   string          `json:"actor"`
	Action  string          `json:"action"`
	Subject string          `json:"subject"`
	Data    json.RawMessage `json:"data"`
	Prev    string          `json:"prev"`
	Hash    string          `json:"hash"`
}

func auditHash(orgID string, e AuditEvent) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s", orgID, e.Seq, e.At, e.Actor, e.Action, e.Subject, e.Data, e.Prev)
	return hex.EncodeToString(h.Sum(nil))
}

// Audit appends a hash-chained event to the organization's audit trail.
func (s *Store) Audit(ctx context.Context, orgID, actor, action, subject string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if data == nil {
		raw = []byte("{}")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	e := AuditEvent{At: s.ts(), Actor: actor, Action: action, Subject: subject, Data: raw, Prev: genesis}
	var lastSeq sql.NullInt64
	var lastHash sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT seq, hash FROM audit_events WHERE org_id = ? ORDER BY seq DESC LIMIT 1`, orgID).Scan(&lastSeq, &lastHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	e.Seq = 1
	if lastSeq.Valid {
		e.Seq = lastSeq.Int64 + 1
		e.Prev = lastHash.String
	}
	e.Hash = auditHash(orgID, e)
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events (org_id, seq, at, actor, action, subject, data, prev, hash) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		orgID, e.Seq, e.At, e.Actor, e.Action, e.Subject, string(e.Data), e.Prev, e.Hash); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AuditEvents(ctx context.Context, orgID string, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `SELECT seq, at, actor, action, subject, data, prev, hash FROM (
		SELECT * FROM audit_events WHERE org_id = ? ORDER BY seq DESC LIMIT ?) ORDER BY seq`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		var data string
		if err := rows.Scan(&e.Seq, &e.At, &e.Actor, &e.Action, &e.Subject, &data, &e.Prev, &e.Hash); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(data)
		out = append(out, e)
	}
	return out, rows.Err()
}

type AuditReport struct {
	Events int      `json:"events"`
	Head   string   `json:"head"`
	Issues []string `json:"issues"`
}

// VerifyAudit recomputes the organization's full audit chain.
func (s *Store) VerifyAudit(ctx context.Context, orgID string) (*AuditReport, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT seq, at, actor, action, subject, data, prev, hash FROM audit_events WHERE org_id = ? ORDER BY seq`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rep := &AuditReport{Head: genesis, Issues: []string{}}
	var expectSeq int64 = 1
	for rows.Next() {
		var e AuditEvent
		var data string
		if err := rows.Scan(&e.Seq, &e.At, &e.Actor, &e.Action, &e.Subject, &data, &e.Prev, &e.Hash); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(data)
		rep.Events++
		if e.Seq != expectSeq {
			rep.Issues = append(rep.Issues, fmt.Sprintf("event %d: sequence gap", e.Seq))
		}
		if e.Prev != rep.Head {
			rep.Issues = append(rep.Issues, fmt.Sprintf("event %d: broken link", e.Seq))
		}
		if auditHash(orgID, e) != e.Hash {
			rep.Issues = append(rep.Issues, fmt.Sprintf("event %d: hash mismatch", e.Seq))
		}
		rep.Head = e.Hash
		expectSeq = e.Seq + 1
	}
	return rep, rows.Err()
}
