# PR114 actual-insert journaling and bounded retention

Review follow-up on PR-C1. Measured implementation: **`0a7c3061adbd12d8bec5246c261388c28659cfbe`**, based on PR114 head `2f8b4ecfc8829a087472f9e8b3c8ce81c64f27d4` and main63fc5e1/PR113. The [original report](performance-c1-2026-10-19.md) and every original raw artifact/checksum remain intact. This report supersedes its unlimited-retention decision; it does not replace or relabel those observations. Full fingerprints, capture/publication rechecks, existing financial authorities, budgets and oracle remain unchanged. No C2/C3 consumer, no-op shortcut, merge or release.

## Conflict correctness

The real-schema regression reproduced the review finding in both `ConfigurationRepository.SaveConfiguration` and quote UPSERTs: identical UPSERT emitted one phantom insert; changed UPSERT emitted insert+update; ignored duplicate insert emitted one phantom insert. [Before-fix log](performance-c1-insert-fix-2026-10-19/before-conflicts.log).

All source INSERT triggers now run **AFTER INSERT**. UPDATE and DELETE retain BEFORE timing, preserving OLD scopes/effective times and parent lookup before deletion. Fresh insertion emits one entry, identical UPSERT/ignored duplicate emit zero, changed UPSERT emits one update. With recursive triggers ON, REPLACE emits DELETE and actual INSERT; with OFF it still emits actual INSERT and conservatively invalidates the whole household. Source scope remains `household_all`, so an absent OLD callback is never evidence for narrowed reuse. Failed journal publication rolls back the source and revision. Explicit guards additionally reject silently ignored revision/journal maintenance; an outer OR IGNORE policy cannot quietly admit untracked source writes. The verifier's exact generated definitions now require AFTER INSERT and the retention trigger; old unmerged prototype v20 files are rejected rather than repaired. No released schema20 migration is being changed.

Real-schema tests cover both sources, fresh/identical/changed/ignored inserts, recursive ON/OFF REPLACE and rollback, forced ABORT/ignored publication, journal failure on insert/update/delete, expected trigger timing, OLD identity/time/parent capture and cascades. The all-column matrix and full application no-op/append/correction typed oracle/lifecycle comparisons are retained.

## Bounded policy and trade-off

Choose a **row bound**, independent of the wall clock: retain4096 journal entries per household at a pruning boundary, with a256-revision cadence and maximum4351 retained entries between boundaries. Current schema enforces one household/database. At revision4352, for example, floor advances to256 and rows≤256 are removed; remaining revisions257..4352 are contiguous. Later boundaries advance the floor monotonically by256 unless explicit maintenance/restore has already advanced it further.

A separate AFTER INSERT journal trigger executes maintenance only when revision>4096 and revision%256=0. It updates the floor then deletes the prefix using the existing `(household_id,revision,change_ordinal)` PK. Floor advancement, deletion, source DML and its revision/journal insertion are one SQLite statement/business transaction. Guards reject ignored floor updates or retained rows below the floor, rolling the entire source statement back. Beyond the initial window this amortizes to approximately one journal-row deletion per logged source change, plus one floor update per256. No per-source-change journal count/full-history scan, background process, timer, secondary index or startup financial-history backfill is introduced.

4096 is deliberately a bounded diagnostic/shadow window, not a promise that a planner can replay every unattended import. It allows thousands of individual source-row changes while keeping the incremental window small. A burst exceeding the window advances the floor and forces any lagging consumer to full recapture. C1 has no consumer; C2/C3 must honor epoch/version/floor and retain their independent temporal/CAS proof gates. This accepts more future full rebuilds in exchange for bounded new metadata overhead. The maximum is rows, **not a byte cap or latency SLA**: scope width varies. Representative retained journal text is about2.07MB below; SQLite rows/indexes/free pages and WAL are separate costs. Financial history and the preexisting reconciliation journal are unaffected and can grow normally.

