# Seralith Documentation

## Product

| Guide | Audience |
| --- | --- |
| [Getting started](getting-started.md) | Developers trying Seralith for the first time |
| [Architecture](architecture.md) | Engineers evaluating or contributing to Seralith |
| [CLI reference](cli.md) | Everyone |
| [GitHub Actions](integrations/github-actions.md) and [GitLab CI](integrations/gitlab-ci.md) | Platform teams |
| [Self-hosting Seralith Control](operations/self-hosting.md) | Operators of Enterprise and Sovereign deployments |

## Features

| Guide | Contents |
| --- | --- |
| [Authorship tracking](features/authorship-tracking.md) | How Seralith observes which lines a person or an agent wrote, and the evidence it records |
| [Policy gates](features/policy-gates.md) | How rules match a change, how the gate evaluates them again, and how approvals are counted |

## Specifications

| Document | Contents |
| --- | --- |
| [Attestation format](spec/attestation.md) | `change/v1` and `approval/v1` predicates, DSSE, storage, and the verification algorithm |
| [Policy language](spec/policy.md) | Rule schema, matching semantics, effects, and approvals |
| [Trust store](spec/trust-store.md) | Public keys, roles, the human kind, and revocation |
| [Audit ledger](spec/ledger.md) | Hash chain, checkpoints, and anchored versus unanchored |
| [Evidence packages](spec/evidence.md) | Manifest bytes, the manifest signature, and what verification checks |

## Security and compliance

| Document | Contents |
| --- | --- |
| [Threat model](security/threat-model.md) | Assets, trust boundaries, threats, mitigations, and known limitations |
| [Compliance mapping](compliance/mapping.md) | How Seralith evidence maps to DoWI 8430.01, NIST SP 800-218, NIST SP 800-171, and the EU CRA |

## Process

- [RFC template](rfcs/0000-template.md)
