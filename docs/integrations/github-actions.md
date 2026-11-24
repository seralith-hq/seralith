# GitHub Actions

Protect a branch by gating every pull request.

```yaml
# .github/workflows/sigil.yml
name: Sigil
on:
  pull_request:

permissions:
  contents: read

jobs:
  gate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0
      - uses: ryan-tobin/new-startup@v0
        with:
          version: latest
```

Then mark the **Sigil** check as required in your branch protection rules.

The action:

1. Installs `sigil` and verifies its checksum.
2. Fetches the base branch and `refs/notes/sigil`.
3. Runs `sigil gate --range origin/<base>..HEAD --config-from origin/<base>`.

`--config-from` loads policy and trust from the **base branch**, so a pull
request cannot loosen the rules it is judged by. Results appear as annotations
and in the job summary.

## Publishing attestations

Developers publish attestations with `sigil sync push` before or after
`git push`. To automate it, add a pre-push hook:

```sh
# .git/hooks/pre-push
#!/bin/sh
exec sigil sync push --remote "$1"
```

## Uploading to Sigil Control

```yaml
      - run: sigil cloud push --range origin/${{ github.base_ref }}..HEAD
        env:
          SIGIL_CLOUD_URL: https://control.example.com
          SIGIL_CLOUD_TOKEN: ${{ secrets.SIGIL_CLOUD_TOKEN }}
          SIGIL_CLOUD_PROJECT: my-service
```

Create a `ci` member in Sigil Control to issue a token that can upload but not
administer.
