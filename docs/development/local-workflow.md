# Local Development and Packaging

All commands in this document run from the repository root. The supported
desktop target is macOS on Apple Silicon; the Wails Taskfile also contains
cross-compilation tasks for other platforms.

## Prerequisites

- Go 1.26 or newer;
- Node.js with pnpm;
- Wails CLI `v3.0.0-beta.28` available as `wails3`;
- macOS and Xcode command-line tools for native `.app` and `.dmg` packaging.
- Linux additionally needs GCC, pkg-config, and Wails GTK/WebKit headers to
  compile `./cmd/nestworth`:

```bash
sudo apt-get install -y gcc pkg-config libgtk-4-dev libwebkitgtk-6.0-dev libsoup-3.0-dev
```

Check the local toolchain before starting:

```bash
go version
node --version
pnpm --version
wails3 version
```

On macOS 27/Xcode 26, set `SDKROOT` to the Xcode macOS SDK for native Go
linking if the default Command Line Tools SDK reports an unknown
`arm64e.x1-macos` architecture:

```bash
export SDKROOT="$(xcrun --sdk macosx --show-sdk-path)"
```

Keep this in the shell used for `wails3 task check`, `wails3 task build`, and
`wails3 task package:release`.

## First-time setup

After cloning the repository or switching to a clean checkout, run:

```bash
wails3 task setup
```

This downloads Go modules, installs the locked frontend dependencies, and
generates the Wails TypeScript bindings under `frontend/bindings/`.

`frontend/bindings/` is generated and gitignored. `frontend/dist/` is also
generated, except for a committed `.gitkeep` so `//go:embed all:frontend/dist`
succeeds on a clean checkout before Vite has produced the production bundle.
Do not hand-edit or commit other files in these directories. The direct
frontend `dev`, `build`, `typecheck`, `test`, and `test:watch` scripts
automatically generate bindings when the expected binding file is missing.
The Wails build tasks regenerate them as part of their normal dependency
graph.

If a bound Go service or DTO changes, regenerate explicitly:

```bash
wails3 task generate:bindings
```

The equivalent frontend command is:

```bash
cd frontend && pnpm run generate:bindings
```

The lower-level Wails command is `wails3 generate bindings -ts -i ./...`.

## Start the desktop app in development

Run the full Wails development loop from the repository root:

```bash
wails3 task dev
```

This starts the Go rebuild loop, the Vite development server, and the Wails
desktop shell using `build/config.yml`. The default Vite port is `9245`; use a
different free port when necessary:

```bash
WAILS_VITE_PORT=9345 wails3 task dev
```

Stop the loop with `Ctrl-C`. `cd frontend && pnpm run dev` starts Vite only; it
is useful for frontend work but is not a complete desktop-app launch because
the Go/Wails IPC host is absent.

## Frontend-only checks

The following commands are safe to run after entering `frontend/`, including
after a clean checkout:

```bash
pnpm run lint
pnpm run typecheck
pnpm run test
pnpm run build
```

`typecheck`, `test`, and `build` run the bindings guard first. `lint` does not
need generated bindings. `build` runs TypeScript checking and creates the Vite
production bundle in `frontend/dist/`.

From the repository root, the combined validation task is:

```bash
wails3 task check
```

It regenerates the frontend bundle, then runs `gofmt` validation, Go tests,
`go vet`, the canonical Go build, frontend lint, frontend type checking,
frontend tests, and `git diff --check`.

The same automated gates run in GitHub Actions on `main` and pull requests
(`.github/workflows/check.yml`), with race detection as a separate job. Native
packaging, signing, and notarization remain manual.

The release task also runs `python3 -m unittest discover -s tools/tests -v`
through `skill:check`. Native-dependency helper tests discover GNU `timeout`
or Homebrew `gtimeout` on `PATH`; macOS does not require installing coreutils.
When neither GNU tool is available, a Python POSIX timeout fixture exercises
the same retry, failure, exit-code, and real TERM/KILL assertions. Every host
also replays those tests with GNU discovery disabled. Linux GitHub Actions
requires the GNU backend for the original tests, so the portable fixture does
not replace validation of production timeout behavior. The fixture is only
for synthetic tests, not the apt installation script. Mac packaging still
requires its own full `wails3 task package:release` acceptance run.

