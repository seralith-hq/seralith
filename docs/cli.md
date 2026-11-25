# CLI reference

Every command accepts `-C <dir>` to run as if started in another directory and
`--json` for machine-readable output. Run `seralith <command> --help` for all
flags.

## Setup

| Command | Description |
| --- | --- |
| `seralith init [--name] [--roles]` | Create `.seralith/` with config, default policy, and a trust store containing your key |
| `seralith whoami` | Show your principal, key, and whether the repository trusts it |
| `seralith key export` | Print your public key |
| `seralith key trust <pem> --principal <id> [--kind human\|machine] [--roles a,b]` | Trust a key |
| `seralith key revoke <key-id>` | Revoke a key |
| `seralith key list` | List trusted keys |
| `seralith agents` | List supported coding tools and whether they are installed |

## Workspaces and sessions

| Command | Description |
| --- | --- |
| `seralith ws new <name> [--base <rev>]` | Create a workspace on `seralith/<name>` |
| `seralith ws ls` | List workspaces and pending segments |
| `seralith ws path <name>` | Print a workspace directory |
| `seralith ws rm <name> [--force] [--keep-branch]` | Remove a workspace |
| `seralith run [--ws] [--agent] [--model] [-- cmd...]` | Run a tool in a workspace as an attributed session |
| `seralith status [--ws]` | Show pending segments, attribution, and policy |
| `seralith commit -m <msg> [--ws] [--force]` | Commit and attach a signed attestation |

## Provenance and verification

| Command | Description |
| --- | --- |
| `seralith approve <rev> [--role] [--rule] [--reject] [-m]` | Sign an approval or rejection |
| `seralith verify [rev]` | Verify one commit |
| `seralith gate [--range a..b \| --base rev] [--fetch] [--format text\|json\|markdown\|github]` | Verify a range; exits 1 on failure |
| `seralith blame <file> [--rev] [--summary]` | Attribute each line to a human or agent |
| `seralith evidence export --range a..b [-o file]` | Write a signed evidence package |
| `seralith evidence verify <zip> [--trust file]` | Verify an evidence package |
| `seralith policy show \| lint [file] \| default` | Inspect and validate policy |

## Operations

| Command | Description |
| --- | --- |
| `seralith log [--type] [--subject] [-n]` | Show the audit ledger |
| `seralith ledger verify` | Verify the hash chain and checkpoints |
| `seralith ledger checkpoint` | Sign the current ledger head |
| `seralith sync push \| fetch [--remote]` | Publish or fetch `refs/notes/seralith` |
| `seralith serve [--addr] [--open]` | Start the workbench on localhost |
| `seralith cloud login \| push \| pull-trust \| pull-policy \| stats` | Use Seralith Control |
| `seralith version` | Print build information |

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Error, or verification or gate failure |
| 3 | `seralith commit` refused by a `deny` policy decision |
| *n* | `seralith run` exits with the session's own exit code |

## Environment

| Variable | Purpose |
| --- | --- |
| `SERALITH_KEY` | Path to the signing key |
| `SERALITH_PRINCIPAL` | Identity to sign as (defaults to `git config user.email`) |
| `SERALITH_DATA_DIR` | Worktree root and other per-user data |
| `SERALITH_CONFIG_DIR` | Per-user configuration directory |
| `SERALITH_CLOUD_URL`, `SERALITH_CLOUD_TOKEN`, `SERALITH_CLOUD_PROJECT` | Seralith Control credentials for CI |
| `NO_COLOR` | Disable colored output |

Sessions receive `SERALITH_SESSION`, `SERALITH_WORKSPACE`, and `SERALITH_ACTOR`.
