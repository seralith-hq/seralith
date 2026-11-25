# GitHub Actions

Protect a branch by gating every pull request.

```yaml
# .github/workflows/seralith.yml
name: Seralith
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
      - uses: seralith-hq/seralith@v0
        with:
          version: latest
```

Then mark the **Seralith** check as required in your branch protection rules.

The action:

1. Installs `seralith` and verifies its checksum.
2. Fetches the base branch and `refs/notes/seralith`.
3. Runs `seralith gate --range origin/<base>..HEAD --config-from origin/<base>`.

`--config-from` loads policy and trust from the **base branch**, so a pull
request cannot loosen the rules it is judged by. Results appear as annotations
and in the job summary.

## Publishing attestations

Developers publish attestations with `seralith sync push` before or after
`git push`. To automate it, add a pre-push hook:

```sh
# .git/hooks/pre-push
#!/bin/sh
exec seralith sync push --remote "$1"
```

## Uploading to Seralith Control

```yaml
      - run: seralith cloud push --range origin/${{ github.base_ref }}..HEAD
        env:
          SERALITH_CLOUD_URL: https://control.example.com
          SERALITH_CLOUD_TOKEN: ${{ secrets.SERALITH_CLOUD_TOKEN }}
          SERALITH_CLOUD_PROJECT: my-service
```

Create a `ci` member in Seralith Control to issue a token that can upload but not
administer.
