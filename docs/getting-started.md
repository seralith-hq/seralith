# Getting started

This guide takes you from a fresh repository to a gated, attested pull request
in about ten minutes.

## Install

```sh
git clone https://github.com/ryan-tobin/new-startup.git sigil && cd sigil
make bootstrap release-build
sudo install bin/sigil /usr/local/bin/
sigil version
```

Requirements: git 2.40+, Linux or macOS. Building from source requires Go 1.26+
and Node.js 26+ with pnpm.

## 1. Initialize a repository

```sh
cd ~/code/your-service
sigil init
git add .sigil && git commit -m "chore: enable sigil"
```

`sigil init` creates three files that you commit:

| File | Purpose |
| --- | --- |
| `.sigil/config.yaml` | Project name, workspace bootstrap, custom agent profiles |
| `.sigil/policy.yaml` | Rules the gate enforces; start from the secure defaults |
| `.sigil/trust.yaml` | Public keys allowed to sign attestations and approvals, with roles |

Your private key is created at `~/.config/sigil/keys/default.pem` and never
leaves your machine.

## 2. Work in parallel

Create one workspace per task. Each is a git worktree on its own branch, so any
number of agents and humans can work at once without stepping on each other.

```sh
sigil ws new payments-retry
sigil ws new docs-refresh

sigil run --ws payments-retry --agent codex
sigil run --ws docs-refresh --agent gemini --model pro
```

Or open the workbench and run them side by side in the browser:

```sh
sigil serve --open
```

Edits you make yourself, in any editor, inside the workspace directory are
attributed to you automatically:

```sh
cd "$(sigil ws path payments-retry)"
$EDITOR internal/payments/retry.go
```

## 3. Review what happened

```sh
sigil status --ws payments-retry
```

```
SEGMENT       KIND     ACTOR                FILES    LINES
ses_1a20ec31  session  agent:codex          4 files  +212 −31
seg_1a20ed90  manual   human:you@corp.com   1 file   +6 −2

attribution agent 198 (94%) · human 12 (6%)
policy
  require-approval   security-critical-review (1 approval from security)
```

## 4. Commit with an attestation

```sh
sigil commit --ws payments-retry -m "feat(payments): retry failed captures"
sigil sync push
git push origin sigil/payments-retry
```

The commit carries a signed in-toto attestation in `refs/notes/sigil`
recording every segment, line-level attribution, and the policy decisions in
effect.

## 5. Approve and gate

A reviewer holding the required role approves the change:

```sh
sigil approve sigil/payments-retry --role security -m "retry backoff reviewed"
sigil sync push
```

CI verifies the whole branch and fails closed:

```sh
sigil gate --base origin/main --fetch --format github
```

## 6. Answer "who wrote this?"

```sh
sigil blame internal/payments/retry.go --summary
# 212 lines · human 14 (6%) · agent 198 (93%)
```

## 7. Hand evidence to an assessor

```sh
sigil evidence export --range v1.4.0..v1.5.0 -o evidence-v1.5.0.zip
sigil evidence verify evidence-v1.5.0.zip
```

## Next steps

- Tune [.sigil/policy.yaml](spec/policy.md) for your codebase
- Add the [GitHub Action](integrations/github-actions.md) to protect `main`
- Read the [threat model](security/threat-model.md) to understand guarantees
