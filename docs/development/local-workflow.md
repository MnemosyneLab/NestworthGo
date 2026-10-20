# Local Development and Packaging

All commands in this document run from the repository root. The supported
desktop target is macOS on Apple Silicon; the Wails Taskfile also contains
cross-compilation tasks for other platforms.

## Prerequisites

- Go 1.26 or newer;
- Node.js with pnpm;
- Wails CLI `v3.0.0-beta.32` available as `wails3`;
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

## macOS deployment target

Native macOS builds require macOS 27.0 or newer and a macOS 27 SDK.
The root Taskfile applies `MACOSX_DEPLOYMENT_TARGET=27.0` and matching
C/C++ compilation and linker flags to checks, bindings, and development.
Darwin packaging uses the same target; both production and development
Info.plists declare `LSMinimumSystemVersion=27.0`.

For direct Go commands outside Wails tasks, use the same environment:

```bash
export MACOSX_DEPLOYMENT_TARGET=27.0
export CGO_CFLAGS="-O2 -g -mmacosx-version-min=27.0"
export CGO_CXXFLAGS="-O2 -g -mmacosx-version-min=27.0"
export CGO_LDFLAGS="-O2 -g -mmacosx-version-min=27.0"
```

This raises the minimum supported macOS version; existing published artifacts
are unchanged. The legacy Docker Darwin cross-build image contains an older
SDK and is not an accepted macOS 27 packaging path; use native macOS packaging.

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
Do not hand-edit or commit files generated in these directories. The direct
frontend `dev`, `build`, `typecheck`, `test`, and `test:watch` scripts
automatically regenerate bindings before importing them. Run
`generate:bindings` before `check:bindings`; the freshness check requires
existing output and compares it with fresh generator output. The Wails build
tasks regenerate bindings before building the frontend.

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

## Isolated native development QA

Use the established `build/darwin/Info.dev.plist` identity: `com.nestworth.dev`,
with application/display name `Nestworth` and bundle `Nestworth.app`. Production
archive acceptance below retains the separate release identity. Do not create
per-task bundle IDs or names that cause repeated permission prompts.

For each development QA run:

1. Record branch, source SHA and working-tree status. Prepare an isolated
   synthetic database/settings directory and verify its test marker/household.
   These steps do not authorize copying real household data.
2. Build the development app using the existing task configuration and inspect
   the actual bundle's identity before launch.
3. Resolve `NESTWORTH_DATABASE_PATH` and `NESTWORTH_SETTINGS_PATH` to that run's
   intended paths. Launch with those overrides in the same environment; native
   UI launchers must preserve them. Do not launch with unverified overrides.
4. Exercise the affected flow, quit normally and relaunch with the same isolated
   paths to check persistence and interaction consistency. Retain source SHA,
   environment, identity, paths and acceptance results.

Report actual native macOS acceptance separately from Linux, headless, browser
and jsdom evidence. Keep synthetic isolation and the safety rules in root
`AGENTS.md` throughout; do not change system security settings to complete QA.

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
(`.github/workflows/check.yml`). Non-race Go tests are checksum shards. Race
detection and `//go:build perf` evidence run daily at 21:00 UTC, and on
manual dispatch, in `.github/workflows/perf.yml`, not on every pull request.
The daily run skips those jobs when the default branch is unchanged since the
last successful scheduled run. Manual dispatch always runs them. Native
packaging, signing, and notarization remain manual.

The release task also runs `python3 -m unittest discover -s tools/tests -v`
through `skill:check`. Native-dependency helper tests discover GNU `timeout`
or Homebrew `gtimeout` on `PATH`; macOS does not require installing coreutils.
When neither GNU tool is available, a Python POSIX timeout fixture exercises
the same retry, failure, exit-code, and real TERM/KILL assertions. Every host
also replays those tests with GNU discovery disabled. Linux GitHub Actions
requires the GNU backend for the original tests, so the portable fixture does
not replace validation of production timeout behavior. Ubuntu 26.04 ships
uutils as `/usr/bin/timeout` and GNU coreutils as `gnutimeout`; the install
script switches the provider with `coreutils-from-gnu` before the long apt
install so `timeout` on `PATH` is GNU again. The fixture is only
for synthetic tests, not the apt installation script. Mac packaging still
requires its own full `wails3 task package:release` acceptance run.
Synthetic apt commands have a 3-second test budget and 1-second TERM-to-KILL
grace period, with a 30-second harness bound. This allows Python/process
startup before testing intentional stalls; normal commands finish immediately.
A regression injects 350ms of startup delay before recording calls or setting
signal handlers and checks success, TERM/KILL status, and exact retry counts
with both backends. These timings do not change production apt budgets.

## CI race budgets and SDK test lifetime

The daily `race` aggregate fails unless every matrix part succeeds.
Application tests use eight checksum partitions; MCP uses four; sqlite uses
two. Infrastructure and all remaining packages also run with the detector.
Enumeration uses the race build and includes Test, Example, and Fuzz names.
Each process has an explicit 10-minute test budget, `-parallel 4` for
application and MCP, and `-p 1` on the heavy shards; each race job has a
30-minute outer bound including setup. Pull-request `check` bins non-race
tests into three jobs from measured whole-job times: application in two
checksum parts, paired with continuous backup and the remainder, plus one
job for MCP and sqlite. Vitest runs as `--shard=1/2` and `--shard=2/2`,
with lint/typecheck on the first shard and the production build on the
second. Sequential
fault-injection and performance tests remain sequential. Matrix regression
checks require every part exactly once and a complete, disjoint name union.
There are no automatic test retries. Seven-day
`race-<package>-<part>-attempt-<attempt>` artifacts retain Go JSON results
(including elapsed times and failures) for diagnosis, with distinct names
so a manual rerun cannot collide with an earlier attempt.

