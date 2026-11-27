# RFC 0001: Independent verifier

| Field | Value |
| --- | --- |
| Author | Seralith engineering |
| Status | Draft |
| Created | 2026-11-27 |

## Summary

This note proposes a small, offline program that checks Seralith attestations,
approvals, policy decisions, ledger checkpoints, and evidence packages by
implementing the published rules itself. It does not link Seralith's packages,
and it does not replace the CI gate. The program is worth building only if a
reviewer can read its source and obtain the same pass or fail result as
`seralith gate`, `seralith evidence verify`, and `seralith ledger verify`
without trusting those commands' code. This document is a design note. It does
not change the gate, the formats, or the threat model.

## Motivation

Design partners want a check they can run on the evidence Seralith produces
when the checker is not Seralith. The people asking are auditors and the
compliance and security leads at those partners, including buyers who map their
controls to DoW Instruction 8430.01, NIST SP 800-218, and the EU Cyber
Resilience Act. The mapping in `docs/compliance/mapping.md` already tells an
assessor to verify an evidence package offline with the trust store. That
command runs Seralith's own verification code. A buyer who requires
verification independent of the vendor cannot use it as the only check.

The same program is a spec review. Where this note marks a check "spec gap:
only defined by code", an implementer cannot get a matching result from
`docs/spec/attestation.md`, `docs/spec/policy.md`, and the threat model alone.
Those gaps should be closed in the specs before the program is described as an
implementation of the spec. Closing them is a later change. This note only
records them.

v1.0.0 does not depend on this program. The gate in CI remains the enforcement
point (`docs/architecture.md`, `docs/security/threat-model.md`).

## Design

### Placement and language

Recommendation: a separate repository, written in Rust, producing one static
binary. The working command name is `seralith-verify`. The binary does not
depend on the Go module `github.com/seralith-hq/seralith`. It does not import
`internal/` or `pkg/`.

A directory or module inside this repository can be barred from those imports
by a test, and it would still share this repository's review, release pipeline,
and, if it were Go, `crypto/ed25519` and `encoding/json`. A defect in those
packages would then sit on both sides of the comparison. The auditor's reason
for a second program is to avoid that common mode. Rust is a different
language, with a different JSON parser, a different YAML parser, and a
different Ed25519 implementation. One static binary and a short lockfile are
easier to review than an interpreter plus a virtualenv. Python is the
reasonable alternative if more reviewers will read it; the cost is packaging
an Ed25519 library and a YAML library, and keeping that environment as small
as the Rust crate list.

Dependencies stay on the verification path: SHA-256, Ed25519, standard base64,
JSON, YAML, and zip. The crate list has no HTTP client. The program makes no
network call, including no update check and no fetch of `refs/notes/seralith`.
If the notes are not already in the repository, the gate scope fails closed.

Tree hashes, parent lists, merge results, and path diffs come from the local
`git` binary, with `--no-ext-diff`, matching `internal/gitx/diff.go` and
`internal/verify/verify.go` (`mergeIsDerived`). The attestation spec defines
the path check as the diff from the parent tree to the commit tree, and names
`git merge-tree --write-tree` for the merge-skip rule
(`docs/spec/attestation.md`). Reimplementing diff would create disagreements
that are not Seralith bugs. The program does not run `fetch` or `push`.
Object ids, parents, diffs, and merge-tree come from local git. A missing
git, or a git error other than the merge-tree case specified below, is an
input failure (exit 2).

The shared test corpus stays in this repository and is produced by the
`seralith` binary. The verifier repository vendors that corpus and runs it.
The two programs do not share a generator.

### Checklist

"From spec" means a published document states the pass/fail rule:
`docs/spec/attestation.md`, `docs/spec/policy.md`, `docs/security/threat-model.md`,
or `docs/compliance/mapping.md`. "Spec gap: only defined by code" means the
pass/fail rule an independent program must match is in the code cited on that
row. Feature guides are not the spec. Where a guide describes the same
behavior, the row says so, and the label stays a spec gap.

The gate checks live in `internal/verify/verify.go` (`Commit`, `Gate`). The
package checks live in `internal/evidence/evidence.go` (`Verify`). The ledger
checks live in `internal/ledger/ledger.go` (`Verify`). Evidence export calls
the ledger check and stores the report; `evidence.Verify` does not run it
again.

