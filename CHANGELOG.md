# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `sigil gate` and `sigil verify` accept `--config-from <rev>` to judge a branch
  by the policy and trust store of its base.
- Release automation with GoReleaser, SBOMs, and build provenance; installer
  script with checksum verification; reusable GitHub Action.
- Container images for the CLI and Sigil Control; Docker Compose and Kubernetes
  manifests.
- Security workflow with govulncheck and dependency review.
- Documentation: architecture, CLI reference, attestation and policy
  specifications, threat model, compliance mapping, CI integrations, and
  self-hosting.

### Changed

- Read-only commands no longer require a configured git identity, so the gate
  runs in CI without setup.

## [0.1.0] - 2026-11-24

### Added

- Parallel workspaces on git worktrees with bootstrap of untracked files and
  setup commands.
- PTY sessions for any terminal coding tool, with transcripts and declared
  model capture.
- Observed, line-level attribution and `sigil blame`.
- Signed in-toto attestations (`change/v1`, `approval/v1`) in DSSE envelopes,
  stored in `refs/notes/sigil`.
- Declarative policy engine with secure defaults.
- `sigil gate` for CI with independent re-verification of signatures, tree
  integrity, segment chains, policy, and role-bound independent approvals.
- Hash-chained audit ledger with signed checkpoints.
- Signed evidence packages mapped to DoWI 8430.01, NIST SP 800-218,
  NIST SP 800-171, and the EU Cyber Resilience Act.
- `sigil serve`: loopback daemon and embedded web workbench with live
  terminals, diffs, reviews, ledger, and policy views.
- Sigil Control (commercial): organization key registry, policy distribution,
  attestation archive with server-side verification, analytics, and a
  hash-chained audit trail; `sigil cloud` client.

[Unreleased]: https://github.com/ryan-tobin/new-startup/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/ryan-tobin/new-startup/releases/tag/v0.1.0
