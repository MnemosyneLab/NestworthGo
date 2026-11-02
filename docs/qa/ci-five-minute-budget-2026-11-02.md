# CI correctness and five-minute budget (2026-11-02)

The five-minute end-to-end target remains an acceptance criterion. The observed
hosted runs have **not** met it (517s initially, 402s after the first redesign). This record separates measured evidence
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
  application-2/remainder dependency. Five Node-only test shards keep two workers per runner. HistoryPage, an
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
| All frontend tests, including money, navigation, focus and interrupted submissions | All five shards; no exclusions | Nothing moved |
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
follow-up head, all five executed frontend shards, fast and extended tooling
results, lint/typecheck/build and unverified native/browser gates. Hosted
correctness and the 300s end-to-end target remain coordinator acceptance gates.
