# Native Mac CI trial

Regular `check.yml` runs one native ARM64 macOS
job named `test`. It runs the full non-race `go test ./...` population (including
examples and newly added packages), gofmt, vet, native desktop compilation,
skill validation, the unchanged fast tooling tier, and frontend lint, typecheck,
production build and the entire unfiltered Vitest suite. Bindings generate once
per run, using a locally cached pinned Wails CLI; no comparison regeneration.
No correctness assertions or timeouts change. Go package completion is checked
against `go list ./...`; missing and duplicate package results fail the gate.
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

GitHub repository metadata was checked on 2026-11-02: visibility `private`,
`allow_forking=false`. This change does not alter those settings. Repository
writers and authenticated dispatchers are trusted to execute code on the
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
that rule separately; this PR changes no branch protection or required checks.

The former Linux regular workflow is archived verbatim from main commit
`a02fb6635aad48e3f8a8f302bb82a99ff1098ae5` at
`.github/workflow-archive/check-linux.yml.disabled`, outside the workflows
folder. It cannot run automatically or by dispatch. To roll back, replace
`.github/workflows/check.yml` with that archive. Existing Linux workflow regression
tests now validate the archive, not an active fallback. `perf.yml`, race, release
and image publication remain unchanged. Normal Mac CI does not validate Linux
GTK/WebKit compilation or Linux-specific build-tagged paths; existing daily/manual
Linux perf/race validation remains, but is not a per-PR substitute.

No organization access, runner security/service settings or credentials change.
Checkout does not persist its token. The foreground runner must remain online.
No runner archive-cache environment variables are changed by this PR.

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

The experimental worker budget is eleven CPU slots (six Go, four frontend, one
tooling), oversubscribing the measured eight-CPU host. Prior memory allowances
do not establish this configuration's peak RSS;
watch for contention and memory pressure. These are concurrency limits, not
OS-enforced quotas or measured peak RSS claims. Smaller
hosts serialize lanes and the lint/typecheck pair, retaining the previous
resource-based individual limits.
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
complete Go package population. No test timeout or assertion is changed.

## Evidence

Every run uploads environment/cache state, package inventory, Go JSON events,
Vitest JSON results and gate outcomes for seven days. Actions step timing gives
setup/build/test costs separately. A first run may download toolchains and build
Wails; only a subsequent identical-head run with reported warm caches supports
a warm comparison. Historical Linux times (296 s warm PR, 328 s first main) are
context, not a controlled cross-platform benchmark. Trial results and exact head
must be attached to the draft PR before drawing conclusions.

### Measured baseline before parallel lanes

Cold run `41448601991` at `ba4c0c4` passed in 1309 seconds (job time): Wails
bootstrap 538 s, bindings 90 s, Go tests 143 s, tooling 110 s and frontend tests
144 s. Runner: macOS/SDK 27.2, Apple clang 21, eight logical CPUs and 16 GiB RAM.
Warm run `41449223580` at `6feb16b` passed in 534 seconds: Wails 1 s, bindings
14 s, Go tests 91 s, tooling 108 s and frontend tests 143 s; pnpm/action-setup
still spent 87 s preparing its temporary executable. This motivates persistent
pnpm bootstrap plus independent lanes; improved-run results remain to be measured.
The two heads differ in trigger/docs/tooling checks, so these are diagnostic
observations, not a controlled identical-head speedup claim.

The bounded-lane/persistent-pnpm revision at `2e34cd4` passed native run
`41454190925` in 269 seconds (job `125306594368`, 13:34:53–13:39:22 UTC),
with hosted Linux PR run `41454199555` also passing. This is one observed full
run, not a percentile or guaranteed budget. The subsequent cancellation-only
correction requires its own exact-head CI result; it does not change gate commands,
worker budgets, assertions or test timeouts.

### Three-worker frontend experiment

The two-worker baseline at `359d0f559206beb7e06ce6b511d766cb583fb9e8` passed
run `41457637547` (job `125318303247`) in 259 seconds. Gate timings: Go tests
105.415 s, vet 1.862 s, build 5.113 s, tooling 127.783 s, frontend lint 16.838 s,
typecheck 10.049 s and tests 154.105 s (frontend lane about 181 s).
The next bounded trial changes only the eligible tier's frontend worker count
from two to three. Go package/thread limits, tooling, assertions, timeouts and
Mac-only triggers stay unchanged; smaller hosts retain their previous limits.
Compare full coverage, gate timings and job elapsed time against this baseline.
Three workers are not assumed faster; failures or resource pressure call for
reverting this parameter, not relaxing test timeouts.

### Four frontend workers / three Go packages experiment

The same-head three-worker rerun `41459520056`, attempt 2, completed in 210 s
with background service and action archive caches active (setup 4 s). Frontend
tests 119.021 s, Go tests 123.05 s, tooling 115.089 s, frontend lint 16.982 s and
typecheck 9.91 s form the next comparison baseline at `0e73e46`.
The requested next trial changes only eligible-tier frontend workers 3 to 4 and
Go package parallelism 2 to 3. GOMAXPROCS remains 2; smaller-host limits, coverage,
assertions, timeouts, runner service and caches remain unchanged. Compare actual
full-run correctness and timing before deciding whether to retain these settings.

### Parallel lint/typecheck experiment

The preceding worker-count run `41481440253` passed every gate: parallel stage
142 s, Vitest internal duration 101.05 s, all 161 files and 1659 tests passed.
The next isolated change overlaps lint with typecheck, waits for both, then runs
frontend tests. Direct commands avoid package-script binding regeneration and
extra tests; ESLint has no fix/cache flag, and TypeScript has no incremental or
composite mode or output generation. Go package concurrency 3, GOMAXPROCS 2,
frontend workers 4, tooling and all timeouts remain unchanged. During the quality
pair, planned slots are six Go + two quality + one tooling (nine); Vitest does
not overlap the pair. Measure actual contention before claiming improvement.
The shared root cancellation controller still owns both subprocess groups;
missing, failed or cancelled individual outcomes fail the aggregate.
