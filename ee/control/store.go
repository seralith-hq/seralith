// Package control implements Seralith Control, the organization control plane.
//
// Copyright 2026 Seralith, Inc. Licensed under the Seralith Commercial License; see ee/LICENSE.
package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

var (
	ErrNotFound = errors.New("control: not found")
	ErrConflict = errors.New("control: already exists")
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open connects to a SQLite database and applies pending migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := path
	if !strings.HasPrefix(dsn, "file:") {
		dsn = "file:" + path
	}
	dsn += "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("control: migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, name, s.ts()); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ts() string { return s.now().Format(time.RFC3339Nano) }

func newID(prefix string) string {
	var b [10]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

type Org struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Plan      string `json:"plan"`
	CreatedAt string `json:"createdAt"`
}

type User struct {
	ID        string `json:"id"`
	OrgID     string `json:"orgId"`
	Email     string `json:"email"`
	Role      Role   `json:"role"`
	CreatedAt string `json:"createdAt"`
}

type Role string

const (
	RoleOwner   Role = "owner"
	RoleAdmin   Role = "admin"
	RoleMember  Role = "member"
	RoleAuditor Role = "auditor"
	RoleCI      Role = "ci"
)

func (r Role) Valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleMember, RoleAuditor, RoleCI:
		return true
	}
	return false
}

// CanAdminister reports whether the role may manage keys, members, and policy.
func (r Role) CanAdminister() bool { return r == RoleOwner || r == RoleAdmin }

// CanWrite reports whether the role may upload attestations.
func (r Role) CanWrite() bool {
	return r == RoleOwner || r == RoleAdmin || r == RoleMember || r == RoleCI
}

func (s *Store) OrgCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM orgs`).Scan(&n)
	return n, err
}

// CreateOrg creates an organization and its first owner.
func (s *Store) CreateOrg(ctx context.Context, name, slug, ownerEmail string) (*Org, *User, error) {
	org := &Org{ID: newID("org"), Name: name, Slug: slug, Plan: "team", CreatedAt: s.ts()}
	user := &User{ID: newID("usr"), OrgID: org.ID, Email: ownerEmail, Role: RoleOwner, CreatedAt: org.CreatedAt}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO orgs (id, name, slug, plan, created_at) VALUES (?, ?, ?, ?, ?)`, org.ID, org.Name, org.Slug, org.Plan, org.CreatedAt); err != nil {
		if isUnique(err) {
			return nil, nil, ErrConflict
		}
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO users (id, org_id, email, role, created_at) VALUES (?, ?, ?, ?, ?)`, user.ID, user.OrgID, user.Email, user.Role, user.CreatedAt); err != nil {
		return nil, nil, err
	}
	return org, user, tx.Commit()
}

func (s *Store) Org(ctx context.Context, id string) (*Org, error) {
	o := &Org{}
	err := s.db.QueryRowContext(ctx, `SELECT id, name, slug, plan, created_at FROM orgs WHERE id = ?`, id).Scan(&o.ID, &o.Name, &o.Slug, &o.Plan, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

func (s *Store) AddUser(ctx context.Context, orgID, email string, role Role) (*User, error) {
	u := &User{ID: newID("usr"), OrgID: orgID, Email: email, Role: role, CreatedAt: s.ts()}
	_, err := s.db.ExecContext(ctx, `INSERT INTO users (id, org_id, email, role, created_at) VALUES (?, ?, ?, ?, ?)`, u.ID, u.OrgID, u.Email, u.Role, u.CreatedAt)
	if isUnique(err) {
		return nil, ErrConflict
	}
	return u, err
}

func (s *Store) Users(ctx context.Context, orgID string) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, org_id, email, role, created_at FROM users WHERE org_id = ? ORDER BY email`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.OrgID, &u.Email, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

type Token struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	CreatedAt string `json:"createdAt"`
	LastUsed  string `json:"lastUsedAt,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Revoked   bool   `json:"revoked"`
}

func hashToken(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// IssueToken creates an API token and returns its secret, which is never stored.
func (s *Store) IssueToken(ctx context.Context, userID, name string, ttl time.Duration) (string, *Token, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", nil, err
	}
	secret := "srl_" + hex.EncodeToString(b[:])
	t := &Token{ID: newID("tok"), UserID: userID, Name: name, Prefix: secret[:12], CreatedAt: s.ts()}
	var expires any
	if ttl > 0 {
		t.ExpiresAt = s.now().Add(ttl).Format(time.RFC3339Nano)
		expires = t.ExpiresAt
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO api_tokens (id, user_id, name, hash, prefix, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.UserID, t.Name, hashToken(secret), t.Prefix, t.CreatedAt, expires)
	return secret, t, err
}

// Authenticate resolves a token secret to its user.
func (s *Store) Authenticate(ctx context.Context, secret string) (*User, error) {
	var u User
	var tokenID string
	var expires sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.org_id, u.email, u.role, u.created_at, t.id, t.expires_at
		FROM api_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.hash = ? AND t.revoked = 0`, hashToken(secret)).
		Scan(&u.ID, &u.OrgID, &u.Email, &u.Role, &u.CreatedAt, &tokenID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if expires.Valid {
		if t, err := time.Parse(time.RFC3339Nano, expires.String); err == nil && s.now().After(t) {
			return nil, ErrNotFound
		}
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, s.ts(), tokenID)
	return &u, nil
}

