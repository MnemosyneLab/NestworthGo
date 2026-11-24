#!/bin/sh
set -eu

if [ "$(uname -s)" != Linux ]; then
	printf 'Run this preview launcher on Linux.\n' >&2
	exit 1
fi

here=$(CDPATH= cd "$(dirname "$0")" && pwd -P)
repo=$(cd "$here/../.." && pwd -P)
run_dir=$(mktemp -d "${TMPDIR:-/tmp}/nestworth-gio.XXXXXXXX")
run_dir=$(cd "$run_dir" && pwd -P)
contents="$run_dir/Nestworth.app/Contents"

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

if ! (
	cd "$repo"
	NESTWORTH_QA_OUTPUT_DIR="$run_dir" NESTWORTH_QA_SCENARIO=complete \
		NESTWORTH_QA_ANCHOR=2026-07-26T00:00:00Z \
		go run -mod=readonly ./cmd/analytics-qa-seed > "$run_dir/seed.log" 2>&1
); then
	printf 'The synthetic fixture could not be prepared. See %s\n' "$run_dir/seed.log" >&2
	exit 1
fi

python3 - "$run_dir" <<'PY'
import os
import plistlib
import sqlite3
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

database = Path(os.environ["NESTWORTH_DATABASE_PATH"]).resolve(strict=True)
settings = Path(os.environ["NESTWORTH_SETTINGS_PATH"]).resolve(strict=True)
if database != run_dir / "data/nestworth.db" or settings != run_dir / "data/settings.json":
    sys.exit("The database and settings paths must resolve inside the isolated run directory.")
if not database.is_file() or not settings.is_file():
    sys.exit("The isolated database and settings files could not be verified.")

# The seeder closed this WAL-mode database. Read only the checkpointed file.
with sqlite3.connect(f"{database.as_uri()}?mode=ro&immutable=1", uri=True) as db:
    if db.execute("SELECT name FROM households").fetchall() != [("Analytics QA",)]:
        sys.exit("The synthetic household could not be verified.")
PY

exec "$contents/MacOS/Nestworth"
