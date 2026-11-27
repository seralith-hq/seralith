# Evidence packages

Status: **Stable**

An evidence package is a zip archive. `seralith evidence verify` checks the
manifest hashes and the manifest signature. It does not re-run the gate, does
not re-hash the ledger, and does not compare `ledger-verify.json` to a fresh
ledger check. A package can verify while the gate report or the ledger report
inside it describes a failure.

## Archive

Members are stored deflated, in path order. Names are the zip header names,
compared as strings. The implementation writes these members:

| Path | Contents |
| --- | --- |
| `gate-report.json` | Gate report for the range, JSON with a trailing newline |
| `controls.json` | Control mapping, JSON with a trailing newline |
| `policy.yaml` | Working-tree policy bytes. Empty when the file is missing |
| `trust.yaml` | Working-tree trust-store bytes. Empty when the file is missing |
| `attestations/<commit>.jsonl` | Note envelopes for each commit in the range that has at least one, one compact JSON object per line |
| `ai-usage.json` | Tools recorded on agent segments, JSON with a trailing newline |
| `ledger.jsonl` | Copy of the ledger file at export time |
| `ledger-verify.json` | Ledger verification report taken at export time, JSON with a trailing newline |
| `report.md` | Human-readable summary |
| `manifest.json` | Manifest, defined below. Not in the hashed list |
| `manifest.sig.json` | DSSE envelope over the exact `manifest.json` bytes. Not in the hashed list |

`seralith evidence export` runs the gate with clean merges skipped, the same
rule as `seralith gate` without `--include-merges`. It has no flag to include
merges. It does not take `--config-from`.

## Manifest

`manifest.json` is Go's `encoding/json.MarshalIndent` of the manifest with
a two-space indent and an empty prefix. There is no trailing newline. The
signature payload MUST be those exact bytes, not a re-encoding of the same
JSON.

| Field | JSON | Rule |
| --- | --- | --- |
| `type` | string | `https://seralith.com/evidence/v1` |
| `project` | string | Project name |
| `range` | string | Revision range passed to export |
| `generated` | string | `time.Time` JSON, UTC |
| `generator` | string | `seralith ` followed by the binary's version |
| `gateStatus` | string | Gate status recorded at export: `pass`, `warn`, `fail`, or `skipped` |
| `files` | array | One object per hashed member, sorted by `path` |

Each file object is `path`, `sha256` (lowercase hex of the member bytes), and
`size` (byte length). `sha256` is the digest of the full member as written.
`size` is recorded and is not read by verification.

## Signature

`manifest.sig.json` is `encoding/json.MarshalIndent` of a DSSE envelope, two
spaces, no trailing newline. The payload type MUST be

```
application/vnd.seralith.evidence-manifest+json
```

The payload bytes MUST equal the `manifest.json` member byte for byte. The
PAE, base64, and key id rules are the [DSSE rules](attestation.md#dsse).
Verification requires one accepted signature under the trust store.

## Verification

Open the zip. For each member, read at most 512 MiB (`512` times `2^20`
bytes). The digest is of the bytes read.

`OK` is true only when all of the following hold:

- `manifest.sig.json` is present, parses as DSSE, has payload type
  `application/vnd.seralith.evidence-manifest+json`, and its decoded payload
  is byte-for-byte equal to `manifest.json`.
- That envelope verifies under the trust store at threshold 1.
- Every `files` entry names a member whose digest equals `sha256`. A missing
  member is listed as missing. A different digest is a mismatch.

A member that is not listed in `files` does not by itself fail the package.
`manifest.json` and `manifest.sig.json` are not hashed. The `size` field is
not compared. The gate report and the ledger report are not re-evaluated.

## Example

`seralith evidence export` wrote this `manifest.json` (1629 bytes, no
trailing newline). `seralith evidence verify --json` reported `signed: true`
and `signedBy` `ed25519:ff83d79db1e82893783a6e89bd42756b`. The payload of
`manifest.sig.json` is these bytes.

```json
{
  "type": "https://seralith.com/evidence/v1",
  "project": "spec-example",
  "range": "272ceb571200c723c4d2ad2300735813035c5b9f..seralith/demo",
  "generated": "2026-11-27T06:47:32.864773466Z",
  "generator": "seralith dev",
  "gateStatus": "pass",
  "files": [
    {
      "path": "ai-usage.json",
      "sha256": "37517e5f3dc66819f61f5a7bb8ace1921282415f10551d2defa5c3eb0985b570",
      "size": 3
    },
    {
      "path": "attestations/77449d82d20f6bab777809cd1be60ff5f4b95de3.jsonl",
      "sha256": "1d26ab9f9bc9e70f77ade74043ec2ab37e78b3603e028b0ce885ccd735008a23",
      "size": 4035
    },
    {
      "path": "controls.json",
      "sha256": "5c51c9adfecf409badb8e262da1d07fc33ee73420a7adae32f4d6af86a53c886",
      "size": 4171
    },
    {
      "path": "gate-report.json",
      "sha256": "051ad148d9baa1a19df2f79ed356918ec4ba9dbfcedae19bdeb40fefb2b56ab3",
      "size": 1507
    },
    {
      "path": "ledger-verify.json",
      "sha256": "ad4f1757e45aa04756f3ca0070df40d39e9a4618eba0f28028162a7b0c3e761e",
      "size": 146
    },
    {
      "path": "ledger.jsonl",
      "sha256": "d92895bcf6fd02cb680926cc80e32106ac143a238fa5abcea43eeb6328dd617e",
      "size": 4878
    },
    {
      "path": "policy.yaml",
      "sha256": "fcff4642b46a45ae0989a0ac471e508f8a41dad71b1400993b35c9f3d0e7349d",
      "size": 202
    },
    {
      "path": "report.md",
      "sha256": "bda4b33ccd96ab5041f582a3421e091e42b8859ea2155f3b6a94e85dd32f0ef3",
      "size": 2976
    },
    {
      "path": "trust.yaml",
      "sha256": "3d44a4e92ca958b82aa2827e4151c42e16e7752078ed2c9d1304a160d9d0da60",
      "size": 914
    }
  ]
}
```
