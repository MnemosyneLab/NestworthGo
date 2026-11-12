#!/usr/bin/env bash
# Install only into the runner's dedicated cache, never a global prefix.
set -euo pipefail
version=12.19.0
node_version=$(node --version)
test "$node_version" = v24.22.3
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
# pnpm 12's --ignore-scripts placeholder intentionally has no shebang. Bash
# retries it as shell code, but Task/Go subprocess execution returns ENOEXEC.
# Keep scripts disabled and expose an executable shim for the official Node entry.
launcher_dir="$prefix/bin"
mkdir -p "$launcher_dir"
cat > "$launcher_dir/pnpm" <<'PNPM_LAUNCHER'
#!/bin/sh
exec node "$(dirname "$0")/../node_modules/pnpm/bin/pnpm.mjs" "$@"
PNPM_LAUNCHER
chmod +x "$launcher_dir/pnpm"
test "$("$launcher_dir/pnpm" --version)" = "$version"
printf '%s\n' "$launcher_dir" >> "$GITHUB_PATH"
