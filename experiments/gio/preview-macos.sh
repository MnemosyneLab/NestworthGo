#!/bin/sh
set -eu

here=$(cd "$(dirname "$0")" && pwd -P)
repo=$(cd "$here/../.." && pwd -P)
run_dir=$(mktemp -d "${TMPDIR:-/tmp}/nestworth-gio.XXXXXXXX")
run_dir=$(cd "$run_dir" && pwd -P)
bundle="$run_dir/Nestworth.app"
contents="$bundle/Contents"

export NESTWORTH_GIO_RUN_DIR="$run_dir"
export NESTWORTH_DATABASE_PATH="$run_dir/data/nestworth.db"
export NESTWORTH_SETTINGS_PATH="$run_dir/data/settings.json"
export SDKROOT="${SDKROOT:-$(xcrun --sdk macosx --show-sdk-path)}"
export MACOSX_DEPLOYMENT_TARGET=27.0
export CGO_CFLAGS="-O2 -g -mmacosx-version-min=27.0"
export CGO_CXXFLAGS="$CGO_CFLAGS"
export CGO_LDFLAGS="-O2 -g -mmacosx-version-min=27.0"

printf 'Preparing isolated Gio preview in %s\n' "$run_dir"
mkdir -p "$contents/MacOS" "$contents/Resources"
cp "$repo/build/darwin/Info.dev.plist" "$contents/Info.plist"
cp "$repo/assets/icons/icon.icns" "$contents/Resources/icon.icns"
cp "$repo/frontend/src/assets/brand/wordmark.png" "$contents/Resources/wordmark.png"
cp "$repo/assets/brand/favicon.png" "$contents/Resources/mark.png"
(
	cd "$here"
	go build -mod=readonly -trimpath -o "$contents/MacOS/Nestworth" .
)

plist="$contents/Info.plist"
for key in CFBundleExecutable CFBundleName CFBundleDisplayName; do
	[ "$(/usr/libexec/PlistBuddy -c "Print :$key" "$plist")" = 'Nestworth' ]
done
[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$plist")" = 'com.nestworth.dev' ]
codesign --force --sign - "$bundle"
"$contents/MacOS/Nestworth" --prepare-pilot-fixture
"$contents/MacOS/Nestworth" --verify-pilot-fixture
open -n -a "$bundle" \
	--env "NESTWORTH_GIO_RUN_DIR=$NESTWORTH_GIO_RUN_DIR" \
	--env "NESTWORTH_DATABASE_PATH=$NESTWORTH_DATABASE_PATH" \
	--env "NESTWORTH_SETTINGS_PATH=$NESTWORTH_SETTINGS_PATH" \
	--stdout "$run_dir/stdout.log" --stderr "$run_dir/stderr.log"
