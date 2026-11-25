#!/bin/sh
set -eu

if [ "$(uname -s)" != Darwin ]; then
	printf 'Run this launcher on macOS.\n' >&2
	exit 1
fi
here=$(CDPATH='' cd "$(dirname "$0")" && pwd -P)
export SDKROOT="${SDKROOT:-$(xcrun --sdk macosx --show-sdk-path)}"
export MACOSX_DEPLOYMENT_TARGET=27.0
export CGO_CFLAGS="-O2 -g -mmacosx-version-min=27.0"
export CGO_CXXFLAGS="$CGO_CFLAGS"
export CGO_LDFLAGS="-O2 -g -mmacosx-version-min=27.0"

exec python3 "$here/packaging/preview.py" "$@"
