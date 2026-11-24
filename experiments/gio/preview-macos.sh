#!/bin/sh
set -eu

here=$(cd "$(dirname "$0")" && pwd -P)
repo=$(cd "$here/../.." && pwd -P)
run_dir=$(mktemp -d "${TMPDIR:-/tmp}/nestworth-gio.XXXXXXXX")
run_dir=$(cd "$run_dir" && pwd -P)
bundle="$run_dir/Nestworth.app"
contents="$bundle/Contents"

export NESTWORTH_DATABASE_PATH="$run_dir/data/nestworth.db"
export NESTWORTH_SETTINGS_PATH="$run_dir/data/settings.json"
export SDKROOT="${SDKROOT:-$(xcrun --sdk macosx --show-sdk-path)}"
export MACOSX_DEPLOYMENT_TARGET=27.0
export CGO_CFLAGS="-O2 -g -mmacosx-version-min=27.0"
export CGO_CXXFLAGS="$CGO_CFLAGS"
export CGO_LDFLAGS="-O2 -g -mmacosx-version-min=27.0"

printf 'Preparing isolated Gio preview in %s\n' "$run_dir"
(
	cd "$repo"
	NESTWORTH_QA_OUTPUT_DIR="$run_dir" NESTWORTH_QA_SCENARIO=complete \
		NESTWORTH_QA_ANCHOR=2026-07-26T00:00:00Z \
		go run ./cmd/analytics-qa-seed > "$run_dir/seed.log" 2>&1
)
# The seeder has closed this WAL-mode database. Verify the checkpointed file
# without creating or attaching to shared-memory sidecars.
household=$(
	cd "$run_dir/data"
	sqlite3 -readonly 'file:nestworth.db?mode=ro&immutable=1' 'SELECT name FROM households;'
)
if [ "$household" != 'Analytics QA' ] || [ ! -f "$NESTWORTH_SETTINGS_PATH" ]; then
	printf 'The isolated synthetic fixture could not be verified.\n' >&2
	exit 1
fi

mkdir -p "$contents/MacOS" "$contents/Resources"
cp "$repo/build/darwin/Info.dev.plist" "$contents/Info.plist"
cp "$repo/assets/icons/icon.icns" "$contents/Resources/icon.icns"
cp "$repo/frontend/src/assets/brand/wordmark.png" "$contents/Resources/wordmark.png"
cp "$repo/assets/brand/favicon.png" "$contents/Resources/mark.png"
(
	cd "$here"
	go build -trimpath -o "$contents/MacOS/Nestworth" .
)

plist="$contents/Info.plist"
for key in CFBundleExecutable CFBundleName CFBundleDisplayName; do
	[ "$(/usr/libexec/PlistBuddy -c "Print :$key" "$plist")" = 'Nestworth' ]
done
[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$plist")" = 'com.nestworth.dev' ]
codesign --force --sign - "$bundle"
open -n -a "$bundle" \
	--env "NESTWORTH_DATABASE_PATH=$NESTWORTH_DATABASE_PATH" \
	--env "NESTWORTH_SETTINGS_PATH=$NESTWORTH_SETTINGS_PATH" \
	--stdout "$run_dir/stdout.log" --stderr "$run_dir/stderr.log"
