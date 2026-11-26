# Policy gates

Seralith policy is a list of declarative rules in `.seralith/policy.yaml`. Each
rule matches changes and has one effect: `allow`, `warn`, `require-approval`,
or `deny`. The rules are evaluated when you commit, and `seralith gate`
evaluates them again. A change that needs review fails the gate until the
signed approvals that rule requires are present.

## See it

A compliant change passes, then a protected path with no approval fails.

![seralith gate passing on a compliant change, then failing because a protected path has no approval](../assets/demo/gate.gif)

## How it works

### Rules match changes

A rule is evaluated over **changes**: one actor touching one path. A rename
produces a change for both the old path and the new path, so moving a
protected file out of a protected directory still matches. The change for the
old path carries no added lines.

Every populated field in `when` has to match. `files` and `linesAdded` are
checked against the changes that already matched the other fields, and the
rule matches only when both thresholds pass. `{ gt: n }` means greater than
*n*; a count equal to *n* does not match.

| Field | Matches when |
| --- | --- |
| `actors` | The actor kind is listed (`human`, `agent`) |
| `paths` | The path matches any glob (`**` crosses directories) |
| `exclude` | The path matches none of these globs |
| `files: { gt: n }` | More than *n* distinct paths matched |
| `linesAdded: { gt: n }` | More than *n* lines were added across matched changes |

`tools`, `toolsNotIn`, and `models` match the declared tool or model.
`toolsNotIn` matches an agent whose tool is not in the list. See the
[policy reference](../spec/policy.md) for every field.

### Effects

| Effect | At commit | At the gate |
| --- | --- | --- |
| `allow` | Recorded in the attestation and printed | Pass |
| `warn` | Recorded and printed | Exit 0, with status `warn` |
| `require-approval` | Recorded and printed; the commit succeeds | Fail until the approvals are satisfied |
| `deny` | Refused, exit 3, unless `--force`. The override is appended to the local ledger, and the deny decision is still recorded | Fail. `--force` does not make the gate pass |

When several rules match, every match is recorded. The strongest effect is
the one `seralith commit` treats as the outcome: `deny` >
`require-approval` > `warn` > `allow`. Only `deny` refuses the commit. The
gate enforces each decision from its own evaluation: a `deny` fails the
gate, a `require-approval` fails until it is satisfied, and a `warn` is
reported.

### Approvals

A reviewer signs an approval with `seralith approve`, or a rejection with
`--reject`. The gate counts distinct approvers. An approval counts only when
all of the following hold:

- The signature verifies under the trust store the gate is using, and the
  accepting key is a human key whose principal is the approver.
- If the rule names `roles`, the approval's role is one of them, and the key
  holds that role. A rule with no `roles` accepts any counted approval.
- If the approval lists rules (`--rule`), the decision's rule is one of them.
- The approver is not the committer, unless that rule sets
  `allowSelfApproval: true` (the default is `false`). The committer is the
  human principal on the change attestation, not the actor who wrote the
  lines.

A rejection that meets those conditions fails the gate.

An approval of the tip commit, stored on that commit, applies to every commit
in the range. An approval stored on an earlier commit counts only for that
commit.

`seralith init` grants your key the `maintainer` role. It does not grant
`security`. Approving with `--role security` from a key that does not hold
`security` still writes the approval, and the command warns that it will not
count under the current trust store. The reviewer's public key has to be in
the trust store the gate loads. With `--config-from`, that is the trust store
committed at the revision you name, so adding the key only on the pull
request does not make the approval count.

### The gate re-evaluates policy

The gate does not use the policy decisions stored in an attestation. It
evaluates the policy file it was given, so a tighter rule applies to commits
that are not merged yet. `--config-from <rev>` replaces both
`.seralith/policy.yaml` and `.seralith/trust.yaml` with the files committed
at `<rev>`. If either file is missing there, the command fails. In CI that
revision is the base branch, so a pull request cannot loosen the rules or the
trust store it is judged by.

## Use it

Inspect and validate the policy in the working tree. `seralith policy lint`
also accepts a path, and exits 1 when the file is not valid:

```sh
seralith policy show
seralith policy lint
```

A rule that requires one approval from a key holding `security` for agent
changes under `auth/` and `crypto/`. This is the form in the
[policy specification](../spec/policy.md). `seralith init` writes the same
rule with three further paths: `**/security/**`, `**/*.tf`, and
`**/Dockerfile`.

