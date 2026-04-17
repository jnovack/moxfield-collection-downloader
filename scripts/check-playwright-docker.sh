#!/usr/bin/env sh
set -eu

dockerfile="build/package/Dockerfile"

if [ ! -f "${dockerfile}" ]; then
  echo "missing ${dockerfile}" >&2
  exit 1
fi

if ! grep -q "go list -m -f '{{.Version}}' github.com/playwright-community/playwright-go" "${dockerfile}"; then
  echo "Dockerfile must resolve playwright-go version from go.mod graph via go list -m." >&2
  exit 1
fi

# This is intentional, I do not WANT to expand it, I want the literal.
# shellcheck disable=SC2016
if ! grep -q 'go install "github.com/playwright-community/playwright-go/cmd/playwright@${PW_GO_VERSION}"' "${dockerfile}"; then
  echo "Dockerfile must install playwright CLI using the resolved PW_GO_VERSION variable." >&2
  exit 1
fi

if grep -Eq 'playwright-go/cmd/playwright@v[0-9]+\.[0-9]+\.[0-9]+' "${dockerfile}"; then
  echo "Dockerfile contains a hardcoded Playwright CLI version; use module-derived version instead." >&2
  exit 1
fi

echo "Playwright Docker version guard passed."
