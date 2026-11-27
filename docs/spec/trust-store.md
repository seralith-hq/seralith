# Trust store

Status: **Stable (version 1)**

The trust store is the set of public keys that may sign attestations,
approvals, evidence manifests, and ledger checkpoints. It is YAML, normally
committed at `.seralith/trust.yaml`. The rules in this document are the
acceptance rules.

## Document

| Field | YAML | Rule |
| --- | --- | --- |
| `version` | integer | MUST be `1` when present. An omitted `version` is version 1 |
| `keys` | list | Trust entries. An omitted or empty list is an empty store |

A field other than `version` and `keys` is ignored. A file that does not
exist, and an empty file, both load as version 1 with no keys. `version`
other than 1, including `0` when the field is present as `0`, MUST be
rejected. A key whose public key does not parse, or whose declared `id`
does not match the [key identifier](attestation.md#key-identifier), MUST
reject the whole document.

## Keys

Each entry has these fields:

| Field | YAML | Rule |
| --- | --- | --- |
| `id` | string | Key id from the raw Ed25519 public key. MUST equal the id computed from `publicKey` |
| `principal` | string | Accountable identity, compared by exact string equality |
| `kind` | string | `human` or `machine`. Approval checks require `human`. Change-attestation checks do not read `kind` |
| `roles` | list of strings | Role names. Membership is exact string equality. Order does not matter. Omitted when empty in files this implementation writes |
| `publicKey` | string | PKIX `PUBLIC KEY` PEM for an Ed25519 public key |
| `revoked` | boolean | When `true`, the key is not used to verify signatures. Omitted when false in files this implementation writes |

`kind` is how a key is marked human. There is no separate boolean. A key
with `kind: machine` can satisfy a change attestation's committer check when
its principal equals `committer.id`. It cannot satisfy an approval.

When an `id` appears more than once, resolution uses the first entry. A
field on an entry other than the ones in the table above is ignored.

## Resolution

`Resolve` returns the public key for an id only when an entry with that id
exists, `revoked` is not true, and the public key parsed. Revoked keys stay
in the file and are still returned by a lookup that does not filter them.
Revocation applies to the store loaded for that run. It does not rewrite an
earlier result that used a different store.

Checkpoint verification, DSSE verification, and approval checks all use this
resolution. An unknown checkpoint key is reported and is not a chain
failure; see the [ledger](ledger.md#verification).

## Loading

`seralith gate` and `seralith verify` load `.seralith/trust.yaml` from the
working tree when `--config-from` is not set. A missing file is an empty
version-1 store, which accepts no signature.

When `--config-from REV` is set, the blob `REV:.seralith/trust.yaml` is
required together with `REV:.seralith/policy.yaml`. If either blob is
missing, the command fails before verification. The rule is stated with the
[attestation checks](attestation.md#policy-and-trust-for-a-run).

`seralith evidence export` reads the working-tree file and does not take
`--config-from`.

## Example

`seralith init`, `seralith key trust`, and `seralith key revoke` wrote this
file. The two human keys are active. The machine key is revoked. Private
keys are not part of the file.

```yaml
version: 1
keys:
    - id: ed25519:ff83d79db1e82893783a6e89bd42756b
      principal: dev@example.com
      kind: human
      roles:
        - maintainer
      publicKey: |
        -----BEGIN PUBLIC KEY-----
        MCowBQYDK2VwAyEAQ85ezcfI4Q4Vt8t02j/ctIIP85F/wKeic2xWsDwK1eQ=
        -----END PUBLIC KEY-----
    - id: ed25519:b67ac11ab00527c1ef94b4450adfefa7
      principal: other@example.com
      kind: machine
      roles:
        - ci
      publicKey: |
        -----BEGIN PUBLIC KEY-----
        MCowBQYDK2VwAyEA7B+8wnEv3i2HoE9OagvD9LbmYGLzNYGNcm9p7VnejGg=
        -----END PUBLIC KEY-----
      revoked: true
    - id: ed25519:a43b0dfdc7d6b14c1a5c3f88ebeca405
      principal: reviewer@example.com
      kind: human
      roles:
        - maintainer
      publicKey: |
        -----BEGIN PUBLIC KEY-----
        MCowBQYDK2VwAyEAymfMWDUSFe45PrpBFQip/KJb8ZuolF7h90mFoiH4oaw=
        -----END PUBLIC KEY-----
```
