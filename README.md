<p align="center">
  <img src="brand/sigil-logo.svg" alt="Sigil" width="280">
</p>

<h3 align="center">Build in parallel. Ship with proof.</h3>

<p align="center">
  The development workbench for teams that let humans and coding agents work side by side,
  and need to prove exactly who changed what.
</p>

<p align="center">
  <a href="https://github.com/ryan-tobin/new-startup/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/ryan-tobin/new-startup/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: Apache-2.0" src="https://img.shields.io/badge/license-Apache--2.0-0E1116"></a>
  <img alt="Go 1.24+" src="https://img.shields.io/badge/go-1.24%2B-FF5A1F">
</p>

---

## Why Sigil

Developers now run several coding agents at once, each on its own branch, using
whatever tool and model they prefer. That makes teams fast. It also leaves an
uncomfortable gap: **no one can say with certainty which lines were written by a
person, which were written by an agent, and who reviewed the risky ones.**

Regulators have noticed. The U.S. Department of War's *Accelerated Mission
Software* instruction (DoWI 8430.01, effective September 2026) treats
AI-generated code as unverified input and calls for verifiable build integrity,
provenance, and pedigree. The EU Cyber Resilience Act's reporting obligations
took effect the same month. Secure software attestations under NIST SP 800-218
are already a condition of selling to the federal government.

Sigil closes the gap at the only moment it can be closed: **when the code is
written.**

## What it does

Sigil is a local-first workbench that runs your coding sessions, human or agent,
in isolated workspaces and records cryptographic evidence of every change.

| Capability | What you get |
| --- | --- |
| **Parallel workspaces** | One command creates an isolated git worktree per task. Run five agents and your own editor at the same time without collisions. |
| **Agent-agnostic sessions** | Launch any terminal tool: your editor, a shell, or any coding agent CLI. Sigil never sits between you and a model provider and never touches your tokens. |
| **Observed attribution** | Sigil snapshots the tree before and after every session, so attribution is measured, not self-reported. Edits between sessions are attributed to the human who owns the workspace. |
| **Signed provenance** | Every commit gets an [in-toto](https://in-toto.io) attestation in a DSSE envelope, signed with Ed25519 and stored in git notes. |
| **Policy gates** | Declarative rules such as "agent changes under `crypto/**` need one approval from `security`" or "agents may not edit CI workflows". Enforced locally and in CI. |
| **Tamper-evident ledger** | An append-only, hash-chained audit log of sessions, commits, approvals, and policy decisions. |
| **Evidence packages** | One command exports the evidence an assessor asks for, mapped to DoWI 8430.01, NIST SSDF, and the EU CRA. |
| **Workbench UI** | A fast web workbench for workspaces, live terminals, diffs, attribution, approvals, and the ledger. |

## Quick start

```sh
# Install (Linux and macOS)
curl -fsSL https://raw.githubusercontent.com/ryan-tobin/new-startup/main/scripts/install.sh | sh

# Or build from source
make bootstrap release-build

# Initialize Sigil in a repository
cd your-repo
sigil init
git add .sigil && git commit -m "chore: enable sigil"

# Create isolated workspaces and run agents in parallel
sigil ws new auth-refactor
sigil ws new docs-refresh
sigil run --ws auth-refactor --agent codex
sigil run --ws docs-refresh --agent gemini --model pro

# Or just use your own editor or shell; your edits are attributed to you
sigil run --ws auth-refactor -- $SHELL

# Review, then commit with a signed attestation
sigil status --ws auth-refactor
sigil commit --ws auth-refactor -m "refactor(auth): extract token service"
sigil sync push

# A security reviewer approves; CI gates the branch
sigil approve sigil/auth-refactor --role security
sigil gate --base origin/main --config-from origin/main

# Who wrote this file?
sigil blame internal/auth/token.go

# Open the workbench
sigil serve --open
```

See [Getting started](docs/getting-started.md) for a full walkthrough.

### Gate pull requests on GitHub

```yaml
- uses: actions/checkout@v7
  with: { fetch-depth: 0 }
- uses: ryan-tobin/new-startup@v0
```

## Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│ Developer machine                                                    │
│                                                                      │
│  sigil CLI ──┐                 ┌──────────── Workbench UI (React) ──┐ │
│              ▼                 ▼                                     │ │
│        ┌──────────────── sigil daemon (Go) ────────────────┐        │ │
│        │ workspaces │ sessions (PTY) │ snapshots │ policy  │◀───────┘ │
│        │ attest (in-toto + DSSE) │ ledger (hash chain)     │          │
│        └───────────────┬───────────────────────────────────┘          │
│                        ▼                                              │
│           git worktrees + refs/notes/sigil                            │
└────────────────────────┬─────────────────────────────────────────────┘
                         │ push
                         ▼
     CI: `sigil gate`  ─────────▶  Sigil Control (ee): orgs, trusted keys,
                                    policy bundles, attestation archive,
                                    evidence packages
```

See [docs/architecture.md](docs/architecture.md) for the full design.

## Repository layout

| Path | Contents |
| --- | --- |
| `cmd/sigil` | The `sigil` CLI and daemon entry point |
| `internal/` | Core packages: attestation, ledger, policy, workspaces, sessions, API |
| `pkg/` | Public Go API for integrators |
| `web/` | Workbench UI (React, TypeScript, Vite) |
| `ee/` | Sigil Control, the commercial control plane |
| `site/` | Marketing site |
| `deploy/` | Container images, Compose, and Kubernetes manifests |
| `scripts/` | Installer and maintenance scripts |
| `docs/` | Product, specification, and company documentation |
| `brand/` | Logo and brand guidelines |

## Editions

| | Community | Team | Enterprise | Sovereign |
| --- | --- | --- | --- | --- |
| Workbench, CLI, local attestations, gates | Yes | Yes | Yes | Yes |
| Hosted control plane, trusted key registry | | Yes | Yes | |
| SSO/SCIM, role-based approvals, retention | | | Yes | Yes |
| Air-gapped, on-premises, FIPS builds | | | | Yes |
| Price | Free | $29 / dev / month | $79 / dev / month | Contact sales |

## Documentation

- [Getting started](docs/getting-started.md)
- [Architecture](docs/architecture.md)
- [CLI reference](docs/cli.md)
- [Attestation specification](docs/spec/attestation.md)
- [Policy reference](docs/spec/policy.md)
- [Threat model](docs/security/threat-model.md)
- [Compliance mapping](docs/compliance/mapping.md)
- [GitHub Actions](docs/integrations/github-actions.md) · [GitLab CI](docs/integrations/gitlab-ci.md)
- [Self-hosting Sigil Control](docs/operations/self-hosting.md)

## Contributing

We welcome contributions. Read [CONTRIBUTING.md](CONTRIBUTING.md) and our
[Code of Conduct](CODE_OF_CONDUCT.md). Report vulnerabilities through
[SECURITY.md](SECURITY.md).

## License

The Sigil workbench, CLI, and libraries are licensed under the
[Apache License 2.0](LICENSE). Code in [`ee/`](ee/) is licensed under the Sigil
Commercial License.

Copyright 2026 Sigil Systems, Inc.
