# Review performance P0: PR-A baseline and PR-B bounded evidence windows

Implementation contract: [performance plan](../design/performance-optimization-plan.zh-CN.md),
sections 3, 4, 5.4 and 9. Base: `bca85a8b5418dd45137a766ab14b3ef10f5521ad`
(main including #108 and #109). Measured implementation:
`0a691b3475a2bb0d799c56159f3bc0849eeda917`.

## Delivered scope and rollout gate

This draft contains the **PR-A baseline/oracle foundation** and **PR-B bounded
queries** within P0. It does not start C1 or change schema 19, source tracking,
publishing transactions, provider behavior, financial precision or wire APIs.

The production constructor retains the original key-first ranking query.
`sqlite.NewRepository(db).WithBoundedReviewWindow()` selects the candidate;
`WithReviewWindowBaseline()` selects the retained old SQL, and
`WithFullReviewRecompute()` independently recaptures authoritative sources and
loads no reusable daily projection inputs. Options return repository copies;
there is no mutable process-global production switch. All financial source
fingerprints and publication rechecks remain active in every mode.

**Default activation remains blocked for parent review.** Native Apple Silicon
latency/RSS and the complete workload matrix remain unmeasured. Corrected final
synthetic distributions below are evidence for review, not an automatic waiver
or authorization to enable the candidate.
No 3s cold / 500ms warm product target is declared met.

The loader selects the union of current rule targets, previously evaluated
instruments and unresolved episodes, including closed positions. Each request
contains at most 100 instruments and 14 scalar parameters (114 parameters),
within one existing capture transaction. A correlated primary-key prefix seek
selects the last W **stored date slots**, including false and unknown. Complete
keys then load input/result JSON; latest streak counts are anchored to the
maximum selected stored date. Sparse dates remain sparse and the existing
planner falls back to full reconstruction when the required civil-date interval
is incomplete. No date arithmetic substitutes for stored-slot selection.

`CROSS JOIN` fixes the small key set before the payload lookup. Without this
constraint SQLite could reorder the payload join into a history-prefix scan.
The per-instrument prepared-seek alternative is retained in tests. The chunked
variant reduced request overhead in the reference workloads without combining
all rules into a single query or adding an index.

The full oracle found and fixed one existing discrepancy: newly emitted daily
`PolicyWindow.ConsecutiveCount` started at the bounded overlap rather than the
complete prior streak. The summary already carried the complete count. Emitted
days now carry that same prefix, allowing exact day-by-day oracle comparison.

## Harness and correctness evidence

The reference R30/R365 workloads remain 1 account, 100 holdings/instruments,
20 rules, 100,000 generated + 100 baseline quotes, 50,000 cash-only activities,
and 120/1,460 FX references. Cash-only background activity is not trade replay.

The new fixture controls accounts, holdings, instruments, rules, dates, generated
quotes, cash-only activity and mixed economic activity independently. Shared
instruments across accounts are supported. Mixed commands use the existing
financial command path: buy/sell, cash transfer and position-cost adjustment;
`trade`, `transfer`, `cost` and `mixed` can be selected separately. Funding adds
one synthetic contribution per account, outside the requested mixed count.

Financial source values and dates use fixed arithmetic and a fixed clock.
Opaque UUIDs are generated once per run and copied with each SQLite `VACUUM INTO`
snapshot; byte-identical UUIDs across separate runs are not claimed.
Each paired sample and each scenario begins with the same authoritative facts,
handling state and published projection. Baseline/candidate order alternates;
full publication runs use independent clones with fresh Service/memo and SQLite
connections. "Warm" here means an existing durable projection, not OS cold pages.
Acknowledgement and permanent
suppression are seeded through the public application commands. No provider
network, real ledger, user settings or cloud backup is used.

The ordinary test suite includes:

- Exact baseline/candidate captures, selected input/result/coverage/fingerprint
  equality and independent full reconstruction for no-op, clock-only, append
  and correction, with mixed commands and shared instruments.
- Every newly emitted day's typed financial input, predicate result and policy
  window; full lifecycle summaries, trigger evidence, epochs and handling;
  reusable evidence compared with independently reconstructed full days.
- All fields in durable control/input/result/epoch/episode/evaluation/event rows
  compared after baseline/candidate publication. Only newly generated episode
  UUIDs are mapped by logical identity and newly generated event UUIDs by event
  key; existing episode/event IDs, links, event keys, payloads, timestamps and
  handling remain checked. A tampered existing event ID is explicitly rejected.
- 30/365/1,095 stored days, four input versions, absent dates including Apia's
  nonexistent 2011-12-30, false/unknown/true, absent/mismatched results,
  exact quantity text, long streak anchors, target/evaluated/open-episode union,
  100-ID chunk boundaries, and cancellation.
- A removed trailing date forcing the application planner back to an independently
  captured full oracle rather than silently accepting a shorter date window.

Existing deletion-without-generation-bump, correction, epoch, acknowledgement,
midnight, cancellation and publication-failure tests remain unchanged. The
existing GUI/MCP application use case and V2 review behavior remain shared.
This stage does not claim new scoped GUI/MCP, temporal-certificate, crash/restart
or journal coverage for the later C–H implementations.

## Measurement protocol and environment

Application/reference evidence is run serially from compiled test binaries at
the immutable implementation commit above; the corrected SQL fixture uses the
test-only commit identified below. All timing precedes builds, race tests and
frontend checks.
Binary checksums and raw logs are retained in the [evidence directory](performance-p0-2026-10-19/).
Earlier exploratory measurements are not substituted for final distributions.

Environment: Linux amd64, Intel Xeon Platinum 8573C, Go 1.26.0,
modernc SQLite 3.53.0, `GOMAXPROCS=4`; cgroup CPU quota 400000/100000 (4 CPUs),
16 GiB memory limit. Synthetic databases live on `/tmp` tmpfs. This is not
native Mac or physical-disk performance; no OS cold-page claim is made.

- Capture timing includes financial source reads, source fingerprinting,
  control/rules/handling reads, all evidence iteration, scan and typed decode.
- Driver **Query-to-Rows.Close** includes consumption, Scan and JSON work performed
  during the Rows lifetime. Returned row counts are not visited keys or physical
  I/O. Next/Scan/decode decomposition remains available in the existing profile mode.
- Stage fingerprint/serialization/read-only recheck timings are independent
  diagnostics. They must not be added again to capture or atomic publish, which
  already include their own fingerprint/recheck/serialization work.
- Atomic publish includes source recheck, serialization, writes and commit.
  Paired end-to-end reconcile includes capture through successful atomic publish.
- Paired timing excludes fixture creation, cloning and oracle comparisons.
  Allocation bytes/counts cover measured reconciliation; process peak RSS covers
  the **whole test lifetime**, including fixture generation, clones and oracle.
  Neither is mislabeled as the other.
- No-op, clock-only, append and correction distributions are separate. Samples
  use fixed workloads; the original continuous-growth warm modes remain available
  but are not mixed into these distributions.
- Each 30-sample nearest-rank p95 is sorted sample 29. Reference stage diagnostics
  use n=1 and have no p95 claim. Fresh projection is application-cold, excluding
  process startup and fixture generation; a new process is not OS cold pages.

The SQL probe uses a non-deterministic test-only function in the date-key WHERE
predicate. It reports predicate evaluations, not VM instructions or disk reads;
measurements use unprobed SQL. With two instruments, W=7 and four input versions,
old/new evaluations were 216/50 (30 days), 2688/50 (365) and 8080/50 (1095), with
14 returned rows in each case. EXPLAIN confirms the covering primary-key prefix
for date selection and complete date keys for payload lookup. Old versions still
increase key work because `input_policy` follows `on_date` in the existing PK;
this stage makes no claim of independence from unbounded version count.

## Final results

### Actual-schema reference stage diagnostics (n=1 per mode)

Evidence reads are a subset of capture. Publish is a separate phase with its
own source recheck; no sums include the standalone diagnostic recheck.

| Workload / mode | Capture (ms) | Evidence reads (ms) | Atomic publish (ms) | Process peak RSS (MiB) | DB+WAL+SHM bytes |
| --- | ---: | ---: | ---: | ---: | ---: |
| R30 / baseline | 3614.079 | 687.492 | 4931.787 | 661.5 | 295,542,760 |
| R30 / bounded | 3535.769 | 246.739 | 3924.205 | 619.5 | 295,542,760 |
| R365 / baseline | 9129.868 | 5990.280 | 3955.205 | 2396.1 | 1,424,376,480 |
| R365 / bounded | 3019.708 | 284.636 | 3755.581 | 2411.3 | 1,424,376,480 |

Both modes returned 6,000 evidence rows using 20 evidence queries. With the
same schema and daily row counts, final file totals are equal; there is no new
index or schema write cost. Publish code is unchanged; its n=1 timing differences
are not attributed to the evidence query. Page-level and checkpoint-separated
DB/WAL/SHM measurements are not supplied by these stage runs.

Initial projection times (n=1, not OS/process cold):
- R30 baseline: 14.131757684s.
- R30 bounded: 13.832367439s.
- R365 baseline: 1m45.624452565s.
- R365 bounded: 1m35.768631019s.

### Paired S workload (n=30 per scenario and mode)

2 accounts, 10 holdings/instruments, 20 rules, 30 days, 310 quotes, 120 FX
references, 100 mixed commands plus two funding contributions. Each scenario
starts from the same cloned projection with an acknowledged episode and a
permanent suppression. Median and p95 below both use nearest rank.

| Scenario | Baseline full reconcile median / p95 (ms) | Bounded full reconcile median / p95 (ms) | Baseline capture median / p95 (ms) | Bounded capture median / p95 (ms) |
| --- | ---: | ---: | ---: | ---: |
| no-op | 177.786 / 215.080 | 143.972 / 165.873 | 91.936 / 98.992 | 58.968 / 69.199 |
| clock-only | 234.451 / 258.688 | 199.924 / 242.644 | 93.284 / 98.186 | 60.852 / 65.251 |
| append | 699.150 / 1055.302 | 711.631 / 994.101 | 24.499 / 36.979 | 24.244 / 27.820 |
| correction | 805.578 / 853.356 | 805.301 / 865.771 | 24.009 / 25.707 | 23.701 / 27.554 |

No semantic differences were observed in the 120 paired scenarios. At measured
commit `0a691b3`, event row UUIDs were normalized; subsequent test-only commit
`cf4e537` strengthens the current oracle to preserve existing event IDs and
explicitly reject replacement of an existing identity. Query/production code is
unchanged. That stronger gate is verified separately from timed samples. Changed
sources returned zero reusable rows in both modes: append rewrote 6,200 pair-days,
correction 6,000. Their small timing differences are variation in the unchanged
full path, not evidence of incremental-update acceleration. Correction full p95
rose 1.45%, below the proposed 10% threshold in this run.

The single initial S projection took 702.542 ms. Process peak RSS for the entire
paired test, including generation/cloning/oracles, was 146.9 MiB; this is not a
per-mode RSS comparison. Per-reconciliation allocated bytes, allocation counts,
SQL queries/executions/writes and affected rows are in the raw sample log.

### Corrected reduced SQL fixture (n=30 per mode)

The final micro fixture mirrors production input/result and evaluation-state
primary keys. An earlier exploratory fixture had an extra evaluation-state
index; its timings are excluded. The final micro run is at test-only correction
commit `0eb07a0e7800fa115630ae8dfdc75ce50a9ae278`; production implementation is
identical to the reference/application measurements at `0a691b3`.

| Stored days | Baseline median / p95 (ms) | Bounded chunk median / p95 (ms) | Per-instrument trial median / p95 (ms) |
| --- | ---: | ---: | ---: |
| 30 | 0.807 / 1.118 | 0.828 / 1.069 | 0.697 / 0.960 |
| 365 | 3.273 / 4.687 | 0.898 / 1.184 | 0.764 / 1.093 |
| 1095 | 8.447 / 11.362 | 0.898 / 1.643 | 0.798 / 1.432 |

All returned typed evidence maps are identical. Instrumented key predicate
evaluations remain 216/50, 2688/50 and 8080/50 (baseline/bounded); these probes
are not included in timings. This in-memory SQL fixture measures two instruments
and four input versions, not the full reference workload. Its per-instrument
trial does not include the 2,000 request overhead observed in the actual R30
trial; production candidate selection uses chunks.

Final measured synthetic p95 regressions are below 10%; this does not authorize
default activation. Earlier unreviewed exploratory results varied, and native
Apple Silicon distributions, workload-specific peak RSS and the remaining matrix
are still missing. The default remains the original query until parent review.

### Analysis baseline only

On 2 accounts / 10 instruments / 30 days / 100 mixed commands, application
SQL → ensure snapshots → kernel → calendar DTO took 135.794 ms for the first
sample and 59.484 ms from a clone with durable snapshots and a fresh Service/memo.
Outputs were identical. Query counts were 494 / 85 and write counts 421 / 0.
These are single cold/warm samples with different snapshot-maintenance work,
not a before/after optimization or p95 claim. Whole-test process peak RSS was
34.8 MiB. G quote batching and H scoped hydration remain unimplemented.

### Local verification

| Gate | Result |
| --- | --- |
| New/relevant review race scope | Passed: application 134.955s, SQLite 10.353s; `-race -count=1 -timeout 10m -p 1 -parallel 4 -run '^TestReview'` |
| Full `go test ./...` | Failed: native `cmd/nestworth` build dependencies, plus the dense-series deadline below; other packages passed |
| Full race attempt | Application cumulative 600.066s timeout, native build blocker; stopped the remaining broad attempt after those failures (exit -15, 1344.409s); not a complete or passing full race run |
| `go vet ./internal/...` | Passed |
| Full vet / canonical desktop Go build | Blocked by missing GTK4, WebKitGTK6, libsoup3 and GIO development packages |
| gofmt / diff check | Passed |
| User-skill checker / Python tool tests | Passed; 53 tool tests |
| Frontend dependency install / lint | Passed with Node 24.19.0 and pinned pnpm 12.2.1 |
| Bindings generation | Failed on missing GTK4/WebKitGTK6 after the writable checksum-cache fix |
| Binding freshness / frontend typecheck, tests, build | Blocked by failed binding generation; not claimed passed |

Exact ordinary failure: `TestHistoricalOverviewDenseEntrySingleAndComparison/series-endpoint-equivalence`
returned `local trend exceeded its computation deadline` at 10.00s; the complete
dense case took 50.32s in the broad run. No deadline, fixture size or assertion was
changed. The race application process hit its **cumulative** 10-minute cap while
`TestHistoricalSeriesDefaultFactLimitBeforeHydration` had been running for 34s,
before reaching the review scope. No detector report was observed; this does not
prove the uncompleted broad remainder race-free. CI's existing four application
shards remain required.

Same-environment isolated comparison, one fresh process per revision, same four-CPU
quota/Go 1.26.0 and unchanged production 10-second series deadline:

| Revision | Dense case | Series phase | Result |
| --- | ---: | ---: | --- |
| Current branch (`cf4e537`, production code at `0a691b3`) | 29.97s | 5.35s | Passed |
| Unchanged main `bca85a8b5418dd45137a766ab14b3ef10f5521ad` | 31.13s | 5.82s | Passed |

The deadline failure did not reproduce in isolation on either revision. This is
consistent with load sensitivity; it is not a full main-suite comparison and
does not turn the original broad failure into a pass. Historical series production
code is unchanged by this draft. The broad attempts and isolated/scoped results
are retained separately in [local evidence](performance-p0-2026-10-19/).

Local dependency installation cannot acquire root privileges, so native headers
remain unavailable. Initial binding generation also hit the read-only default Go
checksum cache; the final attempt uses `GOPATH=/tmp/nestworth-gopath` and retains
checksum verification. No generated bindings or frontend bundle is committed.

## Reproduction and remaining stages

Use an isolated synthetic temporary root and writable Go caches, as in the
[implementation plan](../design/performance-optimization-plan.zh-CN.md#3-可复现基线和工作负载矩阵pr-a).
Representative commands (run serially):

```sh
NW_REVIEW_PERF_EVIDENCE=1 NW_REVIEW_PERF_MODE=stages365 \
  NW_REVIEW_PERF_WINDOW=baseline GOMAXPROCS=4 \
  go test ./internal/application -run '^TestReviewFullPipelinePerformanceEvidence$' -count=1 -v
NW_REVIEW_PERF_EVIDENCE=1 NW_REVIEW_PERF_MODE=stages365 \
  NW_REVIEW_PERF_WINDOW=bounded GOMAXPROCS=4 \
  go test ./internal/application -run '^TestReviewFullPipelinePerformanceEvidence$' -count=1 -v
NW_REVIEW_PERF_EVIDENCE=1 NW_REVIEW_PERF_ACCOUNTS=2 \
  NW_REVIEW_PERF_INSTRUMENTS=10 NW_REVIEW_PERF_RULES=20 \
  NW_REVIEW_PERF_QUOTES=300 NW_REVIEW_PERF_ACTIVITIES=0 \
  NW_REVIEW_PERF_MIXED_ACTIVITIES=100 GOMAXPROCS=4 \
  go test ./internal/application -run '^TestReviewPairedPerformanceEvidence$' -count=1 -v
NW_REVIEW_PERF_EVIDENCE=1 GOMAXPROCS=4 go test ./internal/infrastructure/sqlite \
  -run '^TestReviewReusableWindowEquivalent$' -count=1 -v
```

Optional paired fixture overrides: `NW_REVIEW_PERF_ACCOUNTS`, `INSTRUMENTS`,
`HOLDINGS`, `RULES`, `DAYS`, `QUOTES`, `ACTIVITIES`, `MIXED_ACTIVITIES`,
`MIX_KIND` (each with the same `NW_REVIEW_PERF_` prefix). Holdings default to
instrument count; repeated instruments occupy distinct accounts. Samples default
to 30; `NW_REVIEW_PERF_SAMPLES` may request a smaller **explicitly reported** n.
Unknown original evidence modes and unknown window/mixed-kind values fail.

The parameterized harness can construct the larger account, activity, quote,
date and rule axes from the design. This draft does not claim execution of their
entire Cartesian product, million-quote capacity, all activity variants at
5k/50k, 500-component analysis SQL workload, every mutation/fault scenario or
continuous-growth 30-day distribution. Those remain separately scheduled evidence;
existing kernel-only analysis evidence is not reclassified as end-to-end.

Native launch tests must use the existing `com.nestworth.dev` / `Nestworth`
identity with verified synthetic database/settings overrides. No desktop app was
launched during this work.

Next action is parent architecture/performance review of this draft and the
rollout gate. Do not merge, release or begin dependent schema/protocol work here.
Subsequent approved stages remain C1 writer/trigger/migration audit, C2 shadow
tokens, C3 transactional CAS and complete §5.4 temporal certificates/rollback,
D1/D2 dependency and dirty suffix, E1/E2 shared facts, conditional F after profiles,
and independent G quote batching / H account hydration and proven transfer closure.
F4/F15 secrets remain excluded. No source rechecks may be removed and no no-op
shortcut enabled before those proof gates.
