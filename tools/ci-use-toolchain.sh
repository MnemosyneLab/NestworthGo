#!/usr/bin/env bash
# Select image-owned binaries explicitly and fail closed on version drift.
set -euo pipefail
cd "$(dirname "$0")/.."
versions=$(python3 tools/ci-toolchain-versions.py)
read -r go_version wails_version <<< "$versions"
case ${1:?usage: ci-use-toolchain.sh activate-go|verify-go|wails} in
  activate-go)
    test "${PREINSTALLED:-false}" = true
    binary=/opt/nestworth-toolchain/go/bin/go
    test -x "$binary" || { echo '::error::Pinned toolchain image is missing Go' >&2; exit 1; }
    test "$(GOTOOLCHAIN=local "$binary" env GOVERSION)" = "go$go_version" || {
      echo '::error::Image Go does not match go.mod; rebuild and review its digest' >&2; exit 1;
    }
    printf '%s\n' /opt/nestworth-toolchain/go/bin >> "${GITHUB_PATH:?}"
    ;;
  verify-go)
    if [[ ${PREINSTALLED:-false} == true ]]; then
      test "$(command -v go)" = /opt/nestworth-toolchain/go/bin/go
    fi
    test "$(GOTOOLCHAIN=local go env GOVERSION)" = "go$go_version" || {
      echo '::error::Selected Go does not match go.mod' >&2; exit 1;
    }
    go version
    printf 'go-version=%s\n' "$go_version" >> "${GITHUB_OUTPUT:?}"
    ;;
  wails)
    if [[ ${PREINSTALLED:-false} == true ]]; then
      binary=/opt/nestworth-toolchain/bin/wails3
      test -x "$binary" || { echo '::error::Pinned toolchain image is missing Wails CLI' >&2; exit 1; }
    else
      # Legacy native digest/host rollback. Never replace a mismatched image
      # toolchain silently; only this explicit fallback may build the CLI.
      binary=/tmp/nestworth-ci-wails/wails3
      if [[ ! -x "$binary" ]] || [[ $("$binary" version 2>&1) != "$wails_version" ]]; then
        GOBIN=/tmp/nestworth-ci-wails go install "github.com/wailsapp/wails/v3/cmd/wails3@${wails_version}"
      fi
    fi
    test "$("$binary" version 2>&1)" = "$wails_version" || {
      echo '::error::Wails CLI does not match go.mod; rebuild and review its digest' >&2; exit 1;
    }
    printf '%s\n' "$(dirname "$binary")" >> "${GITHUB_PATH:?}"
    ;;
  *) echo 'Unknown toolchain action' >&2; exit 1 ;;
esac