func (s *Store) Tokens(ctx context.Context, userID string) ([]Token, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, name, prefix, created_at, COALESCE(last_used_at, ''), COALESCE(expires_at, ''), revoked FROM api_tokens WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Token{}
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.CreatedAt, &t.LastUsed, &t.ExpiresAt, &t.Revoked); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) RevokeToken(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET revoked = 1 WHERE id = ? AND user_id = ?`, id, userID)
	return affected(res, err)
}

type Project struct {
	ID         string `json:"id"`
	OrgID      string `json:"orgId"`
	Name       string `json:"name"`
	Repository string `json:"repository"`
	CreatedAt  string `json:"createdAt"`
}

func (s *Store) CreateProject(ctx context.Context, orgID, name, repo string) (*Project, error) {
	p := &Project{ID: newID("prj"), OrgID: orgID, Name: name, Repository: repo, CreatedAt: s.ts()}
	_, err := s.db.ExecContext(ctx, `INSERT INTO projects (id, org_id, name, repository, created_at) VALUES (?, ?, ?, ?, ?)`, p.ID, p.OrgID, p.Name, p.Repository, p.CreatedAt)
	if isUnique(err) {
		return nil, ErrConflict
	}
	return p, err
}

func (s *Store) Projects(ctx context.Context, orgID string) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, org_id, name, repository, created_at FROM projects WHERE org_id = ? ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Repository, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ProjectByName resolves a project within an organization.
func (s *Store) ProjectByName(ctx context.Context, orgID, name string) (*Project, error) {
	p := &Project{}
	err := s.db.QueryRowContext(ctx, `SELECT id, org_id, name, repository, created_at FROM projects WHERE org_id = ? AND name = ?`, orgID, name).
		Scan(&p.ID, &p.OrgID, &p.Name, &p.Repository, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

type TrustedKey struct {
	ID        string   `json:"id"`
	Principal string   `json:"principal"`
	Kind      string   `json:"kind"`
	Roles     []string `json:"roles"`
	PublicKey string   `json:"publicKey"`
	Revoked   bool     `json:"revoked"`
	CreatedAt string   `json:"createdAt"`
	CreatedBy string   `json:"createdBy"`
}

func (s *Store) PutKey(ctx context.Context, orgID string, k TrustedKey) error {
	roles, _ := json.Marshal(k.Roles)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO trusted_keys (id, org_id, principal, kind, roles, public_key, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (org_id, id) DO UPDATE SET principal = excluded.principal, kind = excluded.kind, roles = excluded.roles, revoked = 0`,
		k.ID, orgID, k.Principal, k.Kind, string(roles), k.PublicKey, s.ts(), k.CreatedBy)
	return err
}

