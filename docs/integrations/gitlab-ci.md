# GitLab CI

```yaml
sigil-gate:
  stage: test
  image: ghcr.io/ryan-tobin/sigil:latest
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  variables:
    GIT_DEPTH: 0
  script:
    - git fetch origin "+refs/heads/${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}:refs/remotes/origin/${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}"
    - git fetch origin "+refs/notes/sigil:refs/notes/sigil" || true
    - >
      sigil gate
      --range "origin/${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}..HEAD"
      --config-from "origin/${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}"
      --format markdown | tee sigil-gate.md
  artifacts:
    when: always
    paths: [sigil-gate.md]
```

Require the job in **Settings → Merge requests → Pipelines must succeed**.
