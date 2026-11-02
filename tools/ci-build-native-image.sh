#!/usr/bin/env bash
# Build only public tooling. This helper does not log in or publish.
set -euo pipefail
cd "$(dirname "$0")/.."
image=${1:?usage: ci-build-native-image.sh LOCAL_IMAGE_TAG [native|toolchain]}
target=${2:-native}
case "$target" in native|toolchain) ;; *) echo 'Invalid image target' >&2; exit 1 ;; esac
build_args=(--target "$target")
if [[ "$target" == toolchain ]]; then
  versions=$(python3 tools/ci-toolchain-versions.py)
  read -r go_version wails_version <<< "$versions"
  # Official release metadata authenticates the downloaded archive over TLS.
  go_sha=$(python3 - "$go_version" <<'PYTHON'
import json, sys, urllib.request
version = 'go' + sys.argv[1]
with urllib.request.urlopen('https://go.dev/dl/?mode=json&include=all', timeout=60) as response:
    releases = json.load(response)
checksums = [f['sha256'] for release in releases if release['version'] == version
             for f in release['files'] if f['filename'] == version + '.linux-amd64.tar.gz']
if len(checksums) != 1:
    raise SystemExit('Missing unique official Go archive checksum')
print(checksums[0])
PYTHON
  )
  build_args+=(--build-arg "GO_VERSION=$go_version" --build-arg "GO_SHA256=$go_sha" --build-arg "WAILS_VERSION=$wails_version")
fi
# Optional session CA is mounted only during networked build steps. It is not
# included in the public image or build context.
if [[ -n ${CODEX_PROXY_CERT:-} ]]; then
  build_args+=(--secret "id=proxy_ca,src=$CODEX_PROXY_CERT")
fi
# Docker's predefined proxy arguments are excluded from image history/config;
# do not declare proxy ARG/ENV in the Dockerfile or copy local certificates.
proxy_args=()
for proxy_name in HTTP_PROXY HTTPS_PROXY NO_PROXY http_proxy https_proxy no_proxy; do
  if [[ -v $proxy_name ]]; then
    proxy_args+=(--build-arg "$proxy_name")
  fi
done
docker build --pull --platform linux/amd64 "${build_args[@]}" "${proxy_args[@]}" -t "$image" build/ci
# Exercise the image's default USER and an Actions-like mounted home/tool cache
# as UID 1001. The host directories are disposable, never a runner snapshot.
probe=$(mktemp -d)
trap 'rm -rf "$probe"' EXIT
mkdir -p "$probe/home" "$probe/work/tools" "$probe/temp" "$probe/toolcache"
if [[ "$target" == toolchain ]]; then
  cp go.mod "$probe/work/go.mod"
  cp tools/ci-use-toolchain.sh tools/ci-toolchain-versions.py "$probe/work/tools/"
fi
# GitHub hosted Ubuntu creates these as runner (1001:1001). Local builders
# may have a different UID; change only these freshly created test directories.
if [[ $(id -u) != 1001 ]]; then
  docker run --rm --user 0:0 -v "$probe:/probe" "$image" chown -R 1001:1001 /probe/home /probe/work /probe/temp /probe/toolcache
fi
docker run --rm --user 1001:1001 \
  -e HOME=/github/home -e GITHUB_WORKSPACE=/__w/repo/repo \
  -e RUNNER_TEMP=/__w/_temp -e RUNNER_TOOL_CACHE=/__t \
  -v "$probe/home:/github/home" -v "$probe/work:/__w/repo/repo" \
  -v "$probe/temp:/__w/_temp" -v "$probe/toolcache:/__t" \
  "$image" bash -euo pipefail -c '
    nestworth-ci-verify-native
    if [[ -x /opt/nestworth-toolchain/go/bin/go ]]; then
      export GOMODCACHE=/tmp/nestworth-ci-go-modules GOCACHE=/tmp/nestworth-ci-go-build
      nestworth-ci-verify-native --with-toolchains
      /opt/nestworth-toolchain/go/bin/go version
      /opt/nestworth-toolchain/bin/wails3 version
      cat /opt/nestworth-toolchain/versions.txt
    fi
    for directory in "$HOME" "$GITHUB_WORKSPACE" "$RUNNER_TEMP" "$RUNNER_TOOL_CACHE"; do
      marker=$(mktemp "$directory/.ci-write.XXXXXX")
      rm "$marker"
    done
    wc -l /usr/local/share/nestworth-ci/installed-packages.tsv
    cat /usr/local/share/nestworth-ci/ubuntu.sources
    if [[ -x /opt/nestworth-toolchain/go/bin/go ]]; then
      cd "$GITHUB_WORKSPACE"
      export PREINSTALLED=true GITHUB_PATH="$RUNNER_TEMP/path" GITHUB_OUTPUT="$RUNNER_TEMP/output"
      bash tools/ci-use-toolchain.sh activate-go
      bash tools/ci-use-toolchain.sh verify-go
      bash tools/ci-use-toolchain.sh wails
      # Regression: updated project versions must reject the old image without
      # reaching setup-go, an auto toolchain download or CLI compilation.
      cp go.mod go.mod.original
      sed -i -E "s/^go [0-9.]+/go 1.99.1/" go.mod
      if bash tools/ci-use-toolchain.sh activate-go > "$RUNNER_TEMP/mismatch" 2>&1; then
        echo "Go mismatch incorrectly accepted" >&2; exit 1
      fi
      grep -q "Image Go does not match go.mod" "$RUNNER_TEMP/mismatch"
      cp go.mod.original go.mod
      sed -i -E "s|(github.com/wailsapp/wails/v3 )[^[:space:]]+|\\1v3.99.1|" go.mod
      if bash tools/ci-use-toolchain.sh wails > "$RUNNER_TEMP/mismatch" 2>&1; then
        echo "Wails mismatch incorrectly accepted" >&2; exit 1
      fi
      grep -q "Wails CLI does not match go.mod" "$RUNNER_TEMP/mismatch"
      echo "Pinned toolchain match and Go/Wails mismatch smoke passed"
    fi
  '
