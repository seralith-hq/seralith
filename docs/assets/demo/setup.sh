#!/usr/bin/env bash
# Source this from a demo tape. It creates a throwaway git repository and a
# new signing key under /tmp, then leaves the shell inside that repository.
# The key is generated at record time and is not a real credential.

if [[ ${BASH_SOURCE[0]} == "$0" ]]; then
  echo "source this script from a demo tape" >&2
  exit 1
fi

_demo_prepare() {
  set -euo pipefail
  local demo_dir demo_root repo wt bin
  demo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
  demo_root=$(mktemp -d /tmp/sl-demo.XXXXXX)
  repo=$demo_root/repo
  wt=$demo_root/wt
  bin=$demo_root/bin

  export SERALITH_DATA_DIR=$demo_root/data
  export SERALITH_CONFIG_DIR=$demo_root/config
  export SERALITH_KEY=$demo_root/config/keys/default.pem
  export SERALITH_PRINCIPAL=dev@example.com
  export GIT_CONFIG_GLOBAL=$demo_root/gitconfig
  export GIT_CONFIG_NOSYSTEM=1

  mkdir -p "$bin" "$repo" "$wt" "$demo_root/data" "$demo_root/config"
  cp "$demo_dir/append-delay.sh" "$demo_dir/write-retry.sh" "$demo_dir/write-token.sh" "$demo_dir/show-note" "$bin/"
  chmod +x "$bin"/*

  git config --global user.name "Dev Example"
  git config --global user.email "dev@example.com"
  git config --global init.defaultBranch main

  cd "$repo"
  git init -q -b main
  printf 'payments service\n' > README.md
  git add README.md
  git commit -q -m "init"

  seralith init --name payments --roles maintainer >/dev/null
  python3 - "$repo" "$wt" <<'PY'
import pathlib, sys
repo, wt = sys.argv[1], sys.argv[2]
path = pathlib.Path(repo, ".seralith", "config.yaml")
text = path.read_text()
needle = "workspaces:\n"
if needle not in text:
    raise SystemExit("seralith init did not write a workspaces block")
text = text.replace(needle, f"{needle}    root: {wt}\n", 1)
path.write_text(text)
PY
  git add .seralith
  git commit -q -m "chore: enable seralith"

  cat > "$demo_root/env.sh" <<EOF
export SERALITH_DATA_DIR=$(printf '%q' "$SERALITH_DATA_DIR")
export SERALITH_CONFIG_DIR=$(printf '%q' "$SERALITH_CONFIG_DIR")
export SERALITH_KEY=$(printf '%q' "$SERALITH_KEY")
export SERALITH_PRINCIPAL=$(printf '%q' "$SERALITH_PRINCIPAL")
export GIT_CONFIG_GLOBAL=$(printf '%q' "$GIT_CONFIG_GLOBAL")
export GIT_CONFIG_NOSYSTEM=1
export DEMO_BIN=$(printf '%q' "$bin")
cd $(printf '%q' "$repo")
EOF
  _demo_env=$demo_root/env.sh
}

_demo_prepare
# shellcheck disable=SC1090
source "$_demo_env"
export PS1='payments $ '
unset PROMPT_COMMAND
set +e
set +u
set +o pipefail
# Drop the setup command from the scrollback before the recording resumes.
clear