| # | Check | Where it is done today | Label |
| --- | --- | --- | --- |
| 1 | DSSE envelope: payload type `application/vnd.in-toto+json`, payload is standard base64, signature over the PAE `DSSEv1 <len(type)> <type> <len(payload)> <payload>`, key id `ed25519:` plus the first 16 bytes of SHA-256 of the raw public key. At least one signature must verify under the trust store. | `pkg/dsse/dsse.go` (`PAE`, `Verify`), `internal/keys/keys.go` (`ID`), `docs/spec/attestation.md` | from spec |
| 2 | A repeated DSSE `keyid` does not count twice. The first signature for that id is the one that is verified. | `pkg/dsse/dsse.go` (`Verify`, the `seen` map) | spec gap: only defined by code |
| 3 | Select a `change/v1` statement whose subject is the commit and whose envelope meets check 1, where at least one accepted key's trust-store principal equals `predicate.committer.id`. | `docs/spec/attestation.md` step 1; `internal/verify/verify.go` (`pickChange`) | from spec |
| 4 | Selection order and subject fields. The note is walked in file order. The first envelope that passes check 3 is the one that is checked further. A later envelope is not considered. The subject is the first subject's `digest.gitCommit` only. `subject[0].name` and any later subject are ignored. The committer key's `kind` is not required to be `human`. | `internal/verify/verify.go` (`pickChange`); `internal/notes/notes.go` (`Read`, `Statements`); `pkg/provenance/provenance.go` (`Commit`) | spec gap: only defined by code |
| 5 | `predicate.commit` equals the commit, `predicate.tree` equals the commit's tree, and `predicate.parents` equals the commit's parents in order. | `docs/spec/attestation.md` (field table and step 2); `internal/verify/verify.go` (`checkIntegrity`) | from spec |
| 6 | Segments chain from the first parent's tree, or the empty tree for a root commit, to the commit tree: `segments[0].baseTree` is that start, each next `baseTree` equals the previous `resultTree`, and the last `resultTree` equals the commit tree. | `docs/spec/attestation.md` step 3; `internal/verify/verify.go` (`checkIntegrity`) | from spec |
| 7 | Empty segment list and the empty-tree id. If `segments` is empty and the commit tree equals that start tree, check 6 passes. The start tree for a root is the object id `4b825dc642cb6eb9a060e54bf8d69288fbee4904`. | `internal/verify/verify.go` (`checkIntegrity`); `internal/gitx/git.go` (`EmptyTree`) | spec gap: only defined by code |
| 8 | Every path git reports between the start tree and the commit tree, including the old path of a rename, appears in some segment's `files`. Lines a path adds beyond the sum of `linesAdded` declared for that path are attributed to every actor that declared the path, and those extra lines are part of policy input. Paths a segment declares and git does not report are still policy input. Over-declared line counts are not a failure. | `docs/spec/attestation.md` steps 4 and 5; `docs/spec/policy.md` (a rename is a change to both paths); `internal/verify/verify.go` (`observedChanges`); `internal/policy/policy.go` (`ChangesFromSegments`) | from spec |
| 9 | How that diff is computed. `git diff --no-ext-diff` with rename detection (`-M`), NUL name-status, numstat, and a zero-context patch. A copy (`C`) is treated as a rename. A binary file (numstat `-`) adds zero lines. The rename threshold is git's default. | `internal/gitx/diff.go` (`DiffTrees`, `parseNameStatus`, `parseNumstat`) | spec gap: only defined by code |
| 10 | Policy is the caller's policy file, re-evaluated. The `policy` object inside the attestation is not compared. Any `deny` fails. `require-approval` fails until enough approvers match. `warn` passes and is reported. `allow` passes. `humanLabels: unverified` is the default: it extends an agent path rule to human-labeled changes on those paths, and turns a `deny` that matches only those human-labeled paths into one approval. `humanLabels: trusted` does not extend those rules. Rules with no `paths` are not extended. | `docs/spec/policy.md`; `docs/spec/attestation.md` step 5; `internal/policy/policy.go` (`Evaluate`, `extendsToHuman`); `internal/verify/verify.go` (`Commit`) | from spec |
| 11 | Policy details the spec does not state. An omitted `humanLabels` is treated as `unverified`. Globs use `doublestar.Match` from `github.com/bmatcuk/doublestar/v4` v4.10.7 (`*` does not match `/`). Unknown YAML fields are rejected. A rule id matches `^[a-z0-9][a-z0-9._-]{0,63}$`. `version` other than 1 is rejected. Human-extended matches count toward `files` and `linesAdded`. If a `deny` rule matches an agent path and a human-labeled path, the decision stays `deny` and lists the agent paths only. The human-only relaxation sets `approvals` to 1, keeps the rule's roles, and uses the message in `unverifiedMessage`. `allowSelfApproval` on that rule still applies. The gate enforces every matching rule. `policy.Strongest` does not drop a weaker rule; commit time uses it (`internal/attest/attest.go`), and the gate does not. | `internal/policy/policy.go` (`Parse`, `Validate`, `Evaluate`); `go.mod`; the mixed `deny` case and the omitted field are also described in `docs/features/policy-gates.md` | spec gap: only defined by code |
| 12 | Approvals. For each `require-approval` decision with threshold *n* and roles *R*, count distinct approver principals over `approval/v1` statements that approve this commit and sit on it, or that approve the range tip and sit on the tip. The approval's `commit` equals its subject and that target. The envelope meets check 1 with a human key whose principal equals `approver.id` and, when the approval names a role, holds that role. The role is in *R* when *R* is non-empty. `rules` is empty or contains the rule id. The approver differs from the committer unless the rule sets `allowSelfApproval`. A qualifying `reject` fails the commit. Fewer than *n* approvers fails. | `docs/spec/attestation.md` step 6; `docs/spec/policy.md`; `internal/verify/verify.go` (`collectApprovals`, `satisfy`, `Gate`) | from spec |
| 13 | A `decision` other than `reject` counts as an approval, including a value other than `approve`. A malformed approval predicate is skipped, so it neither approves nor rejects. | `internal/verify/verify.go` (`collectApprovals`, `satisfy`) | spec gap: only defined by code |
| 14 | Merge commits. When merges are skipped, a merge is skipped only if its tree equals one parent's tree, or it has exactly two parents and its tree equals the conflict-free result of `git merge-tree --write-tree` on those parents. Any other merge needs its own change attestation. | `docs/spec/attestation.md`; `internal/verify/verify.go` (`mergeIsDerived`, `Commit`) | from spec |
| 15 | Merge-skip invocation and which command skips. The git arguments are `merge-tree --write-tree --no-messages`. A non-zero exit means "not a clean merge" and is not an error. More than two parents are clean only when the tree equals one parent. `seralith gate` skips unless `--include-merges`. `seralith verify` does not skip. `seralith evidence export` always skips. | `internal/verify/verify.go` (`mergeIsDerived`); `internal/cli/verify.go`; `internal/cli/evidence.go` | spec gap: only defined by code |
| 16 | Policy and trust for a CI run come from the base revision, not from the branch under test. | `docs/security/threat-model.md` T8; `docs/spec/policy.md` (file location `.seralith/policy.yaml`); `docs/integrations/github-actions.md` | from spec |
| 17 | How those files are loaded. `--config-from REV` reads `.seralith/policy.yaml` and `.seralith/trust.yaml` as blobs at `REV` and fails if either blob is missing. With no `--config-from`, a missing policy file is an empty version-1 policy and a missing trust file is an empty store. | `internal/cli/env.go` (`useConfigFrom`); `internal/policy/policy.go` (`Load`); `internal/keys/trust.go` (`LoadTrustStore`) | spec gap: only defined by code |
| 18 | Trust store and revocation. The store is YAML, `version: 1` only. Each key has `id`, `principal`, `kind` (`human` or `machine`), `roles`, a PKIX `PUBLIC KEY` PEM, and optional `revoked`. A declared id that differs from check 1's key id fails the parse. `Resolve` skips revoked keys. Revocation applies when the store used for that run is the updated one. The threat model states the timing and does not state this file. | `internal/keys/trust.go` (`ParseTrustStore`, `Resolve`); `docs/security/threat-model.md` (key compromise) | spec gap: only defined by code |
| 19 | Ledger hash chain and checkpoints. JSON Lines. Each entry's hash is the SHA-256 of a canonical encoding of the entry with the hash cleared, and `prev` is the previous entry's hash. A checkpoint is a later entry whose signature covers an earlier head. Edits, gaps, reordering, and a bad signature from a trusted checkpoint key are failures. A chain with no trusted checkpoint is unanchored: intact, and not proof against a rewrite by someone who can edit the file. | `docs/architecture.md` (Ledger); `docs/security/threat-model.md` T10; `internal/ledger/ledger.go` (`ComputeHash`, `Verify`, `Anchored`) | spec gap: only defined by code |
| 20 | Evidence package. Zip members listed in `manifest.json` are present and their SHA-256 matches. `manifest.sig.json` is a DSSE signature over the exact bytes of `manifest.json`, payload type `application/vnd.seralith.evidence-manifest+json`, threshold one trusted key. `OK` requires a valid signature, no mismatch, and no missing listed file. | `docs/compliance/mapping.md` (member names and the verify command); `internal/evidence/evidence.go` (`Verify`, `Build`, `ManifestPayloadType`) | spec gap: only defined by code |

