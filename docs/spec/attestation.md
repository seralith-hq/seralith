# Attestation format

Status: **Stable (v1)**

Seralith attestations are [in-toto Statement v1](https://github.com/in-toto/attestation/blob/main/spec/v1/statement.md)
documents wrapped in [DSSE](https://github.com/secure-systems-lab/dsse) envelopes
and signed with Ed25519. The rules below are normative for an independent
implementation. They describe the checks the gate performs today.

Related specifications: [policy](policy.md), [trust store](trust-store.md),
[ledger](ledger.md), [evidence packages](evidence.md).

## DSSE

An envelope is a JSON object with three fields, in this order when Seralith
writes it:

| Field | JSON name | Rule |
| --- | --- | --- |
| Payload type | `payloadType` | MUST be `application/vnd.in-toto+json` for a statement |
| Payload | `payload` | Standard base64 of the raw statement bytes |
| Signatures | `signatures` | Array of `{ "keyid", "sig" }` objects |

`encoding/json` ignores any other field. A parser MUST reject an envelope
whose `payloadType` or `payload` is empty or missing. The signature value is
standard base64 of the raw Ed25519 signature (64 bytes). Standard base64 is
the alphabet `A–Z`, `a–z`, `0–9`, `+`, `/`, with `=` padding. Decoding MUST
reject a missing pad and any character outside that alphabet.

### Pre-authentication encoding

The signed message is the DSSE pre-authentication encoding (PAE) of the
payload type and the **decoded** payload bytes, not the base64 text:

```
DSSEv1 <len(payloadType)> <payloadType> <len(payload)> <payload>
```

`DSSEv1` is the six ASCII bytes `DSSEv1`. Each `<len(...)>` is the byte
length written in base 10 with no sign and no leading zeros. A single ASCII
space (`0x20`) separates the five pieces. There is no trailing space or
newline. For a statement the type length is 28, because
`application/vnd.in-toto+json` is 28 bytes, so the message begins:

```
DSSEv1 28 application/vnd.in-toto+json <len> <payload bytes>
```

Verification MUST fail when `payloadType` is empty or `signatures` is empty,
before any key is consulted. The gate and evidence verification require one
accepted signature (threshold 1). A caller that passes a threshold below 1
is treated as threshold 1.

### Key identifier

A key id MUST be the ASCII string `ed25519:` followed by the lowercase
hexadecimal encoding of the first 16 bytes of SHA-256 over the raw 32-byte
Ed25519 public key. The hash is of those 32 key bytes, not of the PKIX
`SubjectPublicKeyInfo` and not of the PEM text.

The dev key in the [worked trust store](trust-store.md#example) has raw
public-key bytes

```
43ce5ecdc7c8e10e15b7cb74da3fdcb4820ff3917fc0a7a2736c56b03c0ad5e4
```

and key id `ed25519:ff83d79db1e82893783a6e89bd42756b`. A trust-store entry
whose declared `id` differs from this computation MUST be rejected at parse
time.

### Repeated key ids

A key id counts at most once toward the threshold. Verification walks
`signatures` from first to last:

- If an earlier signature with the same `keyid` has already verified, this
  signature MUST NOT be verified and MUST NOT be counted again.
- If no earlier signature with that `keyid` has verified, this signature is
  checked. A failure does not stop a later signature with the same `keyid`
  from being checked.
- The accepted set is the distinct key ids for which at least one signature
  verified under a key the trust store resolved.

## Statement

The payload decodes to a JSON object:

| Field | Rule |
| --- | --- |
| `_type` | MUST be `https://in-toto.io/Statement/v1`. Any other value is skipped |
| `subject` | Array of `{ "name", "digest" }` |
| `predicateType` | `https://seralith.com/attestation/change/v1` or `https://seralith.com/attestation/approval/v1` |
| `predicate` | JSON object for that type |

The commit id of a statement is `subject[0].digest.gitCommit` only.
`subject[0].name` and every subject after the first MUST be ignored. A
statement with no subject has an empty commit id.

The attestation id is the lowercase hex SHA-256 of the raw payload bytes
(the bytes inside the base64, not the envelope JSON).

## `change/v1` predicate

Describes the observed history behind one commit. The gate recomputes tree,
parents, the segment chain, the path diff, policy, and approvals. It does
not recompute `attribution`, `summary`, `policy`, `transcriptDigest`,
`builder`, `repository`, `workspace`, `branch`, or `createdAt`. Those fields
remain attested claims.

| Field | Type | Description |
| --- | --- | --- |
| `repository` | string | Remote URL, if any |
| `commit` | string | Commit hash; checked against the subject |
| `tree` | string | Commit tree hash |
| `parents` | string[] | Parent commit hashes, in commit order |
| `workspace`, `branch` | string | Where the work happened |
| `committer` | Actor | Accountable principal. The signing key's principal must equal `committer.id`. The key's kind is not required to be `human` |
| `segments` | Segment[] | Observed intervals of change, in order |
| `attribution` | FileAttribution[] | Line ranges in the commit tree owned by each segment |
| `summary` | Summary | Files changed and surviving lines per actor kind |
| `policy` | PolicyResult | Decisions at commit time. Informational. The gate evaluates its own policy file and does not compare this object |
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

A segment file with a non-empty `oldPath` is also a change to that old path,
with no added lines. See [Diff](#diff).

### Worked `change/v1` statement

`seralith commit` wrote this envelope as the first line of the note on
`77449d82d20f6bab777809cd1be60ff5f4b95de3`. It is one JSON object, shown here
as stored:

```
{"payloadType":"application/vnd.in-toto+json","payload":"eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjEiLCJzdWJqZWN0IjpbeyJuYW1lIjoiZ2l0K2NvbW1pdDo3NzQ0OWQ4MmQyMGY2YmFiNzc3ODA5Y2QxYmU2MGZmNWY0Yjk1ZGUzIiwiZGlnZXN0Ijp7ImdpdENvbW1pdCI6Ijc3NDQ5ZDgyZDIwZjZiYWI3Nzc4MDljZDFiZTYwZmY1ZjRiOTVkZTMifX1dLCJwcmVkaWNhdGVUeXBlIjoiaHR0cHM6Ly9zZXJhbGl0aC5jb20vYXR0ZXN0YXRpb24vY2hhbmdlL3YxIiwicHJlZGljYXRlIjp7ImNvbW1pdCI6Ijc3NDQ5ZDgyZDIwZjZiYWI3Nzc4MDljZDFiZTYwZmY1ZjRiOTVkZTMiLCJ0cmVlIjoiZGUxMWZkNGViNTk4ODEyMjY3YzE4MTZlM2VlZGMzZDNmMmZmNzhlNiIsInBhcmVudHMiOlsiMjcyY2ViNTcxMjAwYzcyM2M0ZDJhZDIzMDA3MzU4MTMwMzVjNWI5ZiJdLCJ3b3Jrc3BhY2UiOiJkZW1vIiwiYnJhbmNoIjoic2VyYWxpdGgvZGVtbyIsImNvbW1pdHRlciI6eyJraW5kIjoiaHVtYW4iLCJpZCI6ImRldkBleGFtcGxlLmNvbSJ9LCJzZWdtZW50cyI6W3siaWQiOiJzZWdfMWEyMWJjMjEwMDZhNDc4MGJkNjgyIiwia2luZCI6Im1hbnVhbCIsImFjdG9yIjp7ImtpbmQiOiJodW1hbiIsImlkIjoiZGV2QGV4YW1wbGUuY29tIn0sInN0YXJ0ZWRBdCI6IjIwMjYtMTEtMjdUMDY6NDc6MTIuNjI0MzYzODkxWiIsImVuZGVkQXQiOiIyMDI2LTExLTI3VDA2OjQ3OjEyLjY0NjYyODE3NFoiLCJiYXNlVHJlZSI6IjQzOGVhZGFmNGNjMjM3YzcxMTQzN2VmMDYzNDAzMmYxMmVhNzQ3NmYiLCJyZXN1bHRUcmVlIjoiZGUxMWZkNGViNTk4ODEyMjY3YzE4MTZlM2VlZGMzZDNmMmZmNzhlNiIsImZpbGVzIjpbeyJwYXRoIjoiaGVsbG8udHh0Iiwic3RhdHVzIjoiYWRkZWQiLCJsaW5lc0FkZGVkIjoxLCJsaW5lc0RlbGV0ZWQiOjAsImh1bmtzIjpbeyJvbGRTdGFydCI6MCwib2xkTGluZXMiOjAsIm5ld1N0YXJ0IjoxLCJuZXdMaW5lcyI6MX1dLCJhZGRlZCI6W3sic3RhcnQiOjEsImVuZCI6MX1dfV19XSwiYXR0cmlidXRpb24iOlt7InBhdGgiOiJoZWxsby50eHQiLCJyYW5nZXMiOlt7InN0YXJ0IjoxLCJlbmQiOjEsInNlZ21lbnQiOiJzZWdfMWEyMWJjMjEwMDZhNDc4MGJkNjgyIn1dfV0sInN1bW1hcnkiOnsiZmlsZXNDaGFuZ2VkIjoxLCJsaW5lc0J5S2luZCI6eyJodW1hbiI6MX0sInNlZ21lbnRzIjoxfSwicG9saWN5Ijp7ImRpZ2VzdCI6eyJzaGEyNTYiOiJmY2ZmNDY0MmI0NmE0NWFlMDk4OWEwYWM0NzFlNTA4ZjhhNDFkYWQ3MWIxNDAwOTkzYjM1YzlmM2QwZTczNDlkIn0sImRlY2lzaW9ucyI6W3sicnVsZSI6InJldmlldy10ZXh0IiwiZWZmZWN0IjoicmVxdWlyZS1hcHByb3ZhbCIsIm1lc3NhZ2UiOiJDaGFuZ2VzIHRvIHRleHQgZmlsZXMgbmVlZCBvbmUgYXBwcm92YWwuIiwicGF0aHMiOlsiaGVsbG8udHh0Il0sImFwcHJvdmFscyI6MSwicm9sZXMiOlsibWFpbnRhaW5lciJdfV19LCJidWlsZGVyIjp7ImlkIjoiaHR0cHM6Ly9zZXJhbGl0aC5jb20vYnVpbGRlci93b3JrYmVuY2giLCJ2ZXJzaW9uIjoiZGV2In0sImNyZWF0ZWRBdCI6IjIwMjYtMTEtMjdUMDY6NDc6MTIuNjU4NDA4NTc4WiJ9fQ==","signatures":[{"keyid":"ed25519:ff83d79db1e82893783a6e89bd42756b","sig":"dRsInL+aAePVtHT6gjXGgcCdbVh4Tk/9zD50uU9SHnXpMcae0vdGAFH3FuBEDSPaKHENJYdBSuAC4vTL6rwBCw=="}]}
```

The payload decodes to the statement below. The wire payload is compact JSON
with no insignificant whitespace. Its SHA-256, the attestation id, is
`ed5fe4d765e5e2e59d6f2699aee124da1ca474b2351dafec0b8d7ffa0c632f9c`. The JSON
below is that payload indented for reading.

```json
{
  "_type": "https://in-toto.io/Statement/v1",
  "subject": [
    {
      "name": "git+commit:77449d82d20f6bab777809cd1be60ff5f4b95de3",
      "digest": {
        "gitCommit": "77449d82d20f6bab777809cd1be60ff5f4b95de3"
      }
    }
  ],
  "predicateType": "https://seralith.com/attestation/change/v1",
  "predicate": {
    "commit": "77449d82d20f6bab777809cd1be60ff5f4b95de3",
    "tree": "de11fd4eb598812267c1816e3eedc3d3f2ff78e6",
    "parents": [
      "272ceb571200c723c4d2ad2300735813035c5b9f"
    ],
    "workspace": "demo",
    "branch": "seralith/demo",
    "committer": {
      "kind": "human",
      "id": "dev@example.com"
    },
    "segments": [
      {
        "id": "seg_1a21bc21006a4780bd682",
        "kind": "manual",
        "actor": {
          "kind": "human",
          "id": "dev@example.com"
        },
        "startedAt": "2026-11-27T06:47:12.624363891Z",
        "endedAt": "2026-11-27T06:47:12.646628174Z",
        "baseTree": "438eadaf4cc237c711437ef0634032f12ea7476f",
        "resultTree": "de11fd4eb598812267c1816e3eedc3d3f2ff78e6",
        "files": [
          {
            "path": "hello.txt",
            "status": "added",
            "linesAdded": 1,
            "linesDeleted": 0,
            "hunks": [
              {
                "oldStart": 0,
                "oldLines": 0,
                "newStart": 1,
                "newLines": 1
              }
            ],
            "added": [
              {
                "start": 1,
                "end": 1
              }
            ]
          }
        ]
      }
    ],
    "attribution": [
      {
        "path": "hello.txt",
        "ranges": [
          {
            "start": 1,
            "end": 1,
            "segment": "seg_1a21bc21006a4780bd682"
          }
        ]
      }
    ],
    "summary": {
      "filesChanged": 1,
      "linesByKind": {
        "human": 1
      },
      "segments": 1
    },
    "policy": {
      "digest": {
        "sha256": "fcff4642b46a45ae0989a0ac471e508f8a41dad71b1400993b35c9f3d0e7349d"
      },
      "decisions": [
        {
          "rule": "review-text",
          "effect": "require-approval",
          "message": "Changes to text files need one approval.",
          "paths": [
            "hello.txt"
          ],
          "approvals": 1,
          "roles": [
            "maintainer"
          ]
        }
      ]
    },
    "builder": {
      "id": "https://seralith.com/builder/workbench",
      "version": "dev"
    },
    "createdAt": "2026-11-27T06:47:12.658408578Z"
  }
}
```

The signature is the Ed25519 signature, under
`ed25519:ff83d79db1e82893783a6e89bd42756b`, of the PAE of
`application/vnd.in-toto+json` and those compact payload bytes.
`segments[0].baseTree` is the tree of the parent commit
`272ceb571200c723c4d2ad2300735813035c5b9f`, and `resultTree` equals the
commit tree.

## `approval/v1` predicate

| Field | Description |
| --- | --- |
| `commit` | Approved commit; must equal the subject and the commit the approval is applied to |
| `approver` | Actor; the signing key's principal must equal `approver.id`, and the key's kind must be `human` |
| `role` | Role the approver acts in. When non-empty, the signing key must list it |
| `rules` | Optional list of rule IDs the approval is limited to |
| `decision` | See [Approvals](#approvals). The string `reject` is a rejection. Every other value, including `approve`, counts as an approval |
| `comment` | Free text |
| `createdAt` | RFC 3339 |

### Worked approval envelope

The same note's third line is an approval by `reviewer@example.com`. Stored
bytes:

```
{"payloadType":"application/vnd.in-toto+json","payload":"eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjEiLCJzdWJqZWN0IjpbeyJuYW1lIjoiZ2l0K2NvbW1pdDo3NzQ0OWQ4MmQyMGY2YmFiNzc3ODA5Y2QxYmU2MGZmNWY0Yjk1ZGUzIiwiZGlnZXN0Ijp7ImdpdENvbW1pdCI6Ijc3NDQ5ZDgyZDIwZjZiYWI3Nzc4MDljZDFiZTYwZmY1ZjRiOTVkZTMifX1dLCJwcmVkaWNhdGVUeXBlIjoiaHR0cHM6Ly9zZXJhbGl0aC5jb20vYXR0ZXN0YXRpb24vYXBwcm92YWwvdjEiLCJwcmVkaWNhdGUiOnsiY29tbWl0IjoiNzc0NDlkODJkMjBmNmJhYjc3NzgwOWNkMWJlNjBmZjVmNGI5NWRlMyIsImFwcHJvdmVyIjp7ImtpbmQiOiJodW1hbiIsImlkIjoicmV2aWV3ZXJAZXhhbXBsZS5jb20ifSwicm9sZSI6Im1haW50YWluZXIiLCJkZWNpc2lvbiI6ImFwcHJvdmUiLCJjb21tZW50IjoibG9va3MgcmlnaHQiLCJjcmVhdGVkQXQiOiIyMDI2LTExLTI3VDA2OjQ3OjMyLjczNDgxMzI2N1oifX0=","signatures":[{"keyid":"ed25519:a43b0dfdc7d6b14c1a5c3f88ebeca405","sig":"h8/0rnMPnhD6lk5iXJluSBT/F5El/tKZdIIUe7Gm7CpM4hKApo+ZC6jwquGF/PyQj5iY7WRMCV121Ifu21tDAA=="}]}
```

The payload bytes, whose SHA-256 is
`1f90eaea6e00cbc48e8474d7f4a236cdbb31b244c25dae2aaeff539a4d88e2a0`, are:

```
{"_type":"https://in-toto.io/Statement/v1","subject":[{"name":"git+commit:77449d82d20f6bab777809cd1be60ff5f4b95de3","digest":{"gitCommit":"77449d82d20f6bab777809cd1be60ff5f4b95de3"}}],"predicateType":"https://seralith.com/attestation/approval/v1","predicate":{"commit":"77449d82d20f6bab777809cd1be60ff5f4b95de3","approver":{"kind":"human","id":"reviewer@example.com"},"role":"maintainer","decision":"approve","comment":"looks right","createdAt":"2026-11-27T06:47:32.734813267Z"}}
```

PAE for this payload is the 522 bytes
`DSSEv1 28 application/vnd.in-toto+json 479 ` followed by those 479 payload
bytes. The signature verifies under
`ed25519:a43b0dfdc7d6b14c1a5c3f88ebeca405`.

## Storage

Attestations live in the git notes ref `refs/notes/seralith`.

The ref points at a commit written by `git notes`. That commit's tree holds
one blob per annotated commit. Readers MUST obtain the note with
`git notes --ref=refs/notes/seralith show <commit>`, which is how Seralith
reads it. For commit `77449d82d20f6bab777809cd1be60ff5f4b95de3` the tree has
one entry, and the blob path is that commit id:

```
100644 blob a681e7282a1f511a4991e15ac3e63915380e896a	77449d82d20f6bab777809cd1be60ff5f4b95de3
```

The blob is text. Each envelope is one compact JSON object, one per line,
and every line including the last ends with `\n`. An empty line is ignored.
A line that is not a DSSE envelope with a non-empty `payloadType` and
`payload` is skipped. An envelope is also skipped when its payload type is
not `application/vnd.in-toto+json`, when the payload is not a JSON statement,
or when `_type` is not `https://in-toto.io/Statement/v1`. A skipped line does
not by itself fail the commit. If no remaining envelope passes
[selection](#selecting-a-change-attestation), the commit fails.

Writing replaces any stored envelope whose attestation id (SHA-256 of the
decoded payload) equals the new envelope's id. The new bytes are
`encoding/json` of the envelope, then a newline.

Notes from two machines merge with
`git notes --ref=refs/notes/seralith merge -s cat_sort_uniq`. Fetch stores
the remote ref at `refs/notes/seralith-remote` and merges that into
`refs/notes/seralith`.

## Verification

Given a commit `C`, a trust store `T`, and a policy `P`, the result is one
of `pass`, `warn`, `fail`, or `skipped`. `skipped` is only the clean-merge
case in [Merge commits](#merge-commits).

### Selecting a change attestation

Walk `change/v1` envelopes in the order they appear in the note.

1. The statement's commit id MUST equal `C`. Otherwise record a subject
   mismatch and try the next envelope.
2. The envelope MUST verify under `T` at threshold 1. Otherwise record that
   the signature is not trusted and try the next envelope.
3. The predicate MUST decode as `change/v1`. Otherwise try the next envelope.
4. At least one accepted key's principal MUST equal `predicate.committer.id`.
   The key's `kind` is not consulted. Other accepted keys are signers either
   way. If no accepted key's principal equals the committer, record that and
   try the next envelope.

The first envelope that passes all four steps is the change attestation.
Later envelopes MUST NOT be considered. Problems recorded for earlier
envelopes are discarded once one is selected. If none is selected, those
problems remain. If none were recorded, the commit fails with no change
attestation.

### Integrity

For the selected predicate:

- `predicate.commit` MUST equal `C`.
- `predicate.tree` MUST equal the tree of `C` (`git rev-parse C^{tree}`).
- `predicate.parents` MUST equal the parents of `C` in order. Parents come
  from `git rev-list --parents -n 1 C`, every field after the commit id.

### Segment chain

Let `t₀` be the tree of `parents[0]`, or, when `C` has no parents, the
object id `4b825dc642cb6eb9a060e54bf8d69288fbee4904` (git's empty tree).
The chain is checked as follows:

- When `segments` is empty, the check passes if and only if the tree of `C`
  equals `t₀`.
- Otherwise `segments[0].baseTree` MUST equal `t₀`, each later
  `baseTree` MUST equal the previous segment's `resultTree`, and the last
  `resultTree` MUST equal the tree of `C`.

### Diff

The paths and added-line counts the gate trusts are the output of these
three git commands, run in the repository, with `core.quotePath=false` and
`color.ui=never` set on the git command. `from` is `t₀` and `to` is the tree
of `C`. When `from` equals `to`, the diff is empty and git is not run.

```
git -c core.quotePath=false -c color.ui=never diff --no-ext-diff --name-status -z -M <from> <to>
git -c core.quotePath=false -c color.ui=never diff --no-ext-diff --numstat -z -M <from> <to>
git -c core.quotePath=false -c color.ui=never diff --no-ext-diff -U0 -M --src-prefix=a/ --dst-prefix=b/ <from> <to>
```

`-M` is git's default rename detection (similarity threshold 50%). The
command does not pass `-C`. A name-status record whose first byte is `C` is
still a rename: the next NUL-terminated field is the old path and the one
after that is the new path. The same old-path rule applies to `R`. A
similarity suffix such as `R100` is ignored beyond that first byte. `A` is
an add, `D` is a deletion, and any other first byte (including `M` and `T`)
is a modification.

Numstat supplies line counts, keyed by the new path. When the added-line
field is `-`, the file is binary and both the added and deleted counts are
0. Otherwise those two fields are the decimal counts. For the worked commit,
name-status is the bytes `A\0hello.txt\0` and numstat is the bytes
`1\t0\thello.txt\0`, which is the segment's `linesAdded` of 1.

Policy input starts as one change per segment file: the actor, `path`, and
`linesAdded`. A non-empty `oldPath` adds a second change for that path with
zero added lines. Then, for each path git reported:

- The path MUST appear in that set. Otherwise the commit fails.
- When git reported an old path, that old path MUST appear in the set too.
- Let `extra` be git's added-line count minus the sum of `linesAdded`
  declared for the new path. When `extra` is greater than zero, add one
  change for each distinct actor that declared the path, with that path and
  `extra` added lines. Those changes are policy input.
- `extra` less than or equal to zero is not a failure. A path a segment
  declares and git does not report stays policy input and is not a failure.

### Policy decisions

Evaluate `P` over that change list. The `policy` object inside the
attestation is not an input. Rules are specified in the
[policy language](policy.md). Each decision is enforced:

| Effect | Result |
| --- | --- |
| `deny` | Fail |
| `warn` | Pass this decision, and the commit is `warn` if nothing fails |
| `require-approval` | Fail until [Approvals](#approvals) satisfies it |
| `allow` | Pass this decision |

The gate enforces every decision. It does not keep only the strongest one.
`seralith commit` uses the strongest effect (`deny` >
`require-approval` > `warn` > `allow`) only to decide whether to refuse the
commit.

### Approvals

Collect `approval/v1` envelopes on `C`, and, when checking a range, also the
`approval/v1` envelopes on the last commit of
`git rev-list --reverse <range>`. An approval on that tip applies to every
commit in the range. Its target commit is the tip. An approval on `C`
applies only to `C`, and its target is `C`.

A predicate that does not decode as `approval/v1` is skipped. It is not an
approval and it is not a rejection.

An approval that decodes is valid only when all of the following hold:

- `subject[0].digest.gitCommit` equals `predicate.commit`.
- `predicate.commit` equals the target commit. An empty target is not equal.
- The envelope verifies under `T` at threshold 1.
- Some accepted key has principal `approver.id`, kind `human`, and, when
  `role` is non-empty, that role in its role list. The first such key is
  enough.

For each `require-approval` decision with threshold *n* and roles *R*, count
distinct `approver.id` values among valid approvals that also pass:

- `rules` is empty or contains the decision's rule id.
- *R* is empty or contains the approval's `role`.
- The approver differs from `predicate.committer.id`, unless the policy rule
  sets `allowSelfApproval`.

A valid approval whose `decision` is the string `reject` and that passes
those three filters fails the commit. Any other `decision` string, including
`approve` and the empty string, counts as an approval. Fewer than *n*
distinct approvers fails the commit.

On the worked commit the note also carries an approval by the committer,
`dev@example.com`. That approval verifies and does not count, because the
rule does not set `allowSelfApproval`. The reviewer approval does count.
`seralith verify` on that commit reported:

```
pass   77449d82d20f docs: add hello
       signed by dev@example.com
       attribution human 1 (100%)
       require-approval review-text satisfied reviewer@example.com
```

### Merge commits

`seralith gate` skips clean merges unless `--include-merges` is set.
`seralith verify` does not skip merges. `seralith evidence export` always
skips clean merges and has no flag to turn that off.

A commit is a merge when it has more than one parent. It is clean when
either:

- its tree equals the tree of any parent, or
- it has exactly two parents and its tree equals the conflict-free result of

```
git -c core.quotePath=false -c color.ui=never merge-tree --write-tree --no-messages <parent0> <parent1>
```

Parents are in the order `git rev-list --parents` prints them. The result is
the first line of stdout, with surrounding whitespace removed. A non-zero
exit from `merge-tree` means the merge is not clean. It is not a command
error, and stdout is ignored in that case. A merge with more than two
parents is clean only when its tree equals some parent's tree.

A clean merge is `skipped` and is not required to carry a change
attestation. Any other merge is checked like any other commit. The check
records that the merge is not a clean merge of its parents and then
continues. A change attestation that passes selection clears that record,
and the commit is then held to the integrity, diff, policy, and approval
rules. If no change attestation is selected, the merge record remains, and
the generic "no change attestation" failure is not added on top of a problem
that was already recorded.

### Policy and trust for a run

`seralith gate` and `seralith verify` take `--config-from REV`. When it is
set, both `.seralith/policy.yaml` and `.seralith/trust.yaml` are read as
blobs at `REV` (`git cat-file blob REV:.seralith/policy.yaml` and the same
for `trust.yaml`). If either blob is missing, or either file does not parse,
the command fails before it verifies commits. A revision that has only one
of the two files fails. From a repository whose root commit does not contain
the directory:

```
error: read policy at 0ac8419838fdf4a41c342558d6e408f8033a592d: git cat-file blob 0ac8419838fdf4a41c342558d6e408f8033a592d:.seralith/policy.yaml: exit status 128: fatal: path '.seralith/policy.yaml' exists on disk, but not in '0ac8419838fdf4a41c342558d6e408f8033a592d'
```

With no `--config-from`, a missing policy file is a version-1 policy with no
rules and an omitted `humanLabels` (treated as `unverified`), and a missing
trust file is a version-1 store with no keys. An empty trust store accepts
no signature. `seralith evidence export` does not take `--config-from`; it
uses the working tree. Parsing and the field rules are in the
[policy](policy.md#loading) and [trust store](trust-store.md#loading)
specifications.
