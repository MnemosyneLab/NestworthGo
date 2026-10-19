# C1 source revision/journal evidence — 2026-10-19

Historical evidence at the commits below is preserved. The subsequent [PR114 insert/retention review fix](performance-c1-insert-fix-2026-10-19.md) corrects UPSERT/IGNORE journaling and adds bounded automatic retention; use that follow-up for the current contract and new measurements.

This implements **PR-C1 tracking only**, from [plan §5](../design/performance-optimization-plan.zh-CN.md#5-事务版本和变更日志pr-c1c2c3依赖-a). It adds schema20 revision/journal metadata, database triggers and verification, without replacing full fingerprints, publication rechecks, review algorithms, CAS or no-op behavior. See the [complete source/field/writer and recovery audit](performance-c1-source-coverage-2026-10-19.md). C2/C3 and later stages await separate review. No merge or release is part of this delivery.

## Provenance and method

Measured code: **`dd394a11880ed2e9ecde7cb62644dfc57d020831`**, combining C1 implementation `d9edd20` with current main `d0eff3aeed1654d98c7acd1c6d1356332c485458` (PR111), including PR112. Main schema was rechecked as19 before allocating20. Later C1 commits add validation/reporting only. Main subsequently advanced to `63fc5e1c16f835f56d13869375f3755f4bfe0767` (PR113 derived-data cost adjustment fix) and was integrated for final combined tests. The SQLite production/benchmark code is unchanged from the measured commit; these measurements are not relabelled as PR113 performance evidence.

Run started 2026-10-19T06:38:56Z on Linux x86_64, kernel6.18.55, Intel Xeon Platinum8573C, Go1.26.0, modernc SQLite3.53.0, GOMAXPROCS4. Storage is this cloud container's `/tmp`/workspace environment. No live providers, real household, external backup or native app was used.

`TestFinancialSourceWriteAmplificationEvidence` runs two primitive write workloads, 30 independent paired samples each, all in one process, alternating path order (15 before-first/15 tracked-first). Each path is opened from a fresh `VACUUM INTO` clone of one synthetic source containing one household, one instrument, one history origin and its snapshot state (four tracked source insertions). The exact source/financial IDs and financial data are shared across paths.

Path0 is an **isolated counterfactual**: after normal verified opening, drop only C1 triggers from that test clone. All financial schema, existing reconciliation triggers, indexes and code remain. Path1 retains C1. The disabled clone is deliberately unverifiable and is neither a shipped switch nor supported rollback. This isolates trigger bookkeeping cost; it is not a benchmark of an older executable/migration, total startup, or full user command.

Each timed interval includes BeginTx, 100 Exec calls and Commit. It excludes fixture/cloning/open/DDL, initial checkpoint, memory sampling, final checkpoint and full fingerprint oracle. Quotes:100 exact-decimal quote inserts in one transaction. Catalog:100 changes to the household name in one transaction (conservative invalidation case). Transactions use existing immediate mode, WAL and managed connection settings. Each clone is checkpointed immediately before timing. Allocation deltas cover the timed Go operation, not SQLite native memory or peak RSS. WAL size is sampled after commit before checkpoint; DB file bytes after checkpoint. Journal text bytes count selected journal text fields, excluding record/page/index overhead.

Connections/pages are freshly opened for each sample, but the process, driver and OS filesystem caches are warmed by setup and prior samples. **These are not cold-cache or established steady-state warm-connection distributions.** There is no controlled OS cache eviction. The process runs without concurrent test/build/race/profile work during the measurement. All 120 full financial fingerprints (both paths, both workloads) are consumed and compared outside timing. Every pair agrees exactly; metadata does not enter the fingerprint.

## Observed distributions and write amplification

Nearest-rank median is sorted rank15; p95 is rank29 for n30. These finite sample distributions are not production p95/SLA estimates.

| 100 writes/transaction | C1 disabled median | C1 tracked median | C1 disabled p95 | C1 tracked p95 |
| --- | ---: | ---: | ---: | ---: |
| Quote inserts | 2.975ms | 16.856ms | 4.098ms | 22.509ms |
| Household catalog updates | 3.584ms | 13.707ms | 5.031ms | 17.168ms |

| Scenario/path | SQLite total_changed rows | Added journal rows | Journal text bytes | WAL before checkpoint | DB after checkpoint | Median Go allocated bytes / allocations |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Quotes / disabled | 100 | 0 | 0 | 65,952 | 1,175,552 | 44,056 / 1,314 |
| Quotes / tracked | 300 | 100 | 50,800 | 144,232 | 1,290,240 | 44,352 / 1,317 |
| Catalog / disabled | 100 | 0 | 0 | 4,152 | 1,175,552 | 40,056 / 1,314 |
| Catalog / tracked | 300 | 100 | 25,800 | 65,952 | 1,220,608 | 40,056 / 1,314 |

For these singleton-household writes each actual source change adds one revision update and one journal insertion: **3× SQLite row changes**. Quote journal payload is508 text bytes/change; catalog258. Tracked median transaction time is about5.67×/3.82× respectively; absolute median increases13.881ms/10.122ms per100 writes. This foundation has material write overhead and makes **no compute speedup claim**. Existing quote/index/cache optimizations are unchanged.

These DB bytes include the complete schema, financial tables/indexes and metadata allocation/free-page behavior; they are not a claimed journal-only or secondary-index size. No new secondary journal index exists. WAL bytes are transient and are not added to DB bytes to claim durable growth. No linear extrapolation to large import latency, retained-history size, SSD durability, peak memory or production commands is justified. Retained journal growth remains an open rollout concern: C1 provides transactional floor/prune but no automatic retention policy.

## Correctness and validation

The [raw evidence directory](performance-c1-2026-10-19/) contains the full 120 samples, [CSV](performance-c1-2026-10-19/write-samples.csv), [summary JSON](performance-c1-2026-10-19/write-summary.json), command/provenance information and validation logs/checksums. The write benchmark fails on unexpected changed-row/journal counts or different full financial fingerprints.

C1 tests cover every source column/operation in the reduced trigger harness, plus real schema decimal edits, OLD/NEW IDs/times, child effective-time resolution, FK deletion, rollback, concurrent writers, identical DML, replay classification, recursive-trigger-off REPLACE fallback, missing/tampered trigger/PRAGMA rejection, journal gap detection, migration/recovery IO failure/concurrent upgrade rejection, readonly behavior, consistent snapshots, restore/crash-resume/restart epoch rotation, transactional prune/floor, finance-only fresh import boundary and JSON v2 exclusion/no writes. Configuration canary tests ensure no key/value/hash enters source scopes. Full application tests compare no-op, append/update and correction captures, typed full-recompute evidence and durable episode/daily lifecycle rows against identical isolated C1-disabled clones; only fresh random episode/event IDs are normalized by the existing strict oracle.

An injected-clock test proves a preexisting future event changes full financial capture between T−1ns and T while source revision/epoch and closed day remain equal. C1 intentionally introduces **no** temporal certificate or no-op shortcut. Full fingerprints/rechecks remain authoritative, preserving §5.4. Future C2/C3 still require their complete temporal/clock rollback, transaction/CAS, rule/state, restore and concurrent-write proof gates.

Validation outcomes and native environment limitations are recorded below after the final run. No test budgets or assertions were relaxed. Existing legacy financial-row preservation tests exclude only the two newly added derived metadata tables from old-schema comparisons; all financial rows/fields remain compared.

Final local outcomes:

- Full `go test ./... -count=1`: application250.818s, SQLite51.903s, backup4.988s, MCP97.083s and all other non-native packages passed **except** two continuous-backup legacy fixtures. Those fixtures were repaired without weakening financial comparisons; full continuous-backup rerun passed33.093s. The original full log is retained. The desktop command could not compile because GTK/WebKit/libsoup headers are missing.
- Expanded paired no-op/append/correction oracle, export and migration checks: passed9.154s. Focused C1 race suite: SQLite144.818s, backup19.888s, application86.630s, all passed. The all-column fixture was then reused sequentially to avoid recompiling identical trigger DDL46 times; all column/operation/assertion coverage is retained, and its separate race rerun passed5.609s. No budgets changed.
- Final integration with PR113 at main `63fc5e1c16f835f56d13869375f3755f4bfe0767`: combined rebuild+C1 oracle tests passed2.238s; PR113 rebuild regressions under race passed9.174s; `go vet ./internal/...` passed. These combined tests are separate from the earlier immutable benchmark measurements.
- `go build ./internal/...`, backend vet, gofmt and `git diff --check`: passed. Installed frontend eslint: passed via direct executable; pnpm wrapper setup failed before lint, preserved separately. Native build, whole-module vet and bindings generation are blocked by missing Linux GTK/WebKit headers. No bound service/DTO/frontend changes are introduced, and no generated files are committed. Frontend typecheck/tests/build dependent on binding generation and macOS/native QA were not completed locally; canonical CI remains required.

C1 is ready for **draft review**, not default token/no-op activation or rollout. Outstanding decisions include automatic retention and write-cost acceptability. C2 fixed-cutoff shadow equivalence/concurrency/time proofs, C3 transactional CAS/temporal-certificate switch, and later dirty-suffix/shared-fact/GUI-MCP scope stages remain separate work. No merge, release, provider access, real ledger or cloud backup operation was performed for C1.