## Build and package the macOS app

For Wails beta.28, use a checkout on local, non-synchronized storage. Our
customized Darwin tasks reject symlinked output; the upstream generated
file-provider relocation task is not applied automatically. See the
[beta.28 compatibility notes](wails-version-upgrade.md#beta28-compatibility-notes)
before building in an iCloud Drive, Dropbox, or similar folder.


Use the root tasks for the normal arm64 flow:

| Command | Output | Purpose |
| --- | --- | --- |
| `wails3 task build` | `bin/nestworth` | Production Go binary with embedded frontend |
| `wails3 task package` | `bin/Nestworth.app` | Ad-hoc signed local `.app` bundle |
| `wails3 task package:dmg` | `bin/Nestworth.dmg` | UDZO DMG with an Applications shortcut |
| `wails3 task package:release` | `dist/macos/Nestworth.app`, versioned arm64 DMG/ZIP, `SHA256SUMS`, and the standalone skill bundle plus checksum | Builds and verifies local release-shaped outputs |

The release task is the recommended local packaging smoke test:

```bash
wails3 task package:release
```

When only the macOS artifacts are needed, run the Darwin platform task:

```bash
wails3 task darwin:package:release
```

This platform task does not run the standalone skill checks or create the skill
archive/checksum. The root `package:release` task includes those skill steps
before invoking the Darwin packaging task.

The generated `.app` receives an ad-hoc signature so it can be launched
locally. This does not satisfy Developer ID signing, notarization, or public
distribution requirements. Those are separate release gates.

To build a universal macOS binary and app bundle:

```bash
wails3 task darwin:package:universal
```

For distribution signing, configure Wails signing credentials first with
`wails3 setup`, then use the platform signing tasks documented by `wails3 task
--list`:

```bash
wails3 task darwin:sign -- --identity "Developer ID Application: ..."
wails3 task darwin:sign:notarize
```

Do not call a locally ad-hoc-signed artifact “released” until signing,
notarization, manual accessibility review, and artifact retention have been
completed or explicitly waived by the release owner.

## Generated artifacts and troubleshooting

The important local outputs are:

```text
frontend/bindings/   Wails-generated TypeScript bindings (ignored)
frontend/dist/       Vite production bundle (ignored except .gitkeep)
bin/nestworth        Production executable (ignored)
bin/Nestworth.app    Local macOS app bundle (ignored)
bin/Nestworth.dmg    Local UDZO disk image (ignored)
dist/macos/          Verified local release-shaped artifacts (ignored)
```

If TypeScript reports `Property 'available' does not exist` or cannot resolve
a module under `frontend/bindings/`, the checkout is missing generated
bindings. Run `wails3 task generate:bindings`, or rerun the affected `pnpm`
script; the guard will repair a missing directory automatically.

If the development server reports that port `9245` is busy, use
`WAILS_VITE_PORT=<free-port> wails3 task dev`. If a packaged app opens with
unexpected data during a smoke test, set isolated `NESTWORTH_DATABASE_PATH`
and `NESTWORTH_SETTINGS_PATH` values; never use real financial data for a
package check.

## Release artifacts and isolated Mac acceptance

Use the exact reviewed source commit with passing CI before packaging. When
unchanged source already passed CI and native acceptance is user-reported, do
not repeat the full test/race suite solely for packaging. Run targeted package
verification and the archive-payload launch checks below. The root release task runs the user-skill checker and installer tests, creates the
standalone skill bundle/checksum, then builds the app and arm64 DMG/ZIP on macOS.

    git status --short
    git rev-parse HEAD
    wails3 task package:release

Default outputs:

    dist/macos/Nestworth.app
    dist/macos/Nestworth-0.3.7-arm64.dmg
    dist/macos/Nestworth-0.3.7-arm64.zip
    dist/macos/SHA256SUMS
    dist/skills/nestworth-skill.tar.gz
    dist/skills/nestworth-skill.tar.gz.sha256

The release task verifies app ID, version, build, arm64 architecture, native
icon, ad-hoc signature, DMG readability, ZIP contents and DMG/ZIP checksums.
The standalone skill task checks its adjacent checksum. The ZIP is created
with ditto's resource-fork, extended-attribute and ACL preservation options.
Neither archive includes a household database, settings file, logs, credentials
or user profile. Do not place local data under dist.

Archive verification alone is not launch acceptance. The package task's
identity, payload, architecture and signature checks remain required; launch
and relaunch **each archive's actual payload** afterward. Close any running
Nestworth process first. Use synthetic test households, leave R2/MCP disabled,
and never point these checks at existing user data. These are instructions for
later Mac execution, not a claim that Mac acceptance was run here.

Run the following in one Bash session, stopping on any failed command. Copy the
read-only DMG payload with `ditto` before detaching, and extract the ZIP with
`ditto` to preserve metadata. Do not launch `dist/macos/Nestworth.app` in place
of either archive's payload.

    set -e
    SMOKE_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/nestworth-v0.3.7.XXXXXX")"
    mkdir -p "$SMOKE_ROOT/mount" "$SMOKE_ROOT/dmg" "$SMOKE_ROOT/zip" "$SMOKE_ROOT/dmg-data" "$SMOKE_ROOT/zip-data"
    APP="dist/macos/Nestworth.app"
    DMG="dist/macos/Nestworth-0.3.7-arm64.dmg"
    ZIP="dist/macos/Nestworth-0.3.7-arm64.zip"
    hdiutil attach -readonly -nobrowse -mountpoint "$SMOKE_ROOT/mount" "$DMG"
    codesign --verify --deep --strict "$SMOKE_ROOT/mount/Nestworth.app"
    /usr/bin/ditto --rsrc --extattr --acl "$SMOKE_ROOT/mount/Nestworth.app" "$SMOKE_ROOT/dmg/Nestworth.app"
    hdiutil detach "$SMOKE_ROOT/mount"
    unzip -tq "$ZIP"
    /usr/bin/ditto -x -k "$ZIP" "$SMOKE_ROOT/zip"

Verify both copied/extracted bundles, retaining the payload checks already
performed by `wails3 task darwin:verify:package`:

    for ARCHIVE_APP in "$SMOKE_ROOT/dmg/Nestworth.app" "$SMOKE_ROOT/zip/Nestworth.app"; do
      test -d "$ARCHIVE_APP"
      test ! -L "$ARCHIVE_APP"
      test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$ARCHIVE_APP/Contents/Info.plist")" = "com.nestworth.app"
      test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$ARCHIVE_APP/Contents/Info.plist")" = "0.3.7"
      test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$ARCHIVE_APP/Contents/Info.plist")" = "8"
      test "$(lipo -archs "$ARCHIVE_APP/Contents/MacOS/Nestworth")" = "arm64"
      for PAYLOAD in Contents/MacOS/Nestworth Contents/Info.plist Contents/Resources/icon.icns; do
        test ! -L "$ARCHIVE_APP/$PAYLOAD"
        cmp -s "$APP/$PAYLOAD" "$ARCHIVE_APP/$PAYLOAD"
      done
      if [ -f "$APP/Contents/Resources/Assets.car" ]; then
        test ! -L "$ARCHIVE_APP/Contents/Resources/Assets.car"
        cmp -s "$APP/Contents/Resources/Assets.car" "$ARCHIVE_APP/Contents/Resources/Assets.car"
      else
        test ! -e "$ARCHIVE_APP/Contents/Resources/Assets.car"
      fi
      codesign --verify --deep --strict "$ARCHIVE_APP"
    done

Direct executable launch passes the environment overrides consumed by
`defaultDatabasePath` in `cmd/nestworth/main.go` and `settings.DefaultStore` in
`internal/settings/settings.go`; these are environment variables, not CLI flags.
Settings also derives diagnostics from the isolated settings location, while
backup configuration lives beside the isolated database.

Launch the DMG copy, create a synthetic household, then quit normally. Run the
same command again and verify that household persists; quit before continuing:

    NESTWORTH_DATABASE_PATH="$SMOKE_ROOT/dmg-data/household.db" \
    NESTWORTH_SETTINGS_PATH="$SMOKE_ROOT/dmg-data/settings.json" \
    "$SMOKE_ROOT/dmg/Nestworth.app/Contents/MacOS/Nestworth"

    NESTWORTH_DATABASE_PATH="$SMOKE_ROOT/dmg-data/household.db" \
    NESTWORTH_SETTINGS_PATH="$SMOKE_ROOT/dmg-data/settings.json" \
    "$SMOKE_ROOT/dmg/Nestworth.app/Contents/MacOS/Nestworth"

Launch the ZIP extraction with its **separate** temporary database/settings.
Create a different synthetic household, quit, relaunch with the same ZIP paths,
and verify persistence; quit normally:

    NESTWORTH_DATABASE_PATH="$SMOKE_ROOT/zip-data/household.db" \
    NESTWORTH_SETTINGS_PATH="$SMOKE_ROOT/zip-data/settings.json" \
    "$SMOKE_ROOT/zip/Nestworth.app/Contents/MacOS/Nestworth"

    NESTWORTH_DATABASE_PATH="$SMOKE_ROOT/zip-data/household.db" \
    NESTWORTH_SETTINGS_PATH="$SMOKE_ROOT/zip-data/settings.json" \
    "$SMOKE_ROOT/zip/Nestworth.app/Contents/MacOS/Nestworth"

Record source SHA, archive checksums and each payload's launch/relaunch result.
Only after both apps exit, remove this run's temporary directory. If a check
failed while the DMG was mounted, detach that mount before cleanup:

    rm -rf "$SMOKE_ROOT"

Verify downloadable manifests separately:

    (cd dist/macos && shasum -a 256 -c SHA256SUMS)
    (cd dist/skills && shasum -a 256 -c nestworth-skill.tar.gz.sha256)

If Developer ID signing or notarization is required, complete signing and
stapling on the final `.app`, then rebuild both the DMG and ZIP from that same
signed/stapled bundle, regenerate SHA256SUMS, and run the package verifier
again. Do not rerun `wails3 task package:release` or
`wails3 task darwin:package:release` after signing: they rebuild the app and
apply an ad-hoc signature. The archive-only sequence is to copy the final app
to both `bin/Nestworth.app` and `dist/macos/Nestworth.app`, run
`wails3 task darwin:create:dmg`, copy `bin/Nestworth.dmg` to
`dist/macos/Nestworth-0.3.7-arm64.dmg`, recreate the ZIP with
`/usr/bin/ditto -c -k --sequesterRsrc --keepParent dist/macos/Nestworth.app dist/macos/Nestworth-0.3.7-arm64.zip`,
regenerate the manifest, and run `wails3 task darwin:verify:package`. Replace
only the task-generated app bundle paths, and ensure they are not symlinks.
Both app copies must come from the same signed/stapled bundle. Regenerate
`SHA256SUMS` with `shasum -a 256 Nestworth-0.3.7-arm64.dmg
Nestworth-0.3.7-arm64.zip > SHA256SUMS` from `dist/macos`, then verify it. The package task's usual ad-hoc
signature is for local launch only. Capture the exact source SHA and artifact
checksums; build and publish from one unchanged source commit. A successful
task does not establish accessibility, Gatekeeper, live provider, or minimum
macOS version acceptance.

See the [current release contract](../releases/v0.3.7.md) for the release
checklist and evidence status. Historical v0.3.6 closeout evidence remains in
the [v0.3.6 contract](../releases/v0.3.6.md).
