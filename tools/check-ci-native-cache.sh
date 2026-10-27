#!/usr/bin/env bash
# Opt-in CGO cache proof. Requires Go and a C compiler; no application/data use.
set -euo pipefail
go version
[[ $(go env CGO_ENABLED) == 1 ]]
probe=$(mktemp -d /tmp/nestworth-ci-native-cache.XXXXXX)
trap 'rm -rf "$probe"' EXIT
mkdir -p "$probe/include" "$probe/src" "$probe/modules"
printf 'download-retained\n' > "$probe/modules/download-marker"
cat > "$probe/src/main.go" <<GOEOF
package main
/*
#cgo CFLAGS: -I$probe/include
#include <native.h>
*/
import "C"
import "fmt"
func main() { fmt.Println(C.NATIVE_VALUE) }
GOEOF
# Explicit synthetic paths; never clear a developer's normal Go/module cache.
export GO111MODULE=off GOMODCACHE="$probe/modules" GOCACHE="$probe/build-a"
build() {
  # Fresh CI checkouts have no prior executable. Exercise the Go build cache.
  rm -f "$probe/probe"
  go build -p 2 -o "$probe/probe" "$probe/src/main.go"
}
printf '#define NATIVE_VALUE 1\n' > "$probe/include/native.h"
build
test "$("$probe/probe")" = 1
printf '#define NATIVE_VALUE 2\n' > "$probe/include/native.h"
build
# Go may eventually track external headers; do not require stale behavior.
printf 'same-cache-after-external-header-change=%s\n' "$("$probe/probe")"
export GOCACHE="$probe/build-b"
build
test "$("$probe/probe")" = 2
echo 'distinct-image-cache=2'
export GOCACHE="$probe/build-a"
printf '#define NATIVE_VALUE 1\n' > "$probe/include/native.h"
build
test "$("$probe/probe")" = 1
echo 'rollback-image-a-cache=1'
printf '#define NATIVE_VALUE 3\n' > "$probe/include/native.h"
go clean -cache
test "$(cat "$probe/modules/download-marker")" = download-retained
build
test "$("$probe/probe")" = 3
echo 'unpinned-clean-cache=3-module-download-retained'
