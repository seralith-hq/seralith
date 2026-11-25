# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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
