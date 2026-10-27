#!/usr/bin/env bash
# Shared by the image build and check jobs; no package installation here.
set -euo pipefail

if ((EUID == 0)); then
  echo '::error::CI must run as an unprivileged user to preserve permission tests' >&2
  exit 1
fi
for tool in bash gcc g++ make pkg-config timeout git tar gzip unzip python3 \
  curl file find grep sed awk ps sudo xz zstd; do
  command -v "$tool" >/dev/null
done
timeout --version | grep -q 'GNU coreutils'
pkg-config --modversion gtk4 webkitgtk-6.0 libsoup-3.0

# This catches root/capability changes that would invalidate the existing
# TestMetadataDeletionFailureDoesNotBreakDisable assertion.
probe=$(mktemp -d)
trap 'chmod 700 "$probe"; rm -rf "$probe"' EXIT
chmod 500 "$probe"
if touch "$probe/must-not-be-writable" 2>/dev/null; then
  echo '::error::CI user can bypass a read-only directory' >&2
  exit 1
fi
chmod 700 "$probe"

if [[ ${1:-} == --with-toolchains ]]; then
  go version
  [[ $(go env CGO_ENABLED) == 1 ]]
  # setup-go uses os-release in its cache key, separating Ubuntu 26.04
  # containers from other distributions. Never tar the runner filesystem.
  for directory in "$HOME" "$GITHUB_WORKSPACE" "$RUNNER_TEMP" \
    "$RUNNER_TOOL_CACHE" "$(go env GOCACHE)" "$(go env GOMODCACHE)"; do
    mkdir -p "$directory"
    marker=$(mktemp "$directory/.nestworth-ci-write.XXXXXX")
    rm "$marker"
  done
fi
