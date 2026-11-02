# Native Mac CI trial

The draft migration replaces regular `check.yml` with one native ARM64 macOS
job named `test (macOS trial)`. It runs the full non-race `go test ./...` population (including
examples and newly added packages), gofmt, vet, native desktop compilation,
skill validation, the unchanged fast tooling tier, and frontend lint, typecheck,
production build and the entire unfiltered Vitest suite. Bindings generate once
per run, using a locally cached pinned Wails CLI; no comparison regeneration.
No correctness assertions or timeouts change. Go package completion is checked
against `go list ./...`; missing and duplicate package results fail the gate.
Each gate retains its own outcome and failures do not suppress later gates.

## Trial trust and rollback

Only pushes to `main` or `ci/macos-arm64-trial`, and authenticated manual dispatch,
trigger this experimental workflow. It has no pull-request event, never checks
out a fork head, and only runs in `MnemosyneLab/NestworthGo`. Thus outside PRs
cannot automatically schedule code through this workflow. Repository writers
and dispatchers remain trusted to execute code on the persistent runner.
Every PR, including same-repository and fork PRs, retains automatic hosted Linux
checks through `check-linux.yml`. Its aggregate remains the required `test` check.
The Mac check has a distinct name, `test (macOS trial)`, so a successful Mac push
run cannot substitute for the required PR merge-ref Linux result. This deliberately
keeps Linux coverage during the experiment; it is not yet an exclusive Mac rollout.
PRs from other branches do not automatically execute on the personal Mac.
Promoting that behavior requires a separately agreed trust design; this draft
should remain unmerged until native results and rollout scope are reviewed.
No organization access, runner security/service settings or credentials change.
Checkout does not persist its token. The foreground runner must remain online.

The prior complete Linux workflow is retained as `check-linux.yml`, with only
its name and triggers changed to PR events plus manual dispatch. To roll back, restore
`check.yml` from `a02fb6635aad48e3f8a8f302bb82a99ff1098ae5` and remove the duplicate
rollback workflow. Its existing container/cache/sharding regression tests now
validate that preserved workflow. `perf.yml`, race, release and image publication
are unchanged. Mac runs do not validate Linux GTK/WebKit compilation
or Linux-specific build-tagged code. Automatic Linux PR checks and existing
daily/manual perf retain that platform coverage; macOS timeout tooling can use its existing
portable fixture, while Linux still requires GNU timeout.

## Tools, isolation and capacity

Official setup-go/setup-node and pnpm setup actions select Go from `go.mod`,
Node 24.22.0 and pnpm 12.14.0. Wails installs via `go install` at the exact module
pin into a dedicated user-writable cache. Native preflight verifies all selected
versions, macOS ARM64, macOS/SDK 27+, clang and Python availability; missing native
prerequisites fail with no system installation. It records CPU, memory and cache
state. This is compile/test evidence, not GUI, signing or packaging acceptance.

Caches live at `$RUNNER_TEMP/../nestworth-ci-cache-v1`, outside checkout cleanup.
Native Go/Wails caches are fenced by architecture, OS, SDK path/version, clang,
Go, deployment flags and module locks. Modules and pnpm content-addressed stores
persist independently. No personal cache directories or real ledger are used.
Fallback database/settings paths are inside the disposable runner temporary
folder; tests continue to create their own synthetic fixtures.

Go package concurrency and GOMAXPROCS are bounded to the minimum of four, half
logical CPUs and one slot per 3 GiB RAM (at least one). Frontend uses at most two
workers, preserving its existing interaction-test pressure limit. Phases run
sequentially to avoid competing compiler/jsdom peaks on the user's desktop.
This avoids queueing nine separate shards on one runner, but does not promise
faster execution: the application package now runs as one complete invocation.

## Evidence

Every run uploads environment/cache state, package inventory, Go JSON events,
Vitest JSON results and gate outcomes for seven days. Actions step timing gives
setup/build/test costs separately. A first run may download toolchains and build
Wails; only a subsequent identical-head run with reported warm caches supports
a warm comparison. Historical Linux times (296 s warm PR, 328 s first main) are
context, not a controlled cross-platform benchmark. Trial results and exact head
must be attached to the draft PR before drawing conclusions.
