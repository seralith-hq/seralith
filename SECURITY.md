# Security Policy

Seralith is security infrastructure. We treat every report with urgency.

## Supported versions

| Version | Supported |
| --- | --- |
| 0.x (latest minor) | Yes |
| Older 0.x minors | Critical fixes only, for 90 days after the next minor |

## Reporting a vulnerability

Do **not** open a public issue. Instead, email **security@seralith.com** or use
GitHub's private vulnerability reporting on this repository.

Please include:

- Affected component (`seralith` CLI, daemon, workbench, control plane) and version
- Reproduction steps or proof of concept
- Impact assessment, especially any way to forge, strip, or replay attestations
  or to bypass a policy gate

## Our commitments

| Stage | Target |
| --- | --- |
| Acknowledgement | 1 business day |
| Triage and severity | 3 business days |
| Fix for critical issues | 14 days |
| Coordinated disclosure | 90 days or on fix release, whichever is first |

We credit reporters in release notes unless they prefer to stay anonymous. We
will not pursue legal action against good-faith research that follows this
policy.

## Scope highlights

In scope: signature verification, ledger integrity, policy evaluation, gate
bypasses, privilege escalation in the daemon, authentication in the control
plane, and supply-chain integrity of our release artifacts.

Out of scope: denial of service through resource exhaustion on a machine the
attacker already controls, and issues in third-party coding agents that Seralith
launches but does not ship.