func (s *Store) RevokeKey(ctx context.Context, orgID, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE trusted_keys SET revoked = 1 WHERE org_id = ? AND id = ?`, orgID, id)
	return affected(res, err)
}

func (s *Store) Keys(ctx context.Context, orgID string) ([]TrustedKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, principal, kind, roles, public_key, revoked, created_at, created_by FROM trusted_keys WHERE org_id = ? ORDER BY principal, id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrustedKey{}
	for rows.Next() {
		var k TrustedKey
		var roles string
		if err := rows.Scan(&k.ID, &k.Principal, &k.Kind, &roles, &k.PublicKey, &k.Revoked, &k.CreatedAt, &k.CreatedBy); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(roles), &k.Roles)
		out = append(out, k)
	}
	return out, rows.Err()
}

type PolicyVersion struct {
	Version   int    `json:"version"`
	Document  string `json:"document"`
	Digest    string `json:"digest"`
	CreatedAt string `json:"createdAt"`
	CreatedBy string `json:"createdBy"`
}

// PutPolicy stores a new policy version and returns it.
func (s *Store) PutPolicy(ctx context.Context, projectID, document, digest, by string) (*PolicyVersion, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var next int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM policies WHERE project_id = ?`, projectID).Scan(&next); err != nil {
		return nil, err
	}
	pv := &PolicyVersion{Version: next, Document: document, Digest: digest, CreatedAt: s.ts(), CreatedBy: by}
	if _, err := tx.ExecContext(ctx, `INSERT INTO policies (project_id, version, document, digest, created_at, created_by) VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, pv.Version, pv.Document, pv.Digest, pv.CreatedAt, pv.CreatedBy); err != nil {
		return nil, err
	}
	return pv, tx.Commit()
}

func (s *Store) LatestPolicy(ctx context.Context, projectID string) (*PolicyVersion, error) {
	pv := &PolicyVersion{}
	err := s.db.QueryRowContext(ctx, `SELECT version, document, digest, created_at, created_by FROM policies WHERE project_id = ? ORDER BY version DESC LIMIT 1`, projectID).
		Scan(&pv.Version, &pv.Document, &pv.Digest, &pv.CreatedAt, &pv.CreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return pv, err
}

type AttestationRecord struct {
	Digest        string   `json:"digest"`
	Commit        string   `json:"commit"`
	PredicateType string   `json:"predicateType"`
	Envelope      string   `json:"envelope,omitempty"`
	Signer        string   `json:"signer"`
	Verified      bool     `json:"verified"`
	Status        string   `json:"status"`
	HumanLines    int      `json:"humanLines"`
	AgentLines    int      `json:"agentLines"`
	Tools         []string `json:"tools"`
	Models        []string `json:"models"`
	PendingRules  []string `json:"pendingRules"`
	ReceivedAt    string   `json:"receivedAt"`
}

// PutAttestation stores a record, reporting false when it already existed. A verified
// record replaces an unverified one with the same digest, so an earlier upload of a
// statement with missing or wrong signatures cannot hide the genuine envelope.
func (s *Store) PutAttestation(ctx context.Context, projectID string, a AttestationRecord) (bool, error) {
	tools, _ := json.Marshal(nonNil(a.Tools))
	models, _ := json.Marshal(nonNil(a.Models))
	pending, _ := json.Marshal(nonNil(a.PendingRules))
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO attestations (digest, project_id, commit_sha, predicate_type, envelope, signer, verified, status, human_lines, agent_lines, tools, models, pending_rules, received_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (project_id, digest) DO UPDATE SET
			envelope = excluded.envelope, signer = excluded.signer, verified = excluded.verified,
			status = excluded.status, human_lines = excluded.human_lines, agent_lines = excluded.agent_lines,
			tools = excluded.tools, models = excluded.models, pending_rules = excluded.pending_rules
		WHERE attestations.verified = 0 AND excluded.verified = 1`,
		a.Digest, projectID, a.Commit, a.PredicateType, a.Envelope, a.Signer, a.Verified, a.Status, a.HumanLines, a.AgentLines, string(tools), string(models), string(pending), s.ts())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

type AttestationQuery struct {
	Commit        string
	PredicateType string
	Limit         int
	WithEnvelope  bool
}

func (s *Store) Attestations(ctx context.Context, projectID string, q AttestationQuery) ([]AttestationRecord, error) {
	query := `SELECT digest, commit_sha, predicate_type, envelope, signer, verified, status, human_lines, agent_lines, tools, models, pending_rules, received_at FROM attestations WHERE project_id = ?`
	args := []any{projectID}
	if q.Commit != "" {
		query += ` AND commit_sha = ?`
		args = append(args, q.Commit)
	}
	if q.PredicateType != "" {
		query += ` AND predicate_type = ?`
		args = append(args, q.PredicateType)
	}
	limit := q.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	query += ` ORDER BY received_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AttestationRecord{}
	for rows.Next() {
		var a AttestationRecord
		var tools, models, pending string
		if err := rows.Scan(&a.Digest, &a.Commit, &a.PredicateType, &a.Envelope, &a.Signer, &a.Verified, &a.Status, &a.HumanLines, &a.AgentLines, &tools, &models, &pending, &a.ReceivedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tools), &a.Tools)
		_ = json.Unmarshal([]byte(models), &a.Models)
		_ = json.Unmarshal([]byte(pending), &a.PendingRules)
		if !q.WithEnvelope {
			a.Envelope = ""
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
