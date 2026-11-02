#!/usr/bin/env bash
# Install only into the runner's dedicated cache, never a global prefix.
set -euo pipefail
version=12.14.0
node_version=$(node --version)
test "$node_version" = v24.22.0
cache="$(dirname "$RUNNER_TEMP")/nestworth-ci-cache-v1"
prefix="$cache/pnpm-cli-darwin-arm64-$node_version-$version"
binary="$prefix/node_modules/.bin/pnpm"
if [ -x "$binary" ] && [ "$("$binary" --version)" = "$version" ]; then
  echo "Reusing pinned pnpm $version"
else
  echo "Bootstrapping pinned pnpm $version into the dedicated runner cache"
  npm install --prefix "$prefix" --cache "$cache/npm-bootstrap" --ignore-scripts --no-audit --no-fund --package-lock=false "pnpm@$version"
fi
test "$("$binary" --version)" = "$version"
printf '%s\n' "$(dirname "$binary")" >> "$GITHUB_PATH"
