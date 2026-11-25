# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Security:** policies take `humanLabels: unverified` (the default) or
  `trusted`. When unverified, agent rules that protect paths also apply to
  human-labeled changes to those paths, with `deny` relaxed to one
  independent approval, so relabeling agent work as human no longer avoids
  review. Existing policies without the field get the new behavior.

- seralith.com deploys to Cloudflare Pages from `main`, with preview deploys
  for pull requests, strict security headers, a published security policy at
  `/security`, an RFC 9116 `security.txt`, a 404 page, and a sitemap.

- `seralith gate` and `seralith verify` accept `--config-from <rev>` to judge a branch
  by the policy and trust store of its base.
- Release automation with GoReleaser, SBOMs, and build provenance; installer
  script with checksum verification; reusable GitHub Action.
- Container images for the CLI and Seralith Control; Docker Compose and Kubernetes
  manifests.
- Security workflow with govulncheck and dependency review.
- Documentation: architecture, CLI reference, attestation and policy
  specifications, threat model, compliance mapping, CI integrations, and
  self-hosting.

### Changed

- **Renamed to Seralith.** The CLI is now `seralith`, the control plane
  `seralith-control`, configuration lives in `.seralith/`, attestations in
  `refs/notes/seralith`, workspace branches use `seralith/<name>`, environment
  variables use the `SERALITH_` prefix, and predicate types live under
  `https://seralith.com/`. Repositories initialized with earlier builds must be
  re-initialized.
- New identity: "the locked stone" symbol (a stone split by a seam, hollowed
  into a lock chamber, and held by a single bolt) with a Roman inscriptional
  wordmark, generated as a complete asset set and applied across the
  workbench, the site, and the README.
- Read-only commands no longer require a configured git identity, so the gate
  runs in CI without setup.

### Fixed

- **Security:** a project's `.seralith/config.yaml` can no longer define an
  agent profile with a built-in ID such as `shell` or `claude`; such profiles
  are ignored with a warning. Built-in profiles win when guessing the tool for
  a command, and the tool version probe no longer runs relative paths.
- **Security:** `seralith ledger verify` reports a ledger as `unanchored`, not
  `intact`, when it has no checkpoints or a checkpoint is signed by a key outside
  the trust store, and lists those keys; `--strict` fails in that case.
  Evidence packages now verify ledger checkpoints against the trust store.
- **Security:** `seralith gate` no longer skips a merge commit whose tree
  differs from the clean merge of its parents. Such a merge previously passed
  with any content and no attestation.
- **Security:** an approval now counts only for the commit it approves, or for
  the range tip when it approves the tip. Copying an approval from another
  commit into a note no longer satisfies policy.
- **Security:** policy is checked against the diff git computes, not only the
  files a change attestation declares. A change that leaves a path out of its
  segments fails, and understated line counts no longer avoid thresholds.
- **Security:** Seralith Control marks a change attestation verified only when
  its committer signed it, and an approval only when its approver signed it with
  a human key holding the stated role. Commits leave "awaiting approval" only
  when independent approvals satisfy each pending rule. A verified upload now
  replaces an earlier unverified one of the same statement, so uploading it first
  with missing or wrong signatures no longer hides the genuine envelope.
- `seralith run` could exit before recording `session.ended` in the ledger,
  because a session signalled completion before writing that entry.
- Email addresses on seralith.com render as written instead of being replaced by
  Cloudflare's obfuscation placeholder.

## [0.1.0] - 2026-11-24

### Added

- Parallel workspaces on git worktrees with bootstrap of untracked files and
  setup commands.
- PTY sessions for any terminal coding tool, with transcripts and declared
  model capture.
- Observed, line-level attribution and `seralith blame`.
- Signed in-toto attestations (`change/v1`, `approval/v1`) in DSSE envelopes,
  stored in `refs/notes/seralith`.
- Declarative policy engine with secure defaults.
- `seralith gate` for CI with independent re-verification of signatures, tree
  integrity, segment chains, policy, and role-bound independent approvals.
- Hash-chained audit ledger with signed checkpoints.
- Signed evidence packages mapped to DoWI 8430.01, NIST SP 800-218,
  NIST SP 800-171, and the EU Cyber Resilience Act.
- `seralith serve`: loopback daemon and embedded web workbench with live
  terminals, diffs, reviews, ledger, and policy views.
- Seralith Control (commercial): organization key registry, policy distribution,
  attestation archive with server-side verification, analytics, and a
  hash-chained audit trail; `seralith cloud` client.

[Unreleased]: https://github.com/seralith-hq/seralith/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/seralith-hq/seralith/releases/tag/v0.1.0