The four application parts address repeated cumulative timeouts, not reported
data races. Reviewed runs:

| Run | Frontend | Application race 1/2 | Application race 2/2 |
| --- | --- | --- | --- |
| [PR60 initial](https://github.com/MnemosyneLab/NestworthGo/actions/runs/38335642341) | 1011 assertions passed; SDK teardown exception | 151.401s | 600.042s timeout |
| [PR60 updated](https://github.com/MnemosyneLab/NestworthGo/actions/runs/38336865478) | 1031 passed | 164.995s | 383.947s |
| [PR60 main](https://github.com/MnemosyneLab/NestworthGo/actions/runs/38337881280) | 1031 assertions passed; SDK teardown exception | 221.544s | 600.041s timeout |
| [PR61](https://github.com/MnemosyneLab/NestworthGo/actions/runs/38338053581) | 1054 passed | 228.105s | 600.053s timeout |
| [main 61604cb](https://github.com/MnemosyneLab/NestworthGo/actions/runs/38339294933) | 1054 assertions passed; SDK teardown exception | 230.543s | 600.042s timeout |

All four timeout logs show active tests and queued parallel tests without an
assertion failure or race detector report. The updated suite's old partition
counts were 261/305; costs are unequal as well as counts. The same 305-test
part succeeded in 383.947s on one runner, so runner throughput affects whether
the cumulative budget expires. Review the retained timings when adding costly
fixtures instead of raising concurrency or removing coverage.

The separate frontend failure was beta.28's 50ms startup interval in
`drag.js:69` accessing `window` after jsdom teardown on Node 22.23.3. The SDK
integration suite installs fake timers before imports, advances startup while
the DOM exists, asserts no timers remain, then restores real timers for the
actual generated-binding cancellation tests. It neither clears arbitrary
timers nor ignores unhandled errors. Pure wire-error helpers stay independent
of the runtime barrel; their regression test rejects any SDK import. Actual
late-native-error identity checks, live errors, unrelated rejections and
cancellation delivery failures remain exercised by the installed SDK tests.

## Build and package the macOS app

For Wails beta.32, use a checkout on local, non-synchronized storage. Our
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

Desktop runs use the operating system's user config directory by default, at
`Nestworth/nestworth.db`. Set `NESTWORTH_DATABASE_PATH` to an explicit writable
database file when the OS config directory is unavailable or for an isolated
synthetic test. Startup stops with an actionable error if it cannot determine
a persistent database path; it does not create a temporary ledger that could
look like an empty household.

## Release artifacts and isolated Mac acceptance

Use the exact reviewed source commit with passing CI before packaging. When
unchanged source already passed CI and native acceptance is user-reported, do
not repeat the full test/race suite solely for packaging. Run targeted package
verification and the archive-payload launch checks below. The root release task runs the user-skill checker and installer tests, creates the
standalone skill bundle/checksum, then builds the app and arm64 DMG/ZIP on macOS.

    git status --short
    git rev-parse HEAD
    wails3 task package:release

For the unreleased v0.3.9/build 10 development version, default outputs are:

    dist/macos/Nestworth.app
    dist/macos/Nestworth-0.3.9-arm64.dmg
    dist/macos/Nestworth-0.3.9-arm64.zip
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
    SMOKE_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/nestworth-v0.3.9.XXXXXX")"
    mkdir -p "$SMOKE_ROOT/mount" "$SMOKE_ROOT/dmg" "$SMOKE_ROOT/zip" "$SMOKE_ROOT/dmg-data" "$SMOKE_ROOT/zip-data"
    APP="dist/macos/Nestworth.app"
    DMG="dist/macos/Nestworth-0.3.9-arm64.dmg"
    ZIP="dist/macos/Nestworth-0.3.9-arm64.zip"
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
      test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$ARCHIVE_APP/Contents/Info.plist")" = "0.3.9"
      test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$ARCHIVE_APP/Contents/Info.plist")" = "10"
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
`dist/macos/Nestworth-0.3.9-arm64.dmg`, recreate the ZIP with
`/usr/bin/ditto -c -k --sequesterRsrc --keepParent dist/macos/Nestworth.app dist/macos/Nestworth-0.3.9-arm64.zip`,
regenerate the manifest, and run `wails3 task darwin:verify:package`. Replace
only the task-generated app bundle paths, and ensure they are not symlinks.
Both app copies must come from the same signed/stapled bundle. Regenerate
`SHA256SUMS` with `shasum -a 256 Nestworth-0.3.9-arm64.dmg
Nestworth-0.3.9-arm64.zip > SHA256SUMS` from `dist/macos`, then verify it. The
package task's usual ad-hoc
signature is for local launch only. Capture the exact source SHA and artifact
checksums; build and publish from one unchanged source commit. A successful
task does not establish accessibility, Gatekeeper, live provider, or minimum
macOS version acceptance.

See the [latest published release record](../releases/v0.3.8.md) for the
v0.3.8 checklist and evidence status. It does not establish acceptance or
publication of the v0.3.9 development version. Historical v0.3.6 closeout evidence remains in
the [v0.3.6 contract](../releases/v0.3.6.md).
