# Audit ledger

Status: **Stable**

The ledger is an append-only JSON Lines file. Each line is one entry. The
hash of each entry commits to the previous entry's hash. Checkpoints are
later entries whose Ed25519 signature covers an earlier head. This document
is the pass/fail rule for `seralith ledger verify`.

The file lives at `<git-common-dir>/seralith/ledger.jsonl`. A clone does not
carry it.

## File

UTF-8. One JSON object per line, each line ending in `\n`. An empty line is
ignored. A line that is not JSON fails verification with an error; it is not
recorded as a chain issue.

Object keys are produced by Go's `encoding/json.Marshal` of this structure,
in this order, with default options (HTML escaping on):

| Key | JSON | Present when |
| --- | --- | --- |
| `seq` | number | Always. The first entry is `1`. Each next entry is one greater than the previous entry's `seq` |
| `time` | string | Always. UTC, truncated to microseconds, encoded as `time.Time` JSON: RFC 3339, fractional seconds only when non-zero, trailing zeros in the fraction omitted, `Z` for UTC |
| `type` | string | Always |
| `actor` | string | Omitted when empty |
| `subject` | string | Omitted when empty |
| `data` | JSON value | Omitted when empty. When present, compact JSON. Inside strings, `&`, `<`, `>`, U+2028, and U+2029 are escaped as `\u0026`, `\u003c`, `\u003e`, `\u2028`, and `\u2029` |
| `prev` | string | Always. 64 lowercase hex digits |
| `hash` | string | Always. 64 lowercase hex digits. The field is present even while it is being computed |

The first entry's `prev` MUST be 64 zero digits:

```
0000000000000000000000000000000000000000000000000000000000000000
```

## Entry hash

The hash is the lowercase hex SHA-256 of a preimage. The preimage is Go's
`encoding/json.Marshal`, default options, of the entry after `hash` is set
to the empty string. When `data` is non-empty, it is replaced first by
`encoding/json.Compact` of those bytes (no extra HTML escaping in that
step). `Marshal` then escapes strings as above, including a second compact
of `data` that does apply HTML escaping. The preimage does not include the
trailing newline of the line.

An implementation in another language MUST produce those same bytes. Field
order, omitted empty `actor`, `subject`, and `data`, the empty `hash`
string, and the time encoding are all part of the preimage.

### Worked entry

This is the first line `seralith init` appended:

```
{"seq":1,"time":"2026-11-27T06:47:00.677179Z","type":"project.initialized","actor":"human:dev@example.com","subject":"spec-example","data":{"key":"ed25519:ff83d79db1e82893783a6e89bd42756b","roles":["maintainer"]},"prev":"0000000000000000000000000000000000000000000000000000000000000000","hash":"a29d39478fc8bf108f2d5e119a7cadf2d9d318389c9ac599fce305b3bace43a4"}
```

Its preimage, the same JSON with `hash` set to `""`, is:

```
{"seq":1,"time":"2026-11-27T06:47:00.677179Z","type":"project.initialized","actor":"human:dev@example.com","subject":"spec-example","data":{"key":"ed25519:ff83d79db1e82893783a6e89bd42756b","roles":["maintainer"]},"prev":"0000000000000000000000000000000000000000000000000000000000000000","hash":""}
```

SHA-256 of that preimage is
`a29d39478fc8bf108f2d5e119a7cadf2d9d318389c9ac599fce305b3bace43a4`.

An entry with empty `data` omits the key. This revocation line's preimage is:

```
{"seq":3,"time":"2026-11-27T06:47:12.592029Z","type":"key.revoked","actor":"human:dev@example.com","subject":"ed25519:b67ac11ab00527c1ef94b4450adfefa7","prev":"a16e03036cf2b2f81762b0bb74c3489e34814c74a665989858b63ecb7b30c578","hash":""}
```

The stored line is:

```
{"seq":3,"time":"2026-11-27T06:47:12.592029Z","type":"key.revoked","actor":"human:dev@example.com","subject":"ed25519:b67ac11ab00527c1ef94b4450adfefa7","prev":"a16e03036cf2b2f81762b0bb74c3489e34814c74a665989858b63ecb7b30c578","hash":"81886abcb1bc4e3a0e580c2be916269f4dcf75d1d7d8c483d789c55b290d5b53"}
```

## Checkpoints

A checkpoint is an entry with `type` `ledger.checkpoint`. Its `data` object
has these keys, in this order:

| Key | Meaning |
| --- | --- |
| `seq` | `seq` of the entry that was the head when the checkpoint was signed |
| `head` | `hash` of that entry |
| `keyid` | Key id of the signer |
| `sig` | Standard base64 of the raw 64-byte Ed25519 signature |

The signed message is the raw bytes, with no newline and no DSSE wrapping:

```
seralith-ledger-checkpoint:v1:<seq>:<head>
```

