# Local Development and Packaging

All commands in this document run from the repository root. The supported
desktop target is macOS on Apple Silicon; the Wails Taskfile also contains
cross-compilation tasks for other platforms.

## Prerequisites

- Go 1.26 or newer;
- Node.js with pnpm;
- Wails CLI `v3.0.0-beta.26` available as `wails3`;
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

## Build and package the macOS app

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

The equivalent explicit platform task is:

```bash
wails3 task darwin:package:release
```

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

Run the full gate on the exact reviewed source commit before packaging. The
root release task runs the user-skill checker and installer tests, creates the
standalone skill bundle/checksum, then builds the app and arm64 DMG/ZIP on macOS.

    git status --short
    git rev-parse HEAD
    wails3 task check
    GOCACHE=/tmp/nestworth-go-0.3.5 go test -race ./...
    wails3 task package:release

Default outputs:

    dist/macos/Nestworth.app
    dist/macos/Nestworth-0.3.5-arm64.dmg
    dist/macos/Nestworth-0.3.5-arm64.zip
    dist/macos/SHA256SUMS
    dist/skills/nestworth-skill.tar.gz
    dist/skills/nestworth-skill.tar.gz.sha256

The release task verifies app ID, version, build, arm64 architecture, native
icon, ad-hoc signature, DMG readability, ZIP contents and DMG/ZIP checksums.
The standalone skill task checks its adjacent checksum. The ZIP is created
with ditto's resource-fork, extended-attribute and ACL preservation options.
Neither archive includes a household database, settings file, logs, credentials
or user profile. Do not place local data under dist.

Run a launch smoke directly from the packaged app with fresh temporary data.
Direct executable launch passes the environment variables used by the app;
do not use an existing installation or the default user data path.

    SMOKE_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/nestworth-v0.3.5.XXXXXX")"
    mkdir -p "$SMOKE_ROOT/dmg"
    APP="dist/macos/Nestworth.app"
    DMG="dist/macos/Nestworth-0.3.5-arm64.dmg"
    hdiutil attach -readonly -nobrowse -mountpoint "$SMOKE_ROOT/dmg" "$DMG"
    codesign --verify --deep --strict "$SMOKE_ROOT/dmg/Nestworth.app"
    DMG_APP="$SMOKE_ROOT/dmg/Nestworth.app"
    test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$DMG_APP/Contents/Info.plist")" = "com.nestworth.app"
    test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$DMG_APP/Contents/Info.plist")" = "0.3.5"
    test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$DMG_APP/Contents/Info.plist")" = "6"
    test "$(lipo -archs "$DMG_APP/Contents/MacOS/Nestworth")" = "arm64"
    codesign --verify --deep --strict "$DMG_APP"
    hdiutil detach "$SMOKE_ROOT/dmg"
    NESTWORTH_DATABASE_PATH="$SMOKE_ROOT/household.db" \
    NESTWORTH_SETTINGS_PATH="$SMOKE_ROOT/settings.json" \
    "$APP/Contents/MacOS/Nestworth"

Quit the app, launch the same executable again with the same temporary
database/settings paths, and verify the test household persists. Close the app
before removing only this run's temporary directory:

    rm -rf "$SMOKE_ROOT"

The task verifies the ZIP by extracting it to its own temporary directory and
checking the extracted app's identity, version, build, arm64 architecture, and
signature. Verify downloadable manifests separately:

    (cd dist/macos && shasum -a 256 -c SHA256SUMS)
    (cd dist/skills && shasum -a 256 -c nestworth-skill.tar.gz.sha256)

If Developer ID signing or notarization is required, perform it through the
approved Mac signing setup and regenerate the ZIP and checksums after signing.
The package task's ad-hoc signature is for local launch only. Capture the exact
source SHA and artifact checksums; build and publish from one unchanged source
commit. A successful task does not establish accessibility, Gatekeeper, live
provider, or minimum macOS version acceptance.

See the [v0.3.5 release contract](../releases/v0.3.5.md) for the release
checklist and evidence status.