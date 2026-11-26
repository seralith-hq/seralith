# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Terminal demos of authorship tracking and `seralith gate`, recorded from the
  `seralith` binary with VHS tapes in `docs/assets/demo/`. Re-render them with
  `make demo`. `tools/sitedocs` copies `docs/assets/` into the published site
  so a docs page can embed one.

- seralith.com/blog: posts rendered from `blog/` by the same `tools/sitedocs`
  command that builds `/docs`, with an index at `/blog`. The first post,
  `/blog/three-gate-bypasses`, describes three `seralith gate` bypasses that
  were fixed before v0.1.0.

- Documentation: a feature guide to authorship tracking, published at
  seralith.com/docs/features/authorship-tracking, with command output captured
  from a real run.

- seralith.com/docs: the repository's documentation, rendered from `docs/` on
  every site deploy with a sidebar, per-page links to edit on GitHub, and a docs
  sitemap. `make site-docs` builds it locally.

- seralith.com and the README link to the GitHub repository and @seralithhq on X.

### Changed

- The root README: the authorship recording sits under the introduction, the
  architecture diagram lines up after the Seralith rename, and the quick start
  matches the CLI. `seralith blame` on a workspace file needs `--rev`, and
  `seralith gate --base` does not see a `seralith/<name>` branch while the
  primary checkout is still on the base.

- Commits that change only `blog/` build and deploy seralith.com.

### Fixed

- The `seralith status` and `seralith blame` examples in the getting-started
  guide now match the CLI's real output: the workspace header line, column
  spacing, consistent line counts, and percentages rounded down.

## [0.1.0] - 2026-11-25

First public release.

### Added

- Parallel workspaces on git worktrees with bootstrap of untracked files and
  setup commands.
- PTY sessions for any terminal coding tool, with transcripts and declared
  model capture. Seralith never proxies model traffic or handles coding-tool
  credentials.
- Observed, line-level attribution and `seralith blame`.
- Signed in-toto attestations (`change/v1`, `approval/v1`) in DSSE envelopes
  with Ed25519 keys, stored in `refs/notes/seralith`.
- Declarative policy engine with secure defaults, including
  `humanLabels: unverified`: agent rules that protect paths also apply to
  human-labeled changes, because the actor label is declared by the
  committer's own tooling.
- `seralith gate` for CI, which independently re-verifies signatures, tree
  integrity, segment chains, the declared files against git's own diff,
  policy, and role-bound independent approvals. `--config-from <rev>` judges a
  branch by the policy and trust store of its base. A merge commit is skipped
  only when it is exactly the clean merge of its parents.
- Reusable GitHub Action and GitLab CI integration.
- Hash-chained audit ledger with signed checkpoints; `seralith ledger verify`
  reports a ledger without trusted checkpoints as unanchored.
- Signed evidence packages mapped to DoWI 8430.01, NIST SP 800-218,
  NIST SP 800-171, and the EU Cyber Resilience Act.
- `seralith serve`: loopback daemon and embedded web workbench with live
  terminals, diffs, reviews, ledger, and policy views.
- Seralith Control (commercial): organization key registry, policy
  distribution, attestation archive verified with the gate's identity rules,
  analytics, and a hash-chained audit trail; `seralith cloud` client.
- Release automation with GoReleaser, SBOMs, and build provenance; installer
  with checksum verification; container images, Docker Compose, and
  Kubernetes manifests.
- Documentation: getting started, architecture, CLI reference, attestation
  and policy specifications, threat model, compliance mapping, CI
  integrations, and self-hosting.

[Unreleased]: https://github.com/seralith-hq/seralith/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/seralith-hq/seralith/releases/tag/v0.1.0
