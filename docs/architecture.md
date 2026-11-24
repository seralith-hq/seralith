# Architecture

Sigil has four parts: a CLI and daemon that run on developer machines, an
attestation format that travels with git history, a gate that runs in CI, and
an optional control plane for organizations.

```
 Developer machine                                         CI / merge queue
┌───────────────────────────────────────────────────┐    ┌──────────────────────┐
│  sigil CLI          Workbench UI (React)          │    │ sigil gate           │
│      │                     │  HTTP + WS (loopback) │    │  • verify signatures │
│      ▼                     ▼                       │    │  • check tree chain  │
│  ┌─────────────── sigil daemon ────────────────┐  │    │  • re-evaluate policy│
│  │ workspace  session   snapshot   attest      │  │    │  • check approvals   │
│  │ manager    runner    + diff     + policy    │  │    └──────────▲───────────┘
│  └────┬──────────┬─────────┬──────────┬────────┘  │               │
│       ▼          ▼         ▼          ▼           │   git push    │ git fetch
│  git worktrees  PTY    tree objects  refs/notes/sigil ───────────►│
│                 transcripts           ledger.jsonl│               │
└───────────────────────────────────────────────────┘    ┌──────────┴───────────┐
                                                         │ Sigil Control (ee)   │
                                                         │ registry · policy ·  │
                                                         │ archive · analytics  │
                                                         └──────────────────────┘
```

## Design principles

1. **Observe, do not ask.** Attribution comes from snapshots Sigil takes around
   each session, not from agents describing their own work. An agent cannot
   under-report what it changed.
2. **Re-derive at the gate.** The CI gate trusts nothing in an attestation that
   it can recompute: tree hashes, the segment chain, and policy decisions are
   all checked independently.
3. **No middleman.** Sigil launches coding tools as ordinary processes. It never
   proxies model traffic, holds API keys, or meters tokens. Local and air-gapped
   models work exactly like hosted ones.
4. **Git-native.** Attestations live in `refs/notes/sigil` beside the commits
   they describe. No service is required to verify a repository.
5. **Local-first.** Everything works offline on one machine. The control plane
   adds coordination; it is never in the critical path of writing code.

## Components

### Workspaces (`internal/workspace`)

A workspace is a git worktree on a `sigil/<name>` branch, stored outside the
repository (by default under `~/.local/share/sigil/worktrees`). Each workspace
records:

- `head`: the commit it is based on
- `checkpoint`: the tree hash of the last observed state
- `segments`: observed changes not yet bound to a commit

New workspaces can copy untracked files such as `.env` and run setup commands,
so every agent starts from a working environment.

### Snapshots (`internal/gitx`)

A snapshot writes the entire working tree, including untracked files that are
not ignored, to a git tree object. It uses a temporary index seeded from the
real one, so it is fast on large repositories and never disturbs the user's
staging area. Snapshots are content-addressed, so identical states have
identical hashes.

### Sessions (`internal/session`)

A session runs one command in one workspace under a pseudo-terminal:

1. Observe the workspace. Any change since the checkpoint becomes a **manual
   segment** attributed to the workspace owner.
2. Start the process with `SIGIL_SESSION`, `SIGIL_WORKSPACE`, and
   `SIGIL_ACTOR` in its environment. Stream output to the terminal, any
   number of WebSocket subscribers, and a transcript file.
3. When it exits, snapshot again. The difference becomes a **session
   segment** attributed to the session's actor, with the transcript's SHA-256.

Only one session may run per workspace at a time, so every change has exactly
one author. Parallelism comes from running many workspaces.

### Attribution (`internal/attribution`)

Attribution replays the segments' zero-context diff hunks in order, tracking
which segment owns each line as run-length intervals. Hunks are applied
bottom-up so old-side coordinates remain valid, and content that predates
tracking is an unbounded baseline run, so file contents never need to be read.
`sigil blame` combines this per-commit attribution with `git blame` to
attribute every surviving line in a file across its history.

### Policy (`internal/policy`)

Declarative YAML rules match on actor kind, tool, declared model, path globs,
and change-size thresholds, producing `allow`, `warn`, `require-approval`, or
`deny`. See the [policy specification](spec/policy.md).

### Attestations (`pkg/provenance`, `pkg/dsse`, `internal/notes`)

`sigil commit` stages the whole workspace, commits, and verifies that the
committed tree equals the last observed tree. It then signs an in-toto
Statement whose subject is the commit and whose `change/v1` predicate holds
the segments, attribution, summary, and policy result. The DSSE envelope is
appended to the commit's note in `refs/notes/sigil`. See the
[attestation specification](spec/attestation.md).

### Gate (`internal/verify`)

For every commit in a range, the gate checks:

| Check | Failure means |
| --- | --- |
| A change attestation exists with this commit as subject | The commit was not produced through Sigil |
| Signature by a trusted key whose principal is the committer | Forged, re-signed, or untrusted |
| Attested tree and parents equal the commit's | Attestation copied from another commit |
| Segments chain from the parent tree to the commit tree | The commit contains unobserved changes |
| Policy, re-evaluated with the gate's own policy file | A rule denies the change |
| Approvals from trusted human keys with the required role, independent of the committer | Review requirement unmet |

Approvals attached to the range tip apply to every commit in the range, which
matches how pull request reviews work.

### Ledger (`internal/ledger`)

Every workspace, session, commit, approval, override, gate run, and key change
is appended to a JSON Lines ledger in the repository's git directory. Each
entry includes the SHA-256 of its predecessor; signed checkpoints prevent a
rewrite of the whole chain without the signing key.

### Daemon and workbench (`internal/daemon`, `web/`)

`sigil serve` exposes the same operations over a JSON API on the loopback
interface, with server-sent events for live updates and a WebSocket bridge for
terminals. The React workbench is embedded in the binary and makes no external
requests.

### Sigil Control (`ee/`)

The commercial control plane is a single binary with embedded SQLite. It holds
the organization key registry and published policies, archives and re-verifies
uploaded attestations, reports analytics, and keeps a hash-chained audit
trail. See [ee/README.md](../ee/README.md).

## Data locations

| Data | Location | Shared |
| --- | --- | --- |
| Configuration, policy, trust store | `<repo>/.sigil/` | Committed |
| Attestations | `refs/notes/sigil` | Pushed with `sigil sync push` |
| Workspace database, ledger, transcripts | `<git-common-dir>/sigil/` | Local |
| Worktrees | `~/.local/share/sigil/worktrees/` | Local |
| Signing key | `~/.config/sigil/keys/default.pem` | Never |