Check 19 is a gap because the pass/fail rules an implementer must match are the ones in `internal/ledger/ledger.go`, not the sentences in the architecture doc. The canonical preimage is Go's `encoding/json` of `Entry` after `Hash` is set to `""` (the field is present as an empty string; it has no `omitempty`), with `data` compacted when non-empty, empty `actor`, `subject`, and `data` omitted, and `time` formatted as `time.Time` JSON. The first entry has `seq` 1 and `prev` equal to 64 zero hex digits (`GenesisHash`). Each next `seq` is one greater. A checkpoint has type `ledger.checkpoint`. Its `data` is `seq`, `head`, `keyid`, and standard-base64 `sig`. The signed message is the raw bytes `seralith-ledger-checkpoint:v1:<seq>:<head>`, signed with Ed25519, not wrapped in DSSE. An unknown checkpoint key is listed and is not a chain failure. `OK` is "no issues". `Anchored` is `OK`, at least one checkpoint, and every checkpoint verified. A prefix of a ledger that still chains verifies, including a prefix that ends on a checkpoint. The check does not prove that some other copy of the log had more entries.

Check 20 is a gap because the compliance doc names the members and the command, and the decision procedure is the code. In particular, `evidence.Verify` does not re-run the gate, does not re-hash the ledger, and does not compare `ledger-verify.json` to a fresh ledger check. It does not reject zip members that are not in the manifest, and it does not read the manifest `size` field. `manifest.json` and `manifest.sig.json` are not in the hashed file list. The signature payload must be byte-for-byte equal to `manifest.json`, not a re-encoding of the same JSON. `signedBy` is key ids. Each member is read up to 512 MiB (`io.LimitReader`); a longer member is hashed as that prefix, so it mismatches a manifest hash of the full member rather than failing the open.