Verification counts only a LIMIT4352 prefix per household and rejects retained count/range mismatches or a revision-floor span above4351. MIN/MAX seek the journal PK. Exact trigger checks ensure the policy is installed. This bounds the added journal verification work, not the entire existing startup integrity/financial verification process. Ordinary opens do not scan/prune source history or rotate epochs. Restore still rotates the installed live epoch, advances floor to revision and clears the journal before resume. The restore test now covers both an empty/small journal and a previously pruned4608-write history. Reopen preserves the retained floor/epoch. Rollback and injected ABORT/IGNORE pruning failures preserve source value, revision, old floor and the full retained prefix. A stale-revision test explicitly demonstrates revision<floor; C1 does not claim to implement a token consumer.

## Same-workload paired n30 rerun

The primitive benchmark harness is unchanged from the original run. It was compiled before measurement, then run alone at07:03:09Z in the same cloud environment (Linux x86_64, Xeon Platinum8573C, Go1.26.0, SQLite3.53.0, GOMAXPROCS4). No concurrent builds, tests, race or profiles ran during measurements. Thirty independent clone pairs/workload, alternating order15/15, one process. Each timed transaction includes BeginTx,100 Exec calls and Commit; excludes clone/open/DDL/checkpoints/memory sampling/full financial fingerprint oracle. Path0 disables only C1 triggers on an isolated verified-open clone, path1 retains them. Both paths preserve all original financial/reconciliation schema and indexes. The disabled clone is not a supported production mode. Fresh connections with process/OS caches warmed by setup: neither controlled cold cache nor steady warm-connection measurements.

| 100 writes/transaction | Disabled median | Tracked median | Disabled p95 | Tracked p95 |
| --- | ---: | ---: | ---: | ---: |
| Quote inserts | 2.735ms | 29.320ms | 3.719ms | 38.312ms |
| Catalog updates | 3.538ms | 22.162ms | 4.240ms | 29.233ms |

Nearest-rank ranks15/29 of30. These samples are not production p95 estimates. All60 paired full financial fingerprints agree exactly outside timing. Changed-row counts remain100→300:100 source changes,100 revision updates,100 journal inserts. These small clones do not cross the pruning threshold. The additional publication guards and journal retention trigger add measurable write overhead; this is **not** a speedup claim. Comparing old-run tracked timings with these is descriptive; the controlled comparison is each run's own disabled/tracked pairs.

| Scenario/path | Journal rows/text bytes added | WAL after commit, before checkpoint | Complete DB after checkpoint |
| --- | ---: | ---: | ---: |
| Quotes / disabled | 0 /0 | 65,952 | 1,228,800 |
| Quotes / tracked | 100 /50,800 | 144,232 | 1,343,488 |
| Catalog / disabled | 0 /0 | 4,152 | 1,228,800 |
| Catalog / tracked | 100 /25,800 | 65,952 | 1,273,856 |

Total DB file bytes include financial data/schema/index/free pages and are not journal-only bytes. WAL is transient, never added to DB bytes as durable metadata size. [Raw120 samples](performance-c1-insert-fix-2026-10-19/write-amplification.log), [CSV](performance-c1-insert-fix-2026-10-19/write-samples.csv), [summary](performance-c1-insert-fix-2026-10-19/write-summary.json).

## Representative end-to-end batches

These are **single samples per step**, run serially after the paired run and before validation. They include complete production persistence/service calls, validation, queries/Rows consumption, source/binding/slot/dirty-generation writes and commit. Setup, checkpoints, verification and financial fingerprint checks occur outside timing. All providers/data are synthetic; fake responses have no network latency. These results do not estimate a distribution, cold/warm production latency or provider latency. No baseline executable comparison is claimed for these additional workloads.

`Service.RefreshAll` resolves100 instrument targets plus one FX pair using the existing fake provider. The second call returns cached status for identical observations but still makes101 fake provider calls; cached status is not proof that provider calls were skipped.

| Refresh step | Elapsed | SQLite changed rows | Source revision delta | WAL bytes | DB growth |
| --- | ---: | ---: | ---: | ---: | ---: |
| Initial100 instrument+1 FX | 59.275ms | 303 | 101 | 2,912,872 | 122,880 |
| Identical repeat | 76.106ms | 101 | 0 | 0 | 0 |

