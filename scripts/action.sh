#!/usr/bin/env bash
# Install gobunshin and run it in a GitHub Actions workspace.
set -euo pipefail

version="${GOBUNSHIN_VERSION:-}"
args="${GOBUNSHIN_ARGS:-release}"
workdir="${GOBUNSHIN_WORKDIR:-.}"

if [[ -n "${version}" ]]; then
  echo "installing gobunshin ${version}"
  go install "github.com/karolsko/gobunshin/cmd/gobunshin@${version}"
else
  if [[ -z "${GITHUB_ACTION_PATH:-}" ]]; then
    echo "GITHUB_ACTION_PATH is empty; pass version or run this script from the action" >&2
    exit 1
  fi
  echo "installing gobunshin from the action checkout"
  (cd "${GITHUB_ACTION_PATH}" && go install ./cmd/gobunshin)
fi

export PATH="$(go env GOPATH)/bin:${PATH}"

if ! command -v gobunshin >/dev/null 2>&1; then
  echo "gobunshin is not on PATH after installation" >&2
  exit 1
fi

if [[ "${workdir}" == /* ]]; then
  cd "${workdir}"
else
  cd "${GITHUB_WORKSPACE:-$PWD}/${workdir}"
fi

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  shallow="$(git rev-parse --is-shallow-repository 2>/dev/null || echo false)"
  if [[ "${shallow}" == "true" ]]; then
    echo "fetching full git history so tags and the changelog are complete"
    git fetch --unshallow --tags origin
  fi
fi

if [[ "${GITHUB_REF_TYPE:-}" == "tag" ]]; then
  echo "tag ${GITHUB_REF_NAME:-}"
fi

echo "gobunshin ${args}"
# Word splitting is intentional. Shell metacharacters in args are not evaluated.
# shellcheck disable=SC2086
gobunshin ${args}