Lines the gate does not recompute, and so this program does not report as verified: `attribution`, `summary`, `predicate.policy`, `transcriptDigest`, `builder`, `repository`, and `createdAt`. The gate copies `summary` into its report for display (`internal/verify/verify.go`). Those fields stay attested claims.

Unreadable notes lines, a payload type other than the in-toto type, and a statement `_type` other than `https://in-toto.io/Statement/v1` are skipped (`internal/notes/notes.go`, `pkg/provenance/provenance.go` `Open`). If nothing remains that passes check 3, the commit fails with no change attestation. That skip is a spec gap, grouped with check 4.

### What the program does not prove

These limits are the ones in `docs/security/threat-model.md`, "Known limitations".
The program reports them in the human summary and in the JSON, on every run,
including a passing run. A pass does not narrow them.

- **Declared actors.** The actor on each segment is written by the committer's
  Seralith. A trusted key holder can label agent work as human. With
  `humanLabels: unverified`, an agent rule that names paths also applies to
  human-labeled edits of those paths. Agent rules without paths, and every
  rule when `humanLabels` is `trusted`, still follow the label. The program
  evaluates that policy. It does not prove the label.
- **Declared models.** `actor.model` is the model the operator declared.
  Seralith does not see model traffic. The program cannot prove which model
  ran.
- **Same-user processes.** An agent runs as the developer and can read a
  signing key that lives in that user's files, or edit local Seralith state,
  before anything is signed. The program sees the signature and the git
  objects after the fact. The gate checks T2 through T9 still apply to that
  signed result. They do not show whether the key was used by the person.
- **Key compromise before revocation.** A stolen key can sign attestations for
  its principal until the trust store used for the run marks it revoked.
  Revocation does not rewrite history that was verified under the old store.

