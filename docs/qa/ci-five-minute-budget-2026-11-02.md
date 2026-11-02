# CI correctness and five-minute budget (2026-11-02)

The five-minute end-to-end target remains an acceptance criterion. The observed
hosted runs have **not** met it (517s initially, then 402s and 307s after the successive fixes). This record separates measured evidence
from the new layout; local timings do not establish the hosted result.

## Hosted baseline

[Run 41382148182](https://github.com/MnemosyneLab/NestworthGo/actions/runs/41382148182)
tested `a7f0d913dc55cf91927cfcd9741af809db1de963`. GitHub's timing endpoint reports
`run_duration_ms=517000`; creation-to-final-update wall time is also **8m37s**,
**3m37s above** the 5m target. It started at 03:16:05 UTC. First job allocation
took 3s, runtime resolution 7s, and dependent jobs started at +13s. The longest
job ended at +510s; aggregate allocation/execution ended at +516s, with final
bookkeeping at +517s. The dominant delay was execution, not a long runner queue.

| Job | Job execution | Actual test execution / significant preparation |
| --- | ---: | --- |
| Frontend 1/2 | 497s | Tests 228s; bindings 102s + verification 12s; lint 43s; typecheck 32s |
| Frontend 2/2 | 490s | Tests 262s; bindings 99s + verification 12s; production build 37s |
| Meta | 281s | Go vet 95s; tooling/skill checks 124s |
| Go application 2/2 + remainder | 279s | Go tests/build preparation step 219s |
| Go application 1/2 + continuous backup | 202s | Go tests/build preparation step 142s |
| Go MCP + SQLite | 194s | Go tests/build preparation step 138s |

Native container initialization took 20–28s per native job and setup-go took
17–19s. These are included in job execution, not counted as queue time. Go step
times include compilation/enumeration; package JSON elapsed values alone do not
account for the whole Go step. The two frontend halves had 228s/262s of test
execution, so test balance alone could not remove the 217s overall gap.

Both frontend jobs hit the shared Go cache, yet `go run wails3@v3.0.0` downloaded
and compiled extra CLI dependencies. The first generation spent 99–102s; a warm
generation/verification in the same job took about 10–12s. Existing immutable
meta/Go cache entries cannot learn those missing CLI inputs on an exact hit.

## First redesign hosted result and bounded follow-up

[Run 41386006359](https://github.com/MnemosyneLab/NestworthGo/actions/runs/41386006359)
on `6405263874628bd821111c8a4ce5c7f08c1a4623` passed every gate. Both GitHub
run duration and 03:59:58–04:06:40 UTC wall time were **402s (6m42s)**: 115s
below the initial 517s but **102s above** the 300s target. Queueing remained only
seconds. This is correctness evidence, not five-minute acceptance.

- Bindings preparation ran 212s: native container 24s, setup-go 18s, Node/pnpm
  setup 16s, CLI overlay lookup 1s, first generation 105s, fresh comparison 12s,
  artifact upload 2s and first cache save 13s (plus normal job overhead).
  Logs explicitly show the overlay miss and successful save under
  `wails-bindings-v1-Linux-X64-8fee68af0385289a9c631b21d0f8c1d5a2ec96d2c6323e825096d75ff3d4cce3`.
  The bounded follow-up changes none of the cache-key inputs. Its next run can
  therefore measure the populated cache, rather than blindly retrying a miss.
- The five frontend jobs took 93/157/112/143/135s, including approximately 32s
  per job of setup. Actual test phases were 60/125/81/110/104s. Quality took 92s
  (ESLint 40s, TypeScript 21s, Vite 3s). All 1657 tests across 161 files passed.
  Timing weights are refreshed from these exact successful shard artifacts;
  assignment and complete/disjoint coverage guards remain unchanged.
- The critical Go application-2/remainder job grew to 381s; its Go step consumed
  321s. Application enumeration/compilation preceded a 120.448s application test
  process, followed by module download/list/compile work and the remainder.
  Other Go jobs were 199s/147s. The follow-up splits remainder into a fourth Go
  job, retaining the existing application checksum halves, package enumeration,
  `-p` bounds and complete coverage. No Go test is removed or moved to perf.
- A review found that restoring the compiled Wails overlay after the fallback's
  post-apt clean could defeat the unpinned native-cache fence. The overlay now
  runs **only** inside the reviewed pinned container. On unpinned bootstrap it
  is neither restored nor saved; module downloads remain reusable and compiled
  artifacts stay cleared. A guard checks this condition and exact step ordering.

The first redesign's local five-shard validation also passed 1657 tests with
complete/disjoint file union; local shard durations were 55.96/61.30/58.50/57.68/
54.20s. Fast tooling passed 67 tests in 30.43s and extended tooling passed 13 in
86.84s. Binding freshness, ESLint, typecheck and production build passed. Those
measurements describe that head; final follow-up evidence belongs to the PR.

The next hosted candidate still needs both correctness and end-to-end timing
validation. In particular, the populated CLI cache must actually hit and the
independent Go remainder must reduce the critical path. No result is inferred
from local timings or from cache-save evidence alone.

## Warm-cache result and final bounded headroom adjustment

[Run 41387631566](https://github.com/MnemosyneLab/NestworthGo/actions/runs/41387631566)
on `a17eea1d29281acc3cf47d026a3039e98efa4420` passed every gate. Its observed
creation-to-final-update interval, 04:17:33–04:22:40 UTC, was **307s**: still
**7s above** the original 300s clock. Go completed by 04:21:10, so the split
removed the former Go critical path. Bindings ended at 04:19:55 after 128s;
frontend shard 3 then took 155s through 04:22:33, followed by aggregate/finalization.
Correctness passed, but this run did not satisfy the latency target.

Two bounded changes add headroom without deleting coverage or loosening deadlines:

1. The bindings producer calls the existing Node scripts directly. Both package
   scripts are exact wrappers around `node scripts/ensure-bindings.mjs` and its
   `--check` variant, whose imports are Node built-ins. The producer no longer
   installs pnpm, restores its store or installs frontend dependencies. The last
   hosted run spent 4s/3s/4s on those steps plus 1s post-setup. Real generation
   and fresh comparison also passed in a clean detached worktree containing no
   `node_modules`, proving the removed installation is unnecessary. Node consumers
   keep their locked dependency installation and package-manager cache.
2. Six frontend shards replace five, still with two workers each and the same
   complete assignment algorithm. One History shard remains isolated; five
   weighted workersets share the remaining files instead of four. This reduces
   per-file environment/import load on the observed 155s tail. A discovered-file
   union guard and actual shard execution must preserve all 161 files/1657 tests.

The producer's generation/comparison gates, image-pinned cache condition and
unpinned fallback clean remain unchanged. This uses one additional ordinary
Node runner, with no new paid runner service or security configuration. The
intended margin is a projection; only the next hosted end-to-end measurement
can establish success, and a single borderline pass is not a throughput guarantee.

## Remaining reconciliation regression

Hosted CI passed 883/884 tests in shard 2. The one failure was a 20-click App
scenario crossing explicit-session retention, guarded navigation, global reset,
account scoping and local-session Back. It exhausted the unchanged 5s test budget
at 5069.04ms. The same scenario took 4866ms locally with two workers sharing one
CPU. An isolated cold run under a 0.5 CPU quota also exposed an earlier Tasks
lazy-import boundary: the first session-list lookup expired at 3012ms overall.
Neither observation establishes a production accounting or navigation defect.

The test is now three independent contracts. Each seeds its relevant prior state
through real navigation, including the previously remembered Reconciliation tab
needed by the local-session Back scenario. Every original assertion remains;
new assertions verify that setup. Cold Tasks/Data Health imports settle before
destination controls are queried. Real controls, Back, confirmation guards and
all 1s lookup / 5s test defaults remain. Two independent cold 0.5 CPU runs passed
all three scenarios: 3905/2276/2789ms and 3912/2371/2706ms respectively.

## New fast-check layout

- A single native `frontend-bindings` job generates and freshly compares bindings.
  Its exact-run, source-SHA-scoped artifact is shared by the Node consumers. It
  overlays a separately saved, image/toolchain/version-fenced Wails CLI cache on
  the shared Go baseline; a first CLI-cache miss keeps the existing baseline.
- `frontend-quality` runs ESLint, one TypeScript check and the production Vite
  build on the same verified artifact. Local developer scripts keep their normal
  binding guards. Aggregate checks fail unless preparation and quality succeed.
- Four Go jobs retain every correctness package while removing the serial
  application-2/remainder dependency. Six Node-only test shards keep two workers per runner. HistoryPage, an
  indivisible long hosted test file, gets its own shard. The others use deterministic
  longest-first assignment from the hosted file timings plus 1.25s/file setup.
  New/unmeasured files receive a conservative default and are always assigned.
  A regression checks the complete discovered-file union, uniqueness, nonempty
  bins, deterministic assignment and the dedicated History shard.
- Node consumers no longer initialize native containers, install Go or regenerate
  bindings. All application assertions remain in the normal PR/main gate. No
  application test is reclassified as performance coverage.

This uses the existing GitHub-hosted Ubuntu runner type and reviewed native image;
it introduces no paid runner service or infrastructure/security configuration.
The additional short Node jobs replace duplicate native preparation. Cache-miss
preparation and native Go compilation may still constrain hosted wall time.
The first cache-populating run must be reported separately from a warm run; no
five-minute claim is made until the coordinator measures the new hosted head.

## Coverage migration and deduplication inventory

| Coverage/work | Normal PR/main check | Existing daily/manual `perf` workflow |
| --- | --- | --- |
| All Go correctness, financial precision, security and commit/recovery tests | Unchanged, all current Go shards | Existing race/perf coverage unchanged |
| All frontend tests, including money, navigation, focus and interrupted submissions | All six shards; no exclusions | Nothing moved |
| Binding generation + freshness comparison | Once in producer, exact-run artifact to consumers | Not moved |
| ESLint / TypeScript / Vite production build | All preserved in quality job; duplicate TypeScript compilation and consumer regeneration removed | Not moved |
| Locale coverage tests | Once with the full frontend suite; the CI ESLint step no longer runs the same test a second time | Not moved |
| GNU timeout happy paths, delayed startup plus real TERM/KILL, transient exhaustion, permanent/mixed failures, nonretryable configuration, tee failures | Kept in fast tooling tier | Portable replay complements these |
| `NativeDependencyTests.test_acquisition_timeout_and_forced_kill_retry_then_success` | Representative real signal coverage remains above | Full repeated update/download × TERM/KILL matrix |
| `NativeDependencyTests.test_exhaustion_preserves_last_exit_and_stops` | Non-signal transient exhaustion split into a fast test; delayed real signals remain | Repeated signal-exhaustion matrix |
| Entire `PortableNativeDependencyTests` inherited replay (11 methods) | Primary GNU policy assertions remain | All eleven tests, including fallback signal behavior |
| All other tooling/installer, binding CLI, logo and workflow guards | Kept in fast tier | Nothing moved |

The explicit tooling partition is **67 fast + 13 extended = 80** tests, disjoint
and complete. `tools/run-tooling-tests.py --tier fast|extended|all --list` lists
the exact inventory. Discovery failures fail every tier. The extended job is
functional tooling validation, not a benchmark: it uses the existing perf
workflow's daily 21:00 UTC / manual trigger and unchanged no-new-main skip rule.
No push/PR trigger is added there. Full local/release unittest discovery still
runs every test, and `--tier all` remains available for source changes or diagnosis.

The GNU non-signal exhaustion subcase was extracted from its mixed loop, not
deleted. Its exit code, two-attempt bound, diagnostic output and error assertions
remain fast. All former signal/permanent-error assertions remain in either tier.

## Validation and acceptance status

Focused cold reconciliation tests and the workflow/shard partition guards passed
before the layout's full validation. The PR validation record names the exact
follow-up head, all six executed frontend shards, fast and extended tooling
results, lint/typecheck/build and unverified native/browser gates. Hosted
correctness and the 300s end-to-end target remain coordinator acceptance gates.


### Wails CLI cache follow-up

The bindings job caches the pinned v3.0.0 CLI executable separately from the
setup-go module and build caches. The key fences the Go version, OS, architecture
and reviewed native image digest. The unpinned apt fallback builds the executable
again after clearing compiled Go artifacts. No module tree is overlaid. A cache
miss still installs the pinned CLI. Generated output now has its own conservative
exact-input cache, described below; hosted cold and warm timings and the
five-minute acceptance target remain to be measured.

A single sequential paired comparison on native macOS arm64 (Go 1.27.3,
Node 24.22.0, Vitest 4.1.11, two workers, generated bindings and copied
installed frontend dependencies) used source `6de444fd` and the existing
HistoryPage isolation. Both configurations passed all 161 files and 1,659
tests. Four shards took 36.82, 37.07, 41.69 and 41.46 seconds (157.04 seconds
sum); six took 36.65, 20.89, 25.87, 23.53, 24.02 and 23.71 seconds (154.69
seconds sum). These are one sequential sample per configuration, in that
order, with no runner setup or artifact transfer. The longest shard is only a
local parallel-wall proxy; these numbers do not establish hosted wall time,
runner cost, stability, or a five-minute workflow. Keep six shards until a
same-head hosted comparison captures queue, setup and aggregate job time.


### Generated bindings cache and independent comparison

The reviewed native-image path fingerprints all tracked and nonignored Go
files, Go module files, native source and headers, and conservative non-presentation
project files, including generator scripts, frontend runtime package/lock files,
CI configuration, registration and event code. It also asks `go list -deps` for
actual Go embedded files and native sources, including matches inside otherwise
excluded documentation and frontend trees. Source names, modes and SHA-256
contents, the pinned CLI bytes, Go and Node versions, and the build environment
form the exact key. Ordinary README, documentation and frontend presentation
changes do not invalidate it unless Go embeds or compiles the file. All Go files
remain included even in excluded directories. Changes to future embed directives
are captured by both the Go source and the Go tool's resolved file list.

Local module replacements, a workspace file, GOFLAGS, ignored Go/native inputs,
missing or symlinked inputs, a failed Go package inventory, unavailable tools,
and the unpinned apt fallback disable reuse. CI uses a clean checkout and does
not install frontend dependencies in the bindings producer. There are no prefix
cache restores. A matching archive is accepted only if the manifest has the exact
input key, generated file set, digests and required service marker. Missing,
extra or changed files regenerate; an immutable corrupt cache entry may cause
repeat misses until eviction. Cache errors also fall back. This is corruption
detection, not authentication: like other Actions caches it trusts writers in
the applicable GitHub cache scope.

A fresh generator result or validated exact-input hit feeds the same-run frontend
artifact. A second independent generation and byte comparison remains required
when Go sources, generator/runtime inputs or other non-presentation inputs
change; missing Git history fails closed to comparison. The daily/manual perf
workflow independently generates and compares even when the source is unchanged,
and macOS release packaging compares after its frontend build. Routine
presentation-only changes avoid the duplicate comparison; direct local
`check:bindings` remains available. Cache reuse is not evidence of deployed
behavior. On native Mac, the earlier full-checkout fingerprint took 0.31s, restored output took 0.06s,
and an independent warm comparison took 1.89s; a warm narrowed fingerprint
including Go package/embed discovery took 0.71s in one separate sample; a deliberately corrupted entry was
rejected and fresh warm generation took 1.64s. These measurements exclude
archive transport, and do not predict hosted performance. The output cache is
currently enabled only with the pinned native image. Its safety depends on
retaining the complete input inventory and a trusted CI cache scope.
