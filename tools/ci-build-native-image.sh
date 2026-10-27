#!/usr/bin/env bash
# Build only public tooling. This helper does not log in or publish.
set -euo pipefail
cd "$(dirname "$0")/.."
image=${1:?usage: ci-build-native-image.sh LOCAL_IMAGE_TAG}
# Docker's predefined proxy arguments are excluded from image history/config;
# do not declare proxy ARG/ENV in the Dockerfile or copy local certificates.
proxy_args=()
for proxy_name in HTTP_PROXY HTTPS_PROXY NO_PROXY http_proxy https_proxy no_proxy; do
  if [[ -v $proxy_name ]]; then
    proxy_args+=(--build-arg "$proxy_name")
  fi
done
docker build --pull --no-cache --platform linux/amd64 "${proxy_args[@]}" -t "$image" build/ci
# Exercise the image's default USER and an Actions-like mounted home/tool cache
# as UID 1001. The host directories are disposable, never a runner snapshot.
probe=$(mktemp -d)
trap 'rm -rf "$probe"' EXIT
mkdir -p "$probe/home" "$probe/work" "$probe/temp" "$probe/toolcache"
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
    for directory in "$HOME" "$GITHUB_WORKSPACE" "$RUNNER_TEMP" "$RUNNER_TOOL_CACHE"; do
      marker=$(mktemp "$directory/.ci-write.XXXXXX")
      rm "$marker"
    done
    wc -l /usr/local/share/nestworth-ci/installed-packages.tsv
    cat /usr/local/share/nestworth-ci/ubuntu.sources
  '
