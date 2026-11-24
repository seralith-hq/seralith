# CLI reference

Every command accepts `-C <dir>` to run as if started in another directory and
`--json` for machine-readable output. Run `sigil <command> --help` for all
flags.

## Setup

| Command | Description |
| --- | --- |
| `sigil init [--name] [--roles]` | Create `.sigil/` with config, default policy, and a trust store containing your key |
| `sigil whoami` | Show your principal, key, and whether the repository trusts it |
| `sigil key export` | Print your public key |
| `sigil key trust <pem> --principal <id> [--kind human\|machine] [--roles a,b]` | Trust a key |
| `sigil key revoke <key-id>` | Revoke a key |
| `sigil key list` | List trusted keys |
| `sigil agents` | List supported coding tools and whether they are installed |

## Workspaces and sessions

| Command | Description |
| --- | --- |
| `sigil ws new <name> [--base <rev>]` | Create a workspace on `sigil/<name>` |
| `sigil ws ls` | List workspaces and pending segments |
| `sigil ws path <name>` | Print a workspace directory |
| `sigil ws rm <name> [--force] [--keep-branch]` | Remove a workspace |
| `sigil run [--ws] [--agent] [--model] [-- cmd...]` | Run a tool in a workspace as an attributed session |
| `sigil status [--ws]` | Show pending segments, attribution, and policy |
| `sigil commit -m <msg> [--ws] [--force]` | Commit and attach a signed attestation |

## Provenance and verification

| Command | Description |
| --- | --- |
| `sigil approve <rev> [--role] [--rule] [--reject] [-m]` | Sign an approval or rejection |
| `sigil verify [rev]` | Verify one commit |
| `sigil gate [--range a..b \| --base rev] [--fetch] [--format text\|json\|markdown\|github]` | Verify a range; exits 1 on failure |
| `sigil blame <file> [--rev] [--summary]` | Attribute each line to a human or agent |
| `sigil evidence export --range a..b [-o file]` | Write a signed evidence package |
| `sigil evidence verify <zip> [--trust file]` | Verify an evidence package |
| `sigil policy show \| lint [file] \| default` | Inspect and validate policy |

## Operations

| Command | Description |
| --- | --- |
| `sigil log [--type] [--subject] [-n]` | Show the audit ledger |
| `sigil ledger verify` | Verify the hash chain and checkpoints |
| `sigil ledger checkpoint` | Sign the current ledger head |
| `sigil sync push \| fetch [--remote]` | Publish or fetch `refs/notes/sigil` |
| `sigil serve [--addr] [--open]` | Start the workbench on localhost |
| `sigil cloud login \| push \| pull-trust \| pull-policy \| stats` | Use Sigil Control |
| `sigil version` | Print build information |

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Error, or verification or gate failure |
| 3 | `sigil commit` refused by a `deny` policy decision |
| *n* | `sigil run` exits with the session's own exit code |

## Environment

| Variable | Purpose |
| --- | --- |
| `SIGIL_KEY` | Path to the signing key |
| `SIGIL_PRINCIPAL` | Identity to sign as (defaults to `git config user.email`) |
| `SIGIL_DATA_DIR` | Worktree root and other per-user data |
| `SIGIL_CONFIG_DIR` | Per-user configuration directory |
| `SIGIL_CLOUD_URL`, `SIGIL_CLOUD_TOKEN`, `SIGIL_CLOUD_PROJECT` | Sigil Control credentials for CI |
| `NO_COLOR` | Disable colored output |

Sessions receive `SIGIL_SESSION`, `SIGIL_WORKSPACE`, and `SIGIL_ACTOR`.
