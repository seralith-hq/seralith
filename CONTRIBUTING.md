# Contributing to Sigil

Thank you for helping build Sigil. This document describes how we work so that
your contribution can be reviewed and merged quickly.

## Ground rules

- Be respectful. All participation is governed by our [Code of Conduct](CODE_OF_CONDUCT.md).
- Report security issues privately. See [SECURITY.md](SECURITY.md). Never open a
  public issue for a vulnerability.
- Discuss large changes first. Open an issue or a design proposal under
  `docs/rfcs/` before investing in a significant change.

## Development setup

Requirements:

| Tool | Version |
| --- | --- |
| Go | 1.26+ |
| Node.js | 26+ |
| pnpm | 9+ |
| git | 2.40+ |

```sh
git clone https://github.com/ryan-tobin/new-startup.git sigil
cd sigil
make bootstrap   # installs web dependencies
make build       # builds the web workbench and the sigil binaries into ./bin
make test        # runs Go and web test suites
```

Useful targets:

| Target | Description |
| --- | --- |
| `make build` | Build `bin/sigil` and `bin/sigil-control` |
| `make test` | Run all tests with the race detector |
| `make lint` | Run `golangci-lint`, `go vet`, and the web linters |
| `make fmt` | Format Go and TypeScript sources |
| `make web` | Build the workbench UI into the embedded asset directory |
| `make dev` | Run the daemon and the Vite dev server with hot reload |

## Branching and commits

- Branch from `main` using `<type>/<short-description>`, for example
  `feat/policy-approvals` or `fix/ledger-verify-offset`.
- We follow [Conventional Commits](https://www.conventionalcommits.org/):

  ```
  <type>(<scope>): <summary in imperative mood, max 72 chars>

  <body explaining what and why, wrapped at 72 columns>

  <footer: Refs #123, BREAKING CHANGE: ...>
  ```

  Types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`,
  `chore`, `revert`. Scopes match top-level packages: `cli`, `core`, `ledger`,
  `policy`, `workspace`, `session`, `daemon`, `web`, `control`, `site`, `docs`.
- Keep commits focused. Each commit should build and pass tests.
- Sign your commits (`git commit -S`). Sigil is a provenance company; we hold
  ourselves to the same standard.

## Pull requests

1. Fill out the pull request template completely.
2. Keep PRs under roughly 600 changed lines where practical.
3. Include tests for new behavior and regressions.
4. Update documentation and `CHANGELOG.md` under `Unreleased`.
5. A PR requires one approving review from a code owner and a green CI run.
6. We merge with merge commits to preserve the attested history of each change.

## Code standards

### Go

- `gofmt` and `goimports` formatting is mandatory.
- Follow [Effective Go](https://go.dev/doc/effective_go) and the
  [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).
- Return errors, never panic in library code. Wrap with `fmt.Errorf("...: %w", err)`.
- Exported identifiers are documented when their purpose is not self-evident.
  Avoid narrating comments inside function bodies.
- Packages under `internal/` are not a public API. Public types live in `pkg/`.

### TypeScript

- Strict mode is on. No `any` without a justification in review.
- Function components and hooks only. Co-locate component styles.
- Run `pnpm -C web lint` and `pnpm -C web typecheck` before pushing.

## Licensing

By contributing you agree that your contributions to directories outside `ee/`
are licensed under the [Apache License 2.0](LICENSE). Contributions to `ee/`
require a signed contributor agreement; contact legal@sigil.systems.
