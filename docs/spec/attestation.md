# Attestation format

Status: **Stable (v1)**

Seralith attestations are [in-toto Statement v1](https://github.com/in-toto/attestation/blob/main/spec/v1/statement.md)
documents wrapped in [DSSE](https://github.com/secure-systems-lab/dsse) envelopes
and signed with Ed25519. Any tool that speaks in-toto can read them.

## Envelope

```json
{
  "payloadType": "application/vnd.in-toto+json",
  "payload": "<base64 statement>",
  "signatures": [{ "keyid": "ed25519:6aff343700915676fefb54c7ad25a643", "sig": "<base64>" }]
}
```

Signatures cover the DSSE pre-authentication encoding
`DSSEv1 <len(type)> <type> <len(payload)> <payload>`. A key ID is `ed25519:`
followed by the first 16 bytes of the SHA-256 of the raw public key, in hex.

## Statement

```json
{
  "_type": "https://in-toto.io/Statement/v1",
  "subject": [{ "name": "git+commit:5a9e07fc…", "digest": { "gitCommit": "5a9e07fc…" } }],
  "predicateType": "https://seralith.com/attestation/change/v1",
  "predicate": { }
}
```

The attestation ID is the SHA-256 of the payload bytes.

## `change/v1` predicate

Describes the observed history behind one commit.

| Field | Type | Description |
| --- | --- | --- |
| `repository` | string | Remote URL, if any |
| `commit` | string | Commit hash; must equal the subject |
| `tree` | string | Commit tree hash |
| `parents` | string[] | Parent commit hashes |
| `workspace`, `branch` | string | Where the work happened |
| `committer` | Actor | The accountable human; must be the signer |
| `segments` | Segment[] | Observed intervals of change, in order |
| `attribution` | FileAttribution[] | Line ranges in the commit tree owned by each segment |
| `summary` | Summary | Files changed and surviving lines per actor kind |
| `policy` | PolicyResult | Decisions at commit time (informational; the gate re-evaluates) |
| `builder` | `{id, version}` | The Seralith build that produced the attestation |
| `createdAt` | RFC 3339 | Creation time |

### Actor

| Field | Description |
| --- | --- |
| `kind` | `human` or `agent` |
| `id` | Principal for humans; profile ID for agents |
| `tool`, `toolVersion` | The program that ran |
| `model` | Model the operator **declared**; recorded as evidence, not verified |
| `operator` | The accountable human an agent acted for |
| `host` | Hostname |

### Segment

| Field | Description |
| --- | --- |
| `id` | Unique ID (`ses_…` for sessions, `seg_…` for manual edits) |
| `kind` | `session` or `manual` |
| `actor` | Actor |
| `command` | argv, for sessions |
| `startedAt`, `endedAt` | Interval |
| `baseTree`, `resultTree` | Snapshots before and after |
| `exitCode` | Process exit status, for sessions |
| `transcriptDigest` | `{ "sha256": … }` of the terminal transcript |
| `files` | Per-file status, line counts, zero-context hunks, and added ranges |

## `approval/v1` predicate

| Field | Description |
| --- | --- |
| `commit` | Approved commit; must equal the subject |
| `approver` | Human actor; must be the signer |
| `role` | Role the approver acts in; the signing key must hold it |
| `rules` | Optional list of rule IDs the approval is limited to |
| `decision` | `approve` or `reject` |
| `comment` | Free text |
| `createdAt` | RFC 3339 |

## Storage

Envelopes are stored one per line in the git note for their commit under
`refs/notes/seralith`, deduplicated by attestation ID. Notes from different
machines merge with git's `cat_sort_uniq` strategy, so there are no conflicts.

## Verification algorithm

Given a commit `C`, a trust store `T`, and a policy `P`:

1. Load envelopes from `refs/notes/seralith` for `C`. Choose a `change/v1`
   statement whose subject is `C` and whose envelope verifies under `T`, where
   at least one accepting key's principal equals `predicate.committer.id`.
2. Require `predicate.tree == tree(C)` and `predicate.parents == parents(C)`.
3. Let `t₀ = tree(parent₁(C))`, or the empty tree for a root commit. Require
   `segments[0].baseTree == t₀`, `segments[i].baseTree ==
   segments[i-1].resultTree`, and `segments[last].resultTree == tree(C)`.
4. Compute the diff from `t₀` to `tree(C)`. Every changed path, including the
   old path of a rename, must appear in some segment's `files`. Lines a path
   adds beyond what the segments declare are attributed to every actor that
   declared the path.
5. Evaluate `P` over the segments' files plus that attribution. Any `deny`
   fails.
6. For each `require-approval` decision with threshold *n* and roles *R*,
   count distinct approvers over `approval/v1` statements that approve `C` and
   sit on `C`, or that approve the range tip and sit on the range tip. An
   approval's `commit` must equal its subject and the commit it approves. Its
   envelope must verify under `T` with a **human** key whose
   principal equals the approver and holds the stated role, where the role is in
   *R* (if *R* is non-empty), the rule list is empty or includes the rule, and
   the approver differs from the committer unless the rule allows
   self-approval. A qualifying `reject` fails. Fewer than *n* approvers fails.

When merges are skipped (the default), a merge commit is skipped only if its
tree equals one parent's tree or the conflict-free result of
`git merge-tree --write-tree` over its two parents. Any other merge, including
one with conflict resolutions, must carry its own change attestation.
