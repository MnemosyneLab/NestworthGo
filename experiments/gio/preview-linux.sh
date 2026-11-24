#!/bin/sh
set -eu

if [ "$(uname -s)" != Linux ]; then
	printf 'Run this preview launcher on Linux.\n' >&2
	exit 1
fi

here=$(CDPATH='' cd "$(dirname "$0")" && pwd -P)
repo=$(cd "$here/../.." && pwd -P)
run_dir=$(mktemp -d "${TMPDIR:-/tmp}/nestworth-gio.XXXXXXXX")
run_dir=$(cd "$run_dir" && pwd -P)
contents="$run_dir/Nestworth.app/Contents"

export NESTWORTH_GIO_RUN_DIR="$run_dir"
export NESTWORTH_DATABASE_PATH="$run_dir/data/nestworth.db"
export NESTWORTH_SETTINGS_PATH="$run_dir/data/settings.json"

printf 'Preparing isolated Gio preview in %s\n' "$run_dir"
mkdir -p "$contents/MacOS" "$contents/Resources"
cp "$repo/build/darwin/Info.dev.plist" "$contents/Info.plist"
cp "$repo/frontend/src/assets/brand/wordmark.png" "$contents/Resources/wordmark.png"
cp "$repo/assets/brand/favicon.png" "$contents/Resources/mark.png"
(
	cd "$here"
	CGO_ENABLED=1 go build -mod=readonly -trimpath -o "$contents/MacOS/Nestworth" .
)

python3 - "$run_dir" <<'PY'
import plistlib
import sys
from pathlib import Path

run_dir = Path(sys.argv[1])
with (run_dir / "Nestworth.app/Contents/Info.plist").open("rb") as plist:
    info = plistlib.load(plist)
if info.get("CFBundleIdentifier") != "com.nestworth.dev" or any(
    info.get(key) != "Nestworth"
    for key in ("CFBundleExecutable", "CFBundleName", "CFBundleDisplayName")
):
    sys.exit("The Nestworth development identity could not be verified.")
PY

"$contents/MacOS/Nestworth" --prepare-pilot-fixture
"$contents/MacOS/Nestworth" --verify-pilot-fixture
exec "$contents/MacOS/Nestworth"
