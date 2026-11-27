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
| `unverified` (default) | A rule that targets agents and names `paths` also applies to human-labeled changes to those paths. A `deny` becomes a requirement for one independent approval, so people can still edit, for example, pipeline definitions. Rules without `paths`, such as line-count thresholds, apply to agents only. An omitted `humanLabels` field MUST be evaluated as `unverified`. |
| `trusted` | Human labels are taken at face value. Agent rules never apply to human-labeled changes. |

A rule "targets agents" when its `actors` include `agent`, or when it has no
`actors` and filters on `tools`, `toolsNotIn`, or `models`. Use `trusted` only
when every key holder is trusted not to relabel agent work.

## Rule fields

| Field | Required | Description |
| --- | --- | --- |
| `id` | Yes | Unique in the policy. MUST match `^[a-z0-9][a-z0-9._-]{0,63}$` (1 to 64 characters) |
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
| `paths` | The path matches any glob ([Globs](#globs)) |
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

Every matching rule produces a decision, and the gate enforces each of them.
`seralith commit` uses the strongest effect only as the commit outcome:
`deny` > `require-approval` > `warn` > `allow`. Only `deny` refuses the
commit. The gate does not drop a weaker decision because a stronger one
also matched. The procedure is [Evaluation](#evaluation).

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

## Syntax

A policy document is YAML. `version` MUST be `1`. An omitted `version` is
`0` and MUST be rejected. `humanLabels` MUST be omitted, `unverified`, or
`trusted`. Any other value MUST be rejected.

The decoder rejects a field it does not know, at every level of the
document. This file is rejected:

```yaml
version: 1
rules:
  - id: a
    when: {colour: red}
    effect: warn
```

`seralith policy lint` exits 1 and prints:

```
error: policy: parse: yaml: unmarshal errors:
  line 4: field colour not found in type policy.Match
```

A duplicate rule id MUST be rejected. `effect` MUST be `allow`, `warn`,
`deny`, or `require-approval`. `require-approval` MUST set `approvals` to
an integer greater than or equal to 1. `actors` values MUST be `human` or
`agent`. Each glob in `paths` and `exclude` MUST be a valid doublestar
pattern, as [Globs](#globs) defines.

The SHA-256 of the policy file bytes, lowercase hex, is the policy digest
recorded on a decision set. A policy loaded because the file was missing
has an empty digest and no rules.

## Globs

A path matches `paths` when `doublestar.Match` from
`github.com/bmatcuk/doublestar/v4` v4.10.7 returns true for any pattern.
The pattern MUST match the whole path. The separator is `/`. Matching is
case-sensitive.

| Pattern | Matches |
| --- | --- |
| `*` | Any run of characters other than `/` |
| `?` | One character other than `/` |
| `**` as its own path segment | Zero or more segments. `/**/` matches zero or more directories |
| `**` not as its own segment | The same as `*` for that segment. `**.txt` does not cross directories |
| `{alt1,alt2}` | One of the alternatives, which may be empty |
| `[abc]`, `[a-z]`, `[^abc]`, `[!abc]` | One character other than `/`, in or outside the class |
| `\x` | A literal special character |

`exclude` is tested first. A path that matches any exclude pattern does not
match, even when a `paths` pattern also matches. When `paths` is empty, every
path that is not excluded matches.

## Evaluation

A rule is evaluated over the change list from the
[attestation diff](attestation.md#diff): one actor and one path, plus added
lines. A rename contributes the new path and the old path. The old path
carries no added lines.

For each rule, in order, keep the changes whose path matches
[Globs](#globs). A change is **direct** when its actor matches every
populated actor field (`actors`, `tools`, `toolsNotIn`, `models`). It is
**extended** when it is not direct and all of the following hold:

- `humanLabels` is not `trusted` (omitted and `unverified` both extend),
- the actor's kind is `human`,
- the rule's `paths` list is non-empty, and
- the rule targets agents: `actors` contains `agent`, or `actors` is empty
  and at least one of `tools`, `toolsNotIn`, and `models` is non-empty.

Added lines from direct and extended changes are summed, including two
changes that name the same path. The path set used for `files` is the
distinct direct paths plus the distinct extended paths that are not also
direct. A rule matches only when that set is non-empty and both thresholds
pass. An absent `files` or `linesAdded` always passes. `{ gt: n }` passes
only when the count is greater than *n*. A count equal to *n* does not
match. Extended paths and their added lines count toward both thresholds.

The decision keeps the rule's `approvals` and `roles`. Its message is
`message`, or `description` when `message` is empty. Its paths are sorted
by byte order. The effect is the rule's effect, except for one case:

- When the effect is `deny` and at least one path is direct, the effect
  stays `deny` and the paths are the direct paths only.
- When the effect is `deny` and every matching path is extended, the effect
  becomes `require-approval`, `approvals` becomes 1, the roles stay the
  rule's roles, and the message becomes `Human-labeled change to a path this rule protects from agents; human labels are not verified, so it needs an independent approval.`
  `allowSelfApproval` on that rule still applies when the gate counts
  approvers.
- Otherwise the paths are the direct and extended paths together.

## Loading

The gate reads the policy it was given. With no `--config-from`,
`seralith gate` and `seralith verify` read `.seralith/policy.yaml` from the
working tree. A missing file is version 1, no rules, and omitted
`humanLabels`. With `--config-from REV`, both that policy blob and the
trust-store blob MUST exist at `REV`; see
[Policy and trust for a run](attestation.md#policy-and-trust-for-a-run).

The policy inside a change attestation is not compared to this file.

The worked attestation was produced under this file, whose SHA-256 is
`fcff4642b46a45ae0989a0ac471e508f8a41dad71b1400993b35c9f3d0e7349d`.
`humanLabels` is omitted, so it is `unverified`. `seralith policy lint`
accepted it (`1 rule`). The change to `hello.txt` matched `review-text`.

```yaml
version: 1
rules:
  - id: review-text
    description: Changes to text files need one approval.
    when:
      paths: ["**/*.txt"]
    effect: require-approval
    approvals: 1
    roles: [maintainer]
```