`<seq>` is the decimal `data.seq`. `<head>` is `data.head`. The checkpoint
entry itself has `seq` one greater than `data.seq`, `prev` equal to
`data.head`, `actor` equal to the key id, and `subject` equal to `data.head`.

### Worked checkpoint

`seralith ledger checkpoint` appended this line over entry 7:

```
{"seq":8,"time":"2026-11-27T06:47:12.710395Z","type":"ledger.checkpoint","actor":"ed25519:ff83d79db1e82893783a6e89bd42756b","subject":"a638c034bea8f449c277eaa23e13b1a227801c74e1621d4c3de7ee9d1346597e","data":{"seq":7,"head":"a638c034bea8f449c277eaa23e13b1a227801c74e1621d4c3de7ee9d1346597e","keyid":"ed25519:ff83d79db1e82893783a6e89bd42756b","sig":"5qU5bjqbyG6ikep5O0W8NF+5k7nfc4ByOb/jMVLqHbBhcd+q56GvS4VRpEgwYB36coartftsQh4dRYmmabePAA=="},"prev":"a638c034bea8f449c277eaa23e13b1a227801c74e1621d4c3de7ee9d1346597e","hash":"ad652b10935b8b39041438861b8b4fbd4457bb926c340c6c7c06ab1c1ef7b1aa"}
```

The signed message is the ASCII string

```
seralith-ledger-checkpoint:v1:7:a638c034bea8f449c277eaa23e13b1a227801c74e1621d4c3de7ee9d1346597e
```

The signature verifies under `ed25519:ff83d79db1e82893783a6e89bd42756b`.
The preimage of the checkpoint line is:

```
{"seq":8,"time":"2026-11-27T06:47:12.710395Z","type":"ledger.checkpoint","actor":"ed25519:ff83d79db1e82893783a6e89bd42756b","subject":"a638c034bea8f449c277eaa23e13b1a227801c74e1621d4c3de7ee9d1346597e","data":{"seq":7,"head":"a638c034bea8f449c277eaa23e13b1a227801c74e1621d4c3de7ee9d1346597e","keyid":"ed25519:ff83d79db1e82893783a6e89bd42756b","sig":"5qU5bjqbyG6ikep5O0W8NF+5k7nfc4ByOb/jMVLqHbBhcd+q56GvS4VRpEgwYB36coartftsQh4dRYmmabePAA=="},"prev":"a638c034bea8f449c277eaa23e13b1a227801c74e1621d4c3de7ee9d1346597e","hash":""}
```

SHA-256 of that preimage is
`ad652b10935b8b39041438861b8b4fbd4457bb926c340c6c7c06ab1c1ef7b1aa`.

## Verification

Walk every non-empty line in order. Track the previous `seq` (starting at 0)
and the previous hash (starting at the 64 zero digits).

For each entry:

- `seq` MUST equal the previous sequence plus one. A gap is an issue. The
  walk continues.
- `prev` MUST equal the previous entry's `hash`. A mismatch is an issue.
- The computed hash MUST equal `hash`. A mismatch is an issue.
- When `type` is `ledger.checkpoint`, check the checkpoint below.

For a checkpoint, `data` MUST decode to the object above, and the entry
whose `seq` is `data.seq` MUST already have been seen with `hash` equal to
`data.head`. Otherwise the checkpoint is an issue.

The signature is checked when a resolver is provided, which the
`seralith ledger verify` command always does:

- If the key id does not resolve in the trust store, the key id is listed
  once in `untrustedCheckpointKeys`. That is not an issue, and the checkpoint
  is not counted as verified.
- If the key resolves, the signature MUST verify over the message above.
  Standard base64. A bad signature or bad base64 is an issue, `invalid
  checkpoint signature`.
- A good signature increments the verified-checkpoint count.

`OK` means the issue list is empty. `Anchored` means `OK`, at least one
checkpoint, and every checkpoint was verified. Entries after a checkpoint
stay in the chain. `Anchored` does not require the last line to be a
checkpoint, and it does not require a checkpoint's `seq` to be the file's
last `seq`.

The check applies to the lines in the file. A file that is a prefix of a
longer log, and that still satisfies the rules above, verifies. The check
does not consult any other copy of the log.

`seralith ledger verify` prints:

| Report | When | Exit |
| --- | --- | --- |
| `intact` | Anchored | 0, including with `--strict` |
| `unanchored` | OK, and not anchored | 0, or 1 with `--strict` |
| `tampered` | Not OK | 1 |

Before any checkpoint, the same ledger reported:

```
unanchored  7 entries, 0/0 checkpoints verified, head a638c034bea8
  ! the hash chain is intact, but without checkpoints signed by trusted keys it does not prove the ledger was not rewritten
```

After the checkpoint above:

```
intact  8 entries, 1/1 checkpoints verified, head ad652b10935b
```

Later entries kept the chain anchored, because that checkpoint still
verified:

```
intact  12 entries, 1/1 checkpoints verified, head 691a580f8295
```