[Refresh raw log](performance-c1-insert-fix-2026-10-19/provider-refresh.log). Both calls return all101 expected statuses; exactly100 instrument quotes persist. The identical repeat's affected/matched SQL rows are not101 financial changes and publish no source events.

`CommitInstrumentHistory` receives2304 daily observations/page (fixed exact price100.0001), three successive synthetic pages. Each page is immediately repeated with identical fetched time/values, exercising real provider-binding/observation-slot UPSERTs, not SQL written solely for a benchmark. Calls include canonical-slot/coverage resolution and dirty generation. At each repeat the complete authoritative historical fingerprint and revision remain equal, with no new quote/revision. All pages and retained ranges pass the full database verifier.

| Page/step | Elapsed | SQLite changed rows | Source revision delta | Retained rows /text bytes | Floor | Total quote/slot pairs | DB growth |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Page0 import | 2.667s | 14,342 | 4,609 | 4,107 /2,073,704 | 512 | 2,304 | 5,476,352 |
| Page0 identical repeat | 1.552s | 2,304 | 0 | 4,107 /2,073,704 | 512 | 2,304 | 0 |
| Page1 import | 2.633s | 18,454 | 4,609 | 4,108 /2,074,093 | 5,120 | 4,608 | 2,273,280 |
| Page1 identical repeat | 1.420s | 2,304 | 0 | 4,108 /2,074,093 | 5,120 | 4,608 | 0 |
| Page2 import | 2.667s | 18,454 | 4,609 | 4,109 /2,074,714 | 9,728 | 6,912 | 2,289,664 |
| Page2 identical repeat | 1.472s | 2,304 | 0 | 4,109 /2,074,714 | 9,728 | 6,912 | 0 |

[Import raw log](performance-c1-insert-fix-2026-10-19/provider-import.log) includes absolute DB bytes and WAL:5.55–5.67MB after new pages;119,512 bytes on identical retries. An unchanged UPSERT can still do SQLite work/WAL without a financial/source change. Tracked changed-row totals on new pages include amortized prefix deletes/floor updates, so the primitive3× figure does not describe these batches. Complete DB grows because authoritative quote/slot history grows; retained journal text stays near2.07MB and row counts below4351. No claim of a constant full database file size or zero repeat cost.

## Validation and review boundaries

Validation results are added below after completion. Original failure evidence and all old benchmark artifacts remain preserved. Automatic bounded retention now runs for all migrated databases; “shadow” means no source-token consumer, **not** zero write overhead. Retention parameter acceptance and measured write-cost acceptance remain parent review decisions. No merge/release or downstream phase activation is authorized.

Final validation:

- New conflict/retention/migration/every-column checks pass; expanded restore/application oracle checks pass. Focused race passed: SQLite407.204s, backup128.950s, application102.921s. The extra stale-floor full-fingerprint/recompute oracle passed under race10.428s. All original budgets remain intact.
- Complete `go test ./internal/... -count=1` passed SQLite88.216s, backup13.594s, continuous backup85.064s, marketdata4.917s, MCP138.882s and the other backend/Wails packages. Its application package failed **only** two unchanged10-second timing gates while concurrent race work was running (cold3Y12.453s; dense trend deadline10s). [Original complete log](performance-c1-insert-fix-2026-10-19/unit-internal.log) is preserved, not labelled green.
- After all other test/race/build processes exited, the **same full timing-gate tests**, with the original fixtures, assertions and budgets, passed alone43.526s: cold3Y5.36s; dense single-day5.573s, comparison6.131s, endpoint equivalence5.62s. [Isolated log](performance-c1-insert-fix-2026-10-19/isolated-timing-gates.log). No test skipped, budget raised, correctness check removed or failed observation overwritten.
- Backend vet/build, gofmt and whitespace checks pass. The original missing Linux GTK/WebKit native-build/bindings limitation remains; no frontend, bound service, DTO or generated artifacts change here. No native app was launched. Canonical CI on the new PR head remains required; parent monitors it.

The source-tracking implementation and both measurement harnesses are unchanged from measured commit0a7c306; the final follow-up adds documentation and the stale-floor oracle test. These are pre-merge review fixes only. Row/cadence choice and observed write overhead remain visible review trade-offs, with no merge/release or C2/C3 activation.