The same section of the threat model records two further limits the program
inherits, because they are not visible in the git objects. Edits made in a
worktree while a session runs are attributed to the session's actor. A merge
is skipped only under check 14; a merge with conflict resolutions of its own
fails unless it carries a change attestation.

### Non-goals

- Replacing `seralith gate` in CI, or writing attestations, approvals, or
  ledger entries.
- Observing sessions, snapshotting worktrees, or launching coding tools.
- Talking to Seralith Control, a remote, or any other network address.
- Proving the line-level `attribution` intervals, the summary counts, the
  transcript bytes behind `transcriptDigest`, or the recorded policy result.
- Deciding the license. That is an open question below.
- Changing `docs/spec/`, the threat model, or the implementation in this
  pull request.

### Inputs and outputs

The program reads only local paths.

| Input | Required when | Source of the bytes |
| --- | --- | --- |
| Trust store | Always | `--trust FILE`. There is no default and no empty store substituted for a missing file. |
| Git repository | Gate scope | `--repo DIR`, already containing `refs/notes/seralith`. |
| Revision range | Gate scope | `--range SPEC`. The commit list is `git rev-list --reverse SPEC`, oldest first, as in `internal/gitx/git.go` (`RevList`). |
| Policy | Gate scope | `--policy FILE`, or the blob selected by `--config-from REV` (check 17). |
| Evidence package | Package scope | `--package FILE`, a zip. |
| Ledger file | Ledger scope | `--ledger FILE`, or `ledger.jsonl` inside the package when a package is given. The program does not open `<git-common-dir>/seralith/ledger.jsonl` unless that path is passed. The ledger is local state (`docs/architecture.md`); a clone does not prove it is the developer's log. |

`--config-from` is the mode for a branch review (check 16). `--strict` on the
ledger scope fails an unanchored chain. Without it, an unanchored chain matches
`seralith ledger verify`: the process can still exit 0, and the report says
`unanchored` (`internal/cli/ledger.go`).

Default merge handling for the gate scope is to skip clean merges, matching
`seralith gate` and the spec (check 15). `seralith verify` leaves `SkipMerges`
false. `--include-merges` requires a change attestation on every merge.

Scopes that have the inputs they need are run. A scope whose inputs were not
passed is `not_run`. The package scope matches check 20. It does not treat
`gate-report.json` or `ledger-verify.json` as evidence that those checks
succeeded. When the package contains `ledger.jsonl`, the ledger scope
recomputes check 19 with the trust store and ignores the packaged
`ledger-verify.json` as a conclusion. When both a repository and a package are
passed, the program also compares `attestations/<commit>.jsonl` in the package
with the note lines for that commit, and compares packaged `policy.yaml` and
`trust.yaml` with the policy and trust used for the gate scope. A difference
fails the run. Those comparisons are not checks in the current code. They are
part of this program so a package cannot tell a different story from the
repository it claims to represent.

The JSON report is the contract. It is deterministic: UTF-8, keys in the order
below, no timestamps added by the program, commits in `rev-list --reverse`
order, and a single trailing newline. `--json` writes it to stdout and writes
the human summary to stderr. Without `--json`, the human summary goes to
stdout and `--report PATH` writes the JSON. The human summary is the overall
status, one line per scope, the limits in "What the program does not prove",
and one line per commit that is not a pass.

```json
{
  "version": 1,
  "status": "pass",
  "scopes": {
    "gate": {
      "status": "pass",
      "range": "origin/main..HEAD",
      "commits": [
        {
          "commit": "<full hash>",
          "status": "pass",
          "attested": true,
          "signers": ["committer@example.com"],
          "reasons": [],
          "unrecognized": [],
          "decisions": [
            {
              "rule": "security-critical-review",
              "effect": "require-approval",
              "satisfied": true,
              "approvers": ["reviewer@example.com"]
            }
          ]
        }
      ]
    },
    "package": {
      "status": "pass",
      "signed": true,
      "signedBy": ["ed25519:"],
      "mismatch": [],
      "missing": []
    },
    "ledger": {
      "status": "unanchored",
      "entries": 0,
      "checkpoints": 0,
      "verifiedCheckpoints": 0,
      "untrustedCheckpointKeys": [],
      "issues": []
    }
  }
}
```

