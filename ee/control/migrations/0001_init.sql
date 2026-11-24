CREATE TABLE orgs (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  slug        TEXT NOT NULL UNIQUE,
  plan        TEXT NOT NULL DEFAULT 'team',
  created_at  TEXT NOT NULL
);

CREATE TABLE users (
  id          TEXT PRIMARY KEY,
  org_id      TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  email       TEXT NOT NULL,
  role        TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'auditor', 'ci')),
  created_at  TEXT NOT NULL,
  UNIQUE (org_id, email)
);

CREATE TABLE api_tokens (
  id           TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  hash         TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL,
  created_at   TEXT NOT NULL,
  last_used_at TEXT,
  expires_at   TEXT,
  revoked      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE projects (
  id          TEXT PRIMARY KEY,
  org_id      TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  repository  TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL,
  UNIQUE (org_id, name)
);

CREATE TABLE trusted_keys (
  id          TEXT NOT NULL,
  org_id      TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  principal   TEXT NOT NULL,
  kind        TEXT NOT NULL CHECK (kind IN ('human', 'machine')),
  roles       TEXT NOT NULL DEFAULT '[]',
  public_key  TEXT NOT NULL,
  revoked     INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL,
  created_by  TEXT NOT NULL,
  PRIMARY KEY (org_id, id)
);

CREATE TABLE policies (
  project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  version     INTEGER NOT NULL,
  document    TEXT NOT NULL,
  digest      TEXT NOT NULL,
  created_at  TEXT NOT NULL,
  created_by  TEXT NOT NULL,
  PRIMARY KEY (project_id, version)
);

CREATE TABLE attestations (
  digest          TEXT NOT NULL,
  project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  commit_sha      TEXT NOT NULL,
  predicate_type  TEXT NOT NULL,
  envelope        TEXT NOT NULL,
  signer          TEXT NOT NULL DEFAULT '',
  verified        INTEGER NOT NULL DEFAULT 0,
  status          TEXT NOT NULL DEFAULT '',
  human_lines     INTEGER NOT NULL DEFAULT 0,
  agent_lines     INTEGER NOT NULL DEFAULT 0,
  tools           TEXT NOT NULL DEFAULT '[]',
  models          TEXT NOT NULL DEFAULT '[]',
  pending_rules   TEXT NOT NULL DEFAULT '[]',
  received_at     TEXT NOT NULL,
  PRIMARY KEY (project_id, digest)
);

CREATE INDEX attestations_commit ON attestations (project_id, commit_sha);

CREATE TABLE audit_events (
  org_id      TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  seq         INTEGER NOT NULL,
  at          TEXT NOT NULL,
  actor       TEXT NOT NULL,
  action      TEXT NOT NULL,
  subject     TEXT NOT NULL DEFAULT '',
  data        TEXT NOT NULL DEFAULT '{}',
  prev        TEXT NOT NULL,
  hash        TEXT NOT NULL,
  PRIMARY KEY (org_id, seq)
);
