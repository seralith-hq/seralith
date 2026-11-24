# Compliance mapping

Sigil produces evidence. Assessors decide compliance. This mapping shows which
artifacts support which requirements. Every evidence package includes it as
`controls.json`.

## DoW Instruction 8430.01, *Accelerated Mission Software* (effective September 2026)

| Requirement area | Sigil evidence |
| --- | --- |
| AI-generated code is unverified input; developers remain accountable | Agent segments record the accountable operator; every commit is signed by its human committer |
| Human review and approval of AI changes to security- or safety-critical functionality | `require-approval` rules bound to paths and roles; signed `approval/v1` attestations; gate enforcement |
| Records of models, versions, and tools used to produce software | `ai-usage.json` in evidence packages; tool, version, declared model, and transcript digest on every segment |
| Verifiable build integrity, provenance, and pedigree | in-toto attestations bound to commits, verifiable offline with the trust store |
| Approved tools only | `toolsNotIn` allowlist rules that deny unapproved agents |

## NIST SP 800-218, Secure Software Development Framework

| Practice | Sigil evidence |
| --- | --- |
| PO.3.2 Secure toolchains | Tool allowlists, isolated workspaces |
| PS.1.1 Protect code from unauthorized changes | Deny rules on pipelines and secrets; gate on protected branches |
| PS.2.1 Integrity verification information | DSSE-signed attestations and published trust store |
| PS.3.1 Archive release provenance | Signed evidence packages per release range |
| PW.7.1 Decide which code requires review | Policy rules |
| PW.7.2 Perform and record review | Signed approvals bound to commits |

## NIST SP 800-171 (CMMC Level 2)

| Requirement | Sigil evidence |
| --- | --- |
| 3.3.1 Create and retain audit records | Hash-chained ledger and control-plane audit trail |
| 3.3.8 Protect audit information | Hash chaining and signed checkpoints |
| 3.4.5 Access restrictions for change | Role-bound approvals and protected-path rules |

## EU Cyber Resilience Act

| Obligation | Sigil evidence |
| --- | --- |
| Art. 14 reporting of actively exploited vulnerabilities (from 11 September 2026) | `sigil blame` identifies the change, actor, tool, and model behind any vulnerable line within the 24-hour early-warning window |
| Annex I Part II vulnerability handling and documentation | Attribution, dependency-manifest rules, and evidence packages per release |

## Producing evidence

```sh
sigil evidence export --range v2.3.0..v2.4.0 -o evidence-v2.4.0.zip
sigil evidence verify evidence-v2.4.0.zip --trust .sigil/trust.yaml
```

| File | Contents |
| --- | --- |
| `manifest.json`, `manifest.sig.json` | SHA-256 of every file; DSSE signature |
| `report.md` | Human-readable summary and control table |
| `gate-report.json` | Per-commit verification results |
| `attestations/*.jsonl` | Every envelope in the range |
| `ai-usage.json` | Tools, versions, declared models, operators |
| `policy.yaml`, `trust.yaml` | Policy and keys in effect |
| `ledger.jsonl`, `ledger-verify.json` | Audit ledger and its verification |
| `controls.json` | This mapping |