```yaml
version: 1
humanLabels: unverified
rules:
  - id: security-critical-review
    description: Agent changes to security-critical code need independent human approval.
    when:
      actors: [agent]
      paths: ["**/auth/**", "**/crypto/**"]
    effect: require-approval
    approvals: 1
    roles: [security]
```

The pipeline rule written by `seralith init`, which is the rule the README
describes for CI workflows and the other pipeline files. It denies agent
edits to `.github/workflows/**`, `.gitlab-ci.yml`, `Jenkinsfile`, and
`.circleci/**`. With `humanLabels: unverified`, a human-labeled edit of
those paths is not denied; it needs one independent approval, and this rule
names no roles.

```yaml
version: 1
humanLabels: unverified
rules:
  - id: agents-no-pipeline-edits
    description: Coding agents may not modify CI/CD pipeline definitions.
    when:
      actors: [agent]
      paths: [".github/workflows/**", ".gitlab-ci.yml", "Jenkinsfile", ".circleci/**"]
    effect: deny
    message: Pipeline changes must be authored by a human.
```

`seralith status` shows which rules a workspace's pending changes trigger:

```sh
seralith status --ws payments-retry
```

```text
workspace payments-retry  seralith/payments-retry

SEGMENT       KIND     ACTOR        FILES   LINES
ses_abead710  session  agent:codex  1 file  +4 −0

attribution agent 4 (100%)
policy
  require-approval   security-critical-review (1 approval from security)
    Agent changes to security-critical code need independent human approval.
    internal/crypto/backoff.go
```

A reviewer whose trusted key holds `security` approves the workspace branch,
and the attestation notes are published:

```sh
seralith approve seralith/payments-retry --role security -m "retry backoff reviewed"
seralith sync push
```

`seralith sync push` publishes `refs/notes/seralith` to the remote. CI has to
fetch that ref before the gate can see the approval.

The gate checks the branch against the base branch's policy and fails closed.
`--base origin/main` means the range `origin/main..HEAD`, so `HEAD` has to be
the branch under review, as it is in CI. After `seralith ws new` the primary
checkout stays on the base branch, the range is empty, the command prints
`no commits in range`, and it exits 0. From that checkout, pass
`--range origin/main..seralith/payments-retry`.

```sh
seralith gate --base origin/main --config-from origin/main
```

Before the approval, the gate fails and exits 1:

```text
seralith gate origin/main..HEAD  fail

fail   6535f97bbb15 feat: add retry backoff
       ✗ policy security-critical-review requires 1 approval(s) from role [security]; have 0

0 pass · 0 warn · 1 fail · 0 skipped   lines: agent 4
```

After the approval, the same command passes and exits 0:

```text
seralith gate origin/main..HEAD  pass

pass   6535f97bbb15 feat: add retry backoff

1 pass · 0 warn · 0 fail · 0 skipped   lines: agent 4
```

`seralith gate` exits 1 on a gate failure. `seralith commit` exits 3 when a
`deny` decision refuses the commit. An agent edit of
`.github/workflows/ci.yml` that matches `agents-no-pipeline-edits` makes
`seralith commit` exit 3.

On GitHub, the [GitHub Action](../integrations/github-actions.md) runs
`seralith gate --range origin/<base>..HEAD --config-from origin/<base>` on
every pull request.

## Human labels

Whether a change was written by a person or an agent is recorded by the
committer's own Seralith, so a "human" label is a claim made by the party it
benefits. `humanLabels` decides how much policy relies on that claim.

A rule targets agents when its `actors` include `agent`, or when it names no
`actors` and filters on `tools`, `toolsNotIn`, or `models`.

| Value | Behavior |
| --- | --- |
| `unverified` (default) | A rule that targets agents and names `paths` also applies to human-labeled changes to those paths. Omitting `humanLabels` behaves the same way. A `deny` becomes one independent approval, with the message that human labels are not verified, so a person can still commit the edit. Roles named on the rule still apply. If an agent change matches that same rule, the decision stays `deny` and lists the agent paths. Rules without `paths`, such as a line-count threshold, are not extended to human-labeled changes. |
| `trusted` | Human labels are taken at face value. Agent rules do not apply to human-labeled changes. Use it only when every key holder is trusted not to relabel agent work. |

See the [threat model](../security/threat-model.md) for what the gate does and
does not guarantee.

## Related

- [Policy language](../spec/policy.md)
- [CLI reference](../cli.md)
- [Architecture](../architecture.md)
- [Authorship tracking](./authorship-tracking.md)