`status` for the run is `fail` if any executed scope is `fail`, otherwise
`warn` if any is `warn`, otherwise `pass`. `unanchored` is a ledger status, not
a run failure, unless `--strict` is set. `not_run` scopes are listed and do
not by themselves fail the run. An empty revision range is `pass` with zero
commits, matching `internal/verify/verify.go` (`Gate`). The commit count is in
the report so a caller can reject an empty range.

Running the ledger scope on a package can fail the run when `seralith evidence
verify` would exit 0. Export writes `ledger.jsonl` even when `Ledger.Verify`
reported issues (`internal/evidence/evidence.go`, `Build`), and `evidence.Verify`
only checks the manifest. The package scope still agrees with that command.
The ledger scope agrees with `seralith ledger verify` on the same file. The
run fails if either scope fails.

Gate `reasons` are codes, not the English sentences from the Go gate. The
corpus compares codes. The set is: `no_change_attestation`,
`subject_mismatch`, `signature_untrusted`, `committer_not_signer`,
`predicate_commit_mismatch`, `tree_mismatch`, `parents_mismatch`,
`segment_chain_break`, `undeclared_path`, `undeclared_rename`, `policy_deny`,
`approval_missing`, `policy_rejected`, `merge_not_clean`.
`merge_not_clean` is recorded only when no envelope passes check 3, because a
selected attestation clears earlier problems (`internal/verify/verify.go`,
`Commit`). A predicate type other than `change/v1` and `approval/v1` is listed
on the commit as `unrecognized` and is not a reason code. It does not change
pass or fail: a missing `change/v1` attestation is still
`no_change_attestation`, which is what the gate reports after ignoring the
unknown envelope. Approval rows use `valid` and a code: `subject_mismatch`,
`wrong_commit`, `signature_untrusted`, `principal_mismatch`, `not_human`,
`missing_role`.

Exit codes:

| Code | Meaning |
| --- | --- |
| 0 | Every executed scope is `pass`, `warn`, or, for the ledger without `--strict`, `unanchored`. |
| 1 | An executed scope is `fail`. |
| 2 | The arguments are wrong, a file or git object cannot be read, or the policy or trust store cannot be parsed. |

`seralith` uses exit 1 both for a failed check and for other errors
(`internal/cli/root.go`). Exit 2 is for this program, so a missing trust store
is not reported as a failed attestation. The corpus compares the JSON status,
not exit 2.

### Conformance corpus

This repository gains a corpus, generated by the `seralith` binary, that both
the gate and `seralith-verify` must agree on. Suggested path:
`conformance/v1/`. Each vector is a directory with the inputs (a git bundle or
a zip, a trust store, a policy, and the arguments) and `expect.json` in the
report shape above. The generator runs `seralith gate --format json`,
`seralith evidence verify --json`, and `seralith ledger verify --json` and
maps that output onto the reason codes. CI regenerates the corpus and fails if
the committed files differ. A pull request that changes a v1 result has to
change `expect.json` in the same pull request, where the change is visible.

The corpus includes valid objects and deliberately broken ones: a tampered
DSSE payload, a wrong tree, a broken segment chain, an undeclared path,
understated `linesAdded`, a clean merge, a merge with its own tree, an
approval replayed onto another commit, a self-approval, a wrong role, a
revoked key, a machine key used as an approver, a `humanLabels: unverified`
path rule on a human-labeled edit, a ledger that is intact, tampered,
unanchored, and signed with an untrusted checkpoint key, and an evidence
package with a changed member and a bad manifest signature.

That corpus is the v1.0 compatibility suite for `change/v1`, `approval/v1`,
the ledger encoding, and the evidence manifest. After the v1.0.0 tag, a change
to a vector under `conformance/v1/` is a breaking change. New predicate
versions get a new directory. They do not replace the v1 vectors.

Building the corpus will hit every spec gap above, because the generator has
to freeze behavior the spec does not state. That is a reason to write those
rules into `docs/spec/` before calling the verifier an implementation of the
spec. The corpus can still lock the current behavior first.

## Security and provenance considerations

The failure that matters is a verifier bug that prints a pass for a commit,
package, or ledger that the checks reject. That result is what an auditor
would rely on. The program cannot forge, strip, or replay an attestation inside
a repository: it does not write git objects, notes, or ledger lines. It can
still be wrong in the direction that helps an attacker, by skipping a check.

