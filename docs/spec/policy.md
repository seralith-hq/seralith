# Policy language

Policies live in `.seralith/policy.yaml`. The gate evaluates the policy file it is
given, not the decisions recorded in an attestation, so tightening a policy
applies retroactively to unmerged work.

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

## Human labels

Whether a segment was written by a human or an agent is recorded by the
committer's own Seralith. A trusted key holder can therefore label agent work
as human, and a "human" label is a claim made by the party it benefits.
`humanLabels` decides how much policy relies on that claim.

| Value | Behavior |
| --- | --- |
| `unverified` (default) | A rule that targets agents and names `paths` also applies to human-labeled changes to those paths. A `deny` becomes a requirement for one independent approval, so people can still edit, for example, pipeline definitions. Rules without `paths`, such as line-count thresholds, apply to agents only. |
| `trusted` | Human labels are taken at face value. Agent rules never apply to human-labeled changes. |

A rule "targets agents" when its `actors` include `agent`, or when it has no
`actors` and filters on `tools`, `toolsNotIn`, or `models`. Use `trusted` only
when every key holder is trusted not to relabel agent work.

## Rule fields

| Field | Required | Description |
| --- | --- | --- |
| `id` | Yes | Lowercase identifier, unique within the policy |
| `description` | No | Shown in reports |
| `when` | Yes | Match conditions; every populated field must match |
| `effect` | Yes | `allow`, `warn`, `require-approval`, or `deny` |
| `message` | No | Overrides `description` in failure messages |
| `approvals` | With `require-approval` | Number of distinct approvers |
| `roles` | No | Approver must act in one of these roles |
| `allowSelfApproval` | No | Let the committer count as an approver (default `false`) |

## Match conditions

A rule is evaluated over **changes**: one actor touching one path. A rename
produces a change for both the old and new path, so moving a protected file
out of a protected directory still matches.

| Field | Matches when |
| --- | --- |
| `actors` | The actor kind is listed (`human`, `agent`) |
| `tools` | The actor's tool is listed |
| `toolsNotIn` | The actor is an agent whose tool is **not** listed (allowlisting) |
| `models` | The declared model is listed |
| `paths` | The path matches any glob (`**` crosses directories) |
| `exclude` | The path matches none of these globs |
| `files: { gt: n }` | More than *n* distinct paths matched |
| `linesAdded: { gt: n }` | More than *n* lines were added across matched changes |

## Effects

| Effect | At commit | At the gate |
| --- | --- | --- |
| `allow` | Recorded | Pass |
| `warn` | Recorded and printed | Pass with warning |
| `require-approval` | Recorded and printed | Fail until approvals are satisfied |
| `deny` | Commit refused unless `--force` (the override is ledgered) | Fail |

The strongest effect wins: `deny` > `require-approval` > `warn` > `allow`.

## Recipes

Approved tools only:

```yaml
- id: approved-tools
  when: { toolsNotIn: [codex, gemini, aider] }
  effect: deny
  message: Only tools approved by the AI review board may contribute.
```

Two-person rule for flight software:

```yaml
- id: flight-software
  when:
    paths: ["flight/**"]
  effect: require-approval
  approvals: 2
  roles: [flight-safety]
```

Large agent change sets:

```yaml
- id: agent-blast-radius
  when: { actors: [agent], files: { gt: 40 } }
  effect: require-approval
  approvals: 1
```
