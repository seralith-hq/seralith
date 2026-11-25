# Threat model

This document states what Seralith guarantees, against whom, and where those
guarantees end. We would rather a customer read about a limitation here than
discover it in an assessment.

## Assets

1. **Attribution integrity**: the record of which actor produced which lines.
2. **Approval integrity**: proof that required humans reviewed specific changes.
3. **Policy enforcement**: protected changes cannot merge without satisfying policy.
4. **Audit integrity**: the ledger and control-plane audit trail cannot be silently altered.
5. **Developer credentials**: signing keys and coding-tool credentials.

## Actors

| Actor | Capability |
| --- | --- |
| Coding agent | Arbitrary code execution as the developer, inside a workspace, during a session |
| Malicious insider | Push access; can run any git command |
| External attacker | Network access to developer machines, CI, or Seralith Control |
| Compromised CI runner | Can read CI secrets and alter build output |

## Trust boundaries

```
 [agent process] ──writes──▶ [worktree] ◀──snapshots── [seralith] ──signs──▶ [notes]
                                                         │
 [developer key] ────────────────────────────────────────┘
 [notes + commits] ──push──▶ [remote] ──fetch──▶ [gate in CI] ◀── [policy + trust from main]
```

The gate is the enforcement point. Everything upstream of it is untrusted until
verified.

## Threats and mitigations

| # | Threat | Mitigation |
| --- | --- | --- |
| T1 | Agent under-reports its changes | Attribution is observed by snapshotting the tree; agents are never asked |
| T2 | Agent changes are passed off as human edits by committing outside Seralith | The gate requires a change attestation for every commit except a merge whose tree is exactly the clean merge of its parents |
| T3 | Attestation copied from a benign commit to a malicious one | Subject, tree, and parents must match the commit |
| T4 | Extra changes slipped into a commit after observation, or hidden from policy | Segments must chain to the commit tree exactly, and every path git reports between the first parent and the commit must be declared by a segment |
| T5 | Forged attestation or approval | Ed25519 signatures verified against a trust store committed on the protected branch or served by Seralith Control |
| T6 | Self-approval of a protected change | Approvers must differ from the committer unless a rule explicitly allows it |
| T7 | Approval by someone without authority | The signing key must be a human key holding the required role |
| T8 | Relaxed policy in a feature branch | Run the gate with the policy and trust store from the **base** branch, not the branch under test (see the CI guides) |
| T9 | Replayed approval from another commit | Approvals are bound to a commit subject; only approvals of the commit itself, or of the range tip, count |
| T10 | Ledger tampering | Hash chain detects edits, deletions, and reordering; signed checkpoints prevent full rewrites without the key |
| T11 | Browser-based attack on the local daemon | Loopback-only listener, per-run token, Host and Origin validation, strict CSP |
| T12 | Credential theft via Seralith | Seralith never proxies model traffic or stores coding-tool credentials; control-plane tokens are stored hashed and sent only over TLS |
| T13 | Forged upload to Seralith Control | Server-side verification against the registry with the gate's identity rules (committer signs changes; approvers sign approvals with a human key holding the role); untrusted envelopes are flagged and replaced when the genuine envelope arrives |

## Known limitations

- **Same-user processes.** An agent runs as the developer and could, in
  principle, read the developer's signing key or edit local Seralith state.
  Signing keys should live in a hardware token or OS keychain for high-assurance
  deployments (on the roadmap), and the Sovereign edition runs agents in
  separate OS users or containers. The gate's server-side checks (T2 to T9)
  hold regardless.
- **Concurrent human edits during an agent session.** Changes made in the same
  worktree while a session runs are attributed to the session's actor. Use a
  separate workspace to work in parallel.
- **Declared models.** The model field records what the operator declared.
  Seralith does not see model traffic, by design, and cannot verify it.
- **Merge commits.** The gate skips a merge commit by default only when its
  tree is exactly what merging its parents produces. A merge with conflict
  resolutions or other content of its own fails unless it is attested; rebase
  instead, or use `--include-merges` to require attestations on every merge.
- **Declared actors.** Which actor made each segment is recorded by the
  committer's own Seralith, so a holder of a trusted key can label agent work
  as human. With the default `humanLabels: unverified`, agent rules that
  protect paths also apply to human-labeled changes to those paths, so a
  protected change needs an independent approval whatever its label. Agent
  rules without paths, such as line-count thresholds, still rely on the
  label, as does every rule under `humanLabels: trusted`. Hardware-backed
  proof of human presence is on the roadmap.
- **Key compromise.** A stolen developer key can sign false attestations for
  that developer until it is revoked. Revocation takes effect at the next gate
  run that uses the updated trust store.
