# Wails v3 Version Upgrade Guide

This guide records the complete procedure for upgrading Wails v3 in Nestworth.
A Wails upgrade is not only a Go module change: the Wails API, frontend runtime,
`wails3` CLI, TypeScript binding generator, and clean-checkout fallback must
remain synchronized.

The current repository baseline is:

```text
Go module:      github.com/wailsapp/wails/v3 v3.0.0-beta.19
Frontend:       @wailsio/runtime 3.0.0-beta.19
CLI/fallback:   v3.0.0-beta.19
```

For another target, replace the version variables below. Go and CLI versions use
a `v` prefix; the npm runtime version does not.

Official references:

- [Wails releases](https://github.com/wailsapp/wails/releases)
- [Wails v3 documentation](https://v3.wails.io/)
- [Wails bindings documentation](https://v3.wails.io/features/bindings/methods/)

## Upgrade boundary

Check all of these locations for every upgrade:

- `go.mod` and `go.sum`: the backend Wails module;
- `frontend/package.json` and `frontend/pnpm-lock.yaml`: `@wailsio/runtime`;
- `frontend/scripts/ensure-bindings.mjs`: the exact version fallback used when
  no global `wails3` is installed;
- `frontend/bindings/`: generated, gitignored TypeScript files;
- README and active development documentation;
- the local `wails3` CLI, installed with `go install` and not stored in the
  repository.

An upgrade must not modify user databases, database schema, business logic, or
handwritten generated bindings. `frontend/dist/`, `bin/`, and `dist/macos/`
build products are not source changes for this procedure.

GitHub Actions pins pnpm separately. Unless the task explicitly includes pnpm,
a Wails upgrade does not change that version; verify that the existing pnpm can
install the target runtime.

## 1. Pre-upgrade checks

Run Go, Wails, and Task commands from the repository root. Run pnpm commands
from `frontend/` only.

Record the workspace state:

```bash
git status --short
git diff --stat
```

If changes already exist, establish that they belong to this task or to the
user's work. Do not use `reset`, `checkout`, or another destructive command to
overwrite them.

Set the target versions and writable caches. Restricted environments should not
rely on the default Go cache directories:

```bash
WAILS_GO_VERSION=v3.0.0-beta.19
WAILS_RUNTIME_VERSION=3.0.0-beta.19
GOCACHE=/tmp/nestworth-wails-gocache
GOMODCACHE=/tmp/nestworth-wails-gomodcache
export WAILS_GO_VERSION WAILS_RUNTIME_VERSION GOCACHE GOMODCACHE
```

Inspect the current toolchain and dependencies:

```bash
go version
node --version
pnpm --version
type -a wails3 2>/dev/null || true
wails3 version 2>/dev/null || true
go list -m github.com/wailsapp/wails/v3
rg -n 'v3\.0\.0-beta|@wailsio/runtime' \
  go.mod frontend/package.json frontend/pnpm-lock.yaml \
  frontend/scripts/ensure-bindings.mjs README.md docs .github
```

Read the target release notes, especially Changed, Fixed, Removed, and
bindings/runtime sections. Passing automated tests does not prove native GUI or
package behavior for a beta release.

## 2. Install the matching `wails3` CLI

The CLI and Go module must use the same exact version:

```bash
go install "github.com/wailsapp/wails/v3/cmd/wails3@$WAILS_GO_VERSION"
PATH="$(go env GOPATH)/bin:$PATH" wails3 version
```

The output must be the target version:

```text
v3.0.0-beta.19
```

If an old version is still reported, inspect `type -a wails3` and retry with the
Go bin directory first in `PATH`. Do not validate only one old absolute path.

## 3. Update the Go Wails module

From the repository root:

```bash
go get "github.com/wailsapp/wails/v3@$WAILS_GO_VERSION"
go mod tidy
```

Inspect the dependency diff:

```bash
git diff -- go.mod go.sum
go list -m github.com/wailsapp/wails/v3
```

The expected change is the Wails module and corresponding checksums. If
`go mod tidy` changes many unrelated dependencies, stop and investigate rather
than mixing unrelated upgrades into this task.

## 4. Update the frontend runtime and lockfile

From `frontend/`, update the exact runtime and install from the lockfile:

```bash
cd frontend
pnpm add "@wailsio/runtime@$WAILS_RUNTIME_VERSION" \
  --save-exact --lockfile-only
pnpm install --frozen-lockfile
```

Inspect both manifests:

```bash
rg -n -C 1 '@wailsio/runtime|3\.0\.0-beta' \
  package.json pnpm-lock.yaml
node -e "console.log(require('./node_modules/@wailsio/runtime/package.json').version)"
cd ..
```

The Node command must print the target runtime version. `--save-exact` keeps the
runtime and backend API relationship explicit; do not use a range here.

## 5. Update the clean-checkout fallback and active documentation

In `frontend/scripts/ensure-bindings.mjs`, update both target-version uses:

1. the `go run github.com/wailsapp/wails/v3/cmd/wails3@...` fallback;
2. the installation version in the generation-failure message.

This fallback matters because CI, a clean checkout, or a new environment may
not have a global CLI. An old fallback generates bindings with an old generator
and can cause frontend type or runtime drift.

Update current-version statements in README, Engineering Guide, Local Workflow,
and this guide:

```bash
rg -n 'v3\.0\.0-beta|@wailsio/runtime' \
  README.md docs/development frontend/scripts/ensure-bindings.mjs
```

Do not globally rewrite historical review baselines. Only active toolchain
documentation describes the current version.

## 6. Regenerate Wails bindings

From the repository root, run the standard repository task:

```bash
PATH="$(go env GOPATH)/bin:$PATH" \
  wails3 task generate:bindings
```

The task runs `go mod tidy` and then an equivalent production-tag generation:

```bash
wails3 generate bindings -f '-tags production' -clean=true -ts -i ./...
```

Do not omit `./...`. Wails services are registered under `cmd/nestworth`; a
root-only scan can miss the actual service packages.

Without a global CLI, use the exact target directly:

```bash
go run "github.com/wailsapp/wails/v3/cmd/wails3@$WAILS_GO_VERSION" \
  generate bindings -f '-tags production' -clean=true -ts -i ./...
```

Confirm that the generator reports services, methods, models, and events, then
inspect the generated location:

```bash
test -f frontend/bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app/index.ts
git status --short --ignored frontend/bindings frontend/dist
```

`frontend/bindings/` is generated and must not be hand-edited or committed. If
tracked files change, determine whether gitignore was accidentally removed
before taking any other action.

## 7. Automated validation

First run static version and format checks:

```bash
test -z "$(gofmt -l cmd internal)"
node --check frontend/scripts/ensure-bindings.mjs
git diff --check
```

Run Go validation with explicit writable caches:

```bash
GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go test ./...
GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go vet ./...
GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" go build ./cmd/nestworth
```

Run frontend validation:

```bash
cd frontend
pnpm install --frozen-lockfile
pnpm run build
pnpm run lint
pnpm run typecheck
pnpm run test
cd ..
```

The repository one-shot check is also available:

```bash
PATH="$(go env GOPATH)/bin:$PATH" \
  GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" \
  wails3 task check
```

Do not count a one-shot check and its repeated component commands as separate
evidence. Record the commands actually run and their exit codes. A Go linker
warning is not a test failure, but its content still belongs in the handoff.

## 8. Native GUI and package validation

Automated checks do not cover native windows, menus, tray behavior, WebView
events, file dialogs, or packaging. On the macOS target environment, run:

```bash
SMOKE_DIR="$(mktemp -d /tmp/nestworth-wails-smoke.XXXXXX)"
NESTWORTH_DATABASE_PATH="$SMOKE_DIR/nestworth.db" \
NESTWORTH_SETTINGS_PATH="$SMOKE_DIR/settings.json" \
  PATH="$(go env GOPATH)/bin:$PATH" wails3 task dev
```

The development smoke test must cover:

- first launch and launch with existing local data;
- at least one frontend-to-Go service call;
- event notification, window resize, menus, and the macOS tray when enabled;
- database-unavailable or recovery UI;
- clean window close and application exit.

Never use a real financial database for smoke testing. Confirm that the
temporary directory and test process no longer consume resources afterward.

Validate a local release-shaped macOS package:

```bash
PATH="$(go env GOPATH)/bin:$PATH" \
  GOCACHE="$GOCACHE" GOMODCACHE="$GOMODCACHE" \
  wails3 task package:release
```

Inspect `dist/macos/Nestworth.app` and the corresponding DMG for bundle ID,
version, build, arm64 architecture, icon, and DMG readability. These are local
unsigned/ad-hoc artifacts. Developer ID signing, notarization, VoiceOver, 200%
zoom, narrow windows, dark mode, and reduced motion are separate release or
manual gates; Go tests and frontend tests cannot replace them.

## 9. Common failures

### `wails3 version` still reports an old version

Check path order:

```bash
type -a wails3
PATH="$(go env GOPATH)/bin:$PATH" wails3 version
```

Do not modify repository scripts to accommodate an incorrect global `PATH`. CI
and clean checkouts must use the exact fallback in
`frontend/scripts/ensure-bindings.mjs`.

### Go reports `cache operation not permitted`

The default cache directory may be unwritable; this does not establish a Wails
API incompatibility. Retry with:

```bash
GOCACHE=/tmp/nestworth-wails-gocache
GOMODCACHE=/tmp/nestworth-wails-gomodcache
export GOCACHE GOMODCACHE
```

Do not change user-directory permissions to bypass this error.

### TypeScript cannot find `frontend/bindings`

Regenerate from the repository root:

```bash
PATH="$(go env GOPATH)/bin:$PATH" wails3 task generate:bindings
```

Without the CLI, use the target-version `go run ...@$WAILS_GO_VERSION` fallback
from the previous section. Do not hand-write missing binding types to hide a
generation failure.

### `pnpm install --frozen-lockfile` fails

Confirm that `pnpm add` ran inside `frontend/` and that the runtime versions in
`package.json` and `pnpm-lock.yaml` agree. Regenerate the lockfile, then rerun a
frozen install. Do not delete the lockfile or create a second root manifest.

### Go API or runtime type errors appear after the upgrade

Read the target release notes and determine whether the cause is an API change,
a production-tag mismatch, or stale generated files. Fix the actual Go service,
frontend call, or Taskfile; do not edit generated bindings. If business code
needs migration, record it as a separate compatibility change with tests.

## 10. Completion checklist

- [ ] Go Wails module, go.sum checksums, frontend runtime, and CLI use the same
      target version.
- [ ] `frontend/scripts/ensure-bindings.mjs` fallback is synchronized.
- [ ] Active development documentation is current; historical review baselines
      were not globally replaced.
- [ ] Bindings were regenerated with the target version and the generated
      directory is not committed.
- [ ] `go test ./...`, `go vet ./...`, and `go build ./cmd/nestworth` pass.
- [ ] Frontend build, lint, typecheck, and test pass.
- [ ] `git diff --check` passes and the diff has no unrelated dependencies or
      build artifacts.
- [ ] Native GUI, package, signing, notarization, and accessibility gate status
      is recorded separately.
- [ ] No user database was opened, migrated, modified, or cleaned.

For a dependency-only change, a suitable Conventional Commit is:

```text
chore(deps): upgrade Wails to v3.0.0-beta.19
```