Fail closed. A parse error, a missing required input, an unknown policy or
trust-store version, a digest mismatch, or an internal error is a failure
(exit 1 or 2). It is not a pass. An empty trust store accepts no signature.
`LoadTrustStore` returns an empty store when the file is absent (check 17).
This program does not. A missing `--trust` is exit 2. Unknown `predicateType`
values do not satisfy `change/v1` or `approval/v1`. The report lists them on
the commit as `unrecognized`. Today's gate ignores them and then reports that
no change attestation exists (`internal/verify/verify.go`). Pass or fail stays
the same. The extra field is so a `change/v2` envelope is visible.

Zip members are data. The program reads them in memory and does not use entry
names as filesystem paths. Package pass or fail still matches check 20:
`evidence.Verify` keeps whatever name the header has and ignores members that
are not listed in the manifest (`internal/evidence/evidence.go`). An unlisted
name does not, by itself, fail the package scope.

The verifier has its own supply chain. A release is the source revision, a
static binary, a checksum file, and a detached signature from a release key
that is published separately from the keys in any customer trust store.
Reviewers compare the binary to a build of the tagged source. The release key
is not a Seralith developer key and is not loaded from a repository under
test. The program does not verify itself by calling `seralith`.

A ledger result is the result `Ledger.Verify` would give for that file. It
does not prove the file is the only log that was ever kept, and it does not
prove entries after the last checkpoint were never deleted, if what remains
still chains (check 19).

## Compatibility

The program accepts `https://seralith.com/attestation/change/v1` and
`https://seralith.com/attestation/approval/v1` only (`pkg/provenance/provenance.go`).
Those are the predicates the gate accepts. Existing v1 envelopes, policies
(`version: 1`), and trust stores (`version: 1`) keep their current meaning. A
new predicate version is a new document and a new corpus directory. It does
not pass a v1 check. A `change/v2` envelope on a commit that has no valid
`change/v1` attestation fails the gate scope, just as the gate fails that
commit today. A policy or trust store whose `version` is not 1 is exit 2,
matching `policy.Parse` and `ParseTrustStore`, which reject the file before
verification starts.

The ledger encoding and the evidence manifest have no version field in a spec.
Check 19 and check 20 are the behavior to freeze in `conformance/v1/`. Changing
the hash preimage or the manifest payload after that is a breaking change,
same as a predicate change.

This note does not add a predicate, a policy field, or a trust-store field.

## Alternatives considered

**Wrap `seralith evidence verify`.** The command already checks manifest
hashes and the manifest signature (`internal/evidence/evidence.go`,
`internal/cli/evidence.go`). It does not re-evaluate policy, re-check segment
chains against git, or re-verify the ledger. It is the same binary a
vendor-independent check is meant to avoid. Wrapping it would not meet the
motivation.

**Publish `pkg/dsse` and `pkg/provenance` as a library.** Callers could verify
signatures and decode predicates with Seralith's code. They would still share
that code's defects, and they would still have to implement policy, the merge
rule, the ledger, and the evidence manifest. The library is a useful
dependency for Seralith. It is not a second implementation.

**Do nothing.** v1.0.0 can ship this way. Auditors keep running the vendor
gate and the vendor package check. Buyers who require a second implementation
cannot be told that one exists, and the spec gaps stay implicit in the Go
sources. This note exists so that choice is explicit. It does not block the
release.

## Open questions

1. **License and edition.** The core in this repository is Apache-2.0
   (`LICENSE`, `README.md`). `ee/` is under the commercial license
   (`ee/README.md`). The verifier is useful to an auditor who can read and
   build the source. A paid edition can still be source-available to
   customers. This note does not choose. Ryan decides whether the program is
   Apache-2.0 or a paid edition.
2. **Corpus in the v1.0.0 tag.** The verifier itself is after v1.0.0. The
   corpus can still be generated and tagged with v1.0.0 so the compatibility
   suite exists before the second program does. That is a release-contents
   decision.
3. **Repository and release keys.** Who owns the verifier repository, and who
   holds the release key described above?
4. **Spec fixes before code.** The recommendation is to write checks 2, 4, 7,
   9, 11, 13, 15, 17, 18, 19, and 20 into `docs/spec/` before the verifier is
   described as implementing the spec, and to let the corpus lock today's
   behavior in the meantime. Ryan decides whether a spec change lands before
   any verifier code.
