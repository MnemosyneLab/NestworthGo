# Native Mac CI

Regular `check.yml` runs one native ARM64 macOS
job named `test`. It runs the full non-race `go test ./...` population (including
examples and newly added packages), gofmt, vet, native desktop compilation,
skill validation, the unchanged fast tooling tier, and frontend lint, typecheck,
production build and the entire unfiltered Vitest suite. Bindings generate once
per run, using a locally cached pinned Wails CLI; no comparison regeneration.
Go package completion is checked against `go list ./...`; missing and
duplicate package results fail the gate.
Each gate retains its own outcome and failures do not suppress later gates.

## Automatic checks, trust and rollback

Regular CI runs only on the native Mac: every push to any branch in
`MnemosyneLab/NestworthGo`, plus authenticated manual dispatch. The stable required
check name is `test`. Internal PRs get automatic full checks on each pushed head,
including branches created before opening the PR; main pushes also run. This
checks the branch head, not GitHub's synthetic PR merge commit. Base-only updates,
opening or reopening a PR without a new push do not rerun a previously checked
head. Update the internal branch from main and push when integration needs
revalidation; manual dispatch on that internal branch is also available.

Repository writers and authenticated dispatchers are trusted to execute code on the
persistent personal runner. There is deliberately no `pull_request` or
`pull_request_target` trigger: a push to a fork cannot schedule this repository's
Mac even if the fork edits its copy of the workflow. The repository identity
check also prevents this workflow copy from running in another repository.

If external fork contributions become possible later, their PRs do not receive a
successful or skipped `test` from this workflow. The required check remains
missing/pending until a maintainer has reviewed the entire proposed code,
workflow, dependency and install-script changes and imported the approved commit
onto an internal branch. That internal push runs the full Mac suite; use the
reviewed internal PR for integration. Do not dispatch a fork ref or bypass the
required check. If a protected rule requires a specific workflow identity/event
rather than the existing `test` check context, the repository owner must review
that rule separately.

The former Linux regular workflow is archived verbatim from main commit
`a02fb6635aad48e3f8a8f302bb82a99ff1098ae5` at
`.github/workflow-archive/check-linux.yml.disabled`, outside the workflows
folder. It cannot run automatically or by dispatch. To roll back, replace
`.github/workflows/check.yml` with that archive. Existing Linux workflow regression
tests validate the archive. Normal Mac CI does not validate Linux
GTK/WebKit compilation or Linux-specific build-tagged paths; existing daily/manual
Linux perf/race validation remains, but is not a per-PR substitute.

Checkout does not persist its token. The foreground runner must remain online.

## Tools, isolation and capacity

Official setup-go/setup-node actions select Go from `go.mod` and Node 24.22.3.
Pinned pnpm 12.19.0 is installed once with npm into the dedicated runner cache
(`pnpm-cli-darwin-arm64-v24.22.3-12.19.0`), then version-checked and reused without
invoking npm on warm runs. Bootstrap disables lifecycle scripts, audit and funding
requests and never installs globally. Wails installs via `go install` at the exact module
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

Bindings and the Vite production build complete before parallel checks, so Go
embed readers never race frontend output writes. On hosts with at least eight
logical CPUs and 16 GiB RAM, three sequential lanes overlap:

- Go: formatting, full tests, vet, native build; package parallelism three and
  GOMAXPROCS two (up to six Go worker threads across package processes).
- Tooling: unchanged fast tier and skill validation, then whitespace checking.
- Frontend: direct `eslint .` and `tsc --noEmit` overlap, then both finish
  before the full suite starts with four jsdom workers. Each check retains its
  own log, exit status and timing; a failed check does not suppress the tests.

The worker settings allow up to eleven concurrent CPU slots across lanes
(six Go, four frontend, one tooling). These are concurrency settings.
Smaller hosts serialize the lanes and lint/typecheck and use lower individual
limits.
Tooling's bindings fixtures use temporary copies, never the shared generated tree.

Each gate records its exit code, outcome, seconds and separate complete log under
`macos-gate-logs/`. Console START/END messages expose progress; completed logs print
in separate GitHub groups. Failure does not suppress later independent checks.
Cancellation forwards TERM to each owned gate group before bounded KILL escalation
and prevents later gates starting. Nested schedulers inherit a shorter grace
budget; the portable timeout fixture forwards cancellation to its separately
created session and reaps it before the enclosing owner escalates. No unrelated
host processes are enumerated or signalled.
On macOS, an owned group can contain only zombies after TERM and leader
reaping. XNU's [killpg1 implementation](https://github.com/apple-oss-distributions/xnu/blob/f6217f891ac0bb64f3d375211650a4c1ff8ca1ea/bsd/kern/kern_sig.c)
filters zombie members and returns EPERM when it counts no signalable members.
EPERM therefore does not by itself prove either a live process or permission
misconfiguration. Only on Darwin and only after EPERM, the scheduler queries
that one owned group with `ps -g PGID -o pid=,pgid=,stat=` in Unix2003 mode.
No members or exclusively zombie members establish that nothing is executing.
A live member, unexpected group/state, failed query or timeout preserves the
permission failure; no blanket permission suppression is used. The direct child
is still reaped, surviving descendants still receive bounded KILL escalation,
and nested owners retain their shorter cleanup grace. Normal successful gates
perform no extra process inspection. Native/Linux fixtures deliberately retain
an owned zombie to record the platform's actual probe errno; live-descendant,
permission-denial and parallel cancellation tests retain cleanup guarantees.
Cancellation fixtures also require the scheduler to exit successfully, so an
unhandled cleanup exception cannot hide behind a child-exit assertion.

The final aggregate rejects missing, failed or cancelled gates and rechecks the
complete Go package population.

## Evidence

Every run uploads environment/cache state, package inventory, Go JSON events,
Vitest JSON results and gate outcomes for seven days. Actions step timing gives
setup/build/test costs separately. A first run may download toolchains and build
Wails; only a subsequent identical-head run with reported warm caches supports
a warm comparison. Record the exact head, cache state and gate timings in the PR
or Actions run for any CI performance comparison.
