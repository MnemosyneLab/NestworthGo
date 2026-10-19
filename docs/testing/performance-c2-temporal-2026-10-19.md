# C2b temporal validation and publication fence

Originally stacked on C2a/PR119 (`6bbb499bc6c03ee23ba7579736f3749118c4ee94`), now based on its main squash merge `c3867c626d3b6a0cdbcf669a0047ff8d07d4215f`, including main's CI declaration from PR120. Original implementation checkpoints: `c258a80c8fe6bbac1ff889cafa4254876a69b449`, `dde7a3dfa76d20ea22adbdacafeda2ab5680cf10`, and `669ea13`; subsequent evidence commits identify final changes. Contract: [performance plan §5.4](../design/performance-optimization-plan.zh-CN.md#54-时间生效边界同日且无-dml-也可能变化). C3 remains disabled. Full source capture, exact financial fingerprints, publication rechecks and control/rule/handling checks remain authoritative.

## Publication interval and logical time

A preexisting quantity event can activate at T without another financial write. At T−1ns and T the source revision and every rule's closed day can agree while current eligibility and position epochs differ. Rechecking the fingerprint at the old capture cutoff cannot validate that staged position for a later instant.

Capture builds a finite `ReviewTemporalCertificate` in the same transaction as all financial/review inputs: UTC `[EvaluatedAt, NextTransitionAt)` at nanosecond precision, temporal policy/scope, source checkpoint, config/state/rule revisions, and DB session/clock generation. `Eligible` is diagnostic; production C2 never uses it to skip work.

Publication retains the existing immediate write reservation, full old-cutoff recapture/fingerprint, source checkpoint and review revision checks. It also repeats the uncut future-source fence there. The injected clock is sampled **after** those checks and before projection writes. Both captured/current intervals must cover that instant, clock generation must match, and existing per-rule closed-day checks must pass. Publication saves their intersection: its end is the earlier captured/rechecked boundary, never an extension of either proof. The caller's captured certificate remains unchanged. A conflict discards the entire stage and uses the existing three-attempt recapture limit.

The clock-aware publication port is required by the shared service. An adapter lacking that capability is unavailable; it cannot fall back to a time sampled before the recheck. The explicit-time repository method remains for deterministic direct callers/tests.

That final sample is the logical as-of, saved atomically with projections/lifecycle/checkpoint. Later wall time or commit cannot extend validity. Alert pages/detail groups and coverage reads retain old data and return `projectionStatus: stale` outside the interval or after session/source invalidation. Reads never reconcile. Existing page status UI is reused. The certificate digest participates in report/cursor revisions even when daily rows are unchanged.

## Complete temporal dependency audit

Scope is the singleton household, including closed/archived holdings, transfer sources and historical routes. Early invalidation is accepted. Missing/unknown namespace or checkpoint cannot prove reuse. Invalid instants/calendar intervals produce errors rather than infinity.

| Sources/read path | Time predicate and treatment |
| --- | --- |
| `history_origins`, all `history_origin_*` | Current position admits origin through the exclusive `now+1ns` cutoff; future `StartedAt` is fenced. Other baseline membership/ownership/cost/preferences use recorded origin and fixed daily cutoffs. |
| `holdings` | All holdings are captured. Current unknown-creation guard uses `CreatedAt < now+1ns`; every future creation is fenced, including archived holdings. Origin holding membership compares with fixed origin. |
| `activities`, `activity_effects`, `activity_correction_groups` | Scan all raw household activity effective instants without capture cutoff, including transfers, reversals and original/replacement chains. Replacements inherit their chain's original instant; immutable original rows remain in the scan. Earliest future raw instant conservatively bounds activation. Atomic quantity effects drive current epochs. |
| Trade/dividend details and holding cost effects | Complete cost events are read independently, then selected at fixed closed-day cutoffs. They do not activate solely when current time passes T within an unchanged closed-day vector. Quantity-changing parent events are fenced above. |
| Households, members, institutions, groups, accounts, account ownership, instruments; current FX preferences | Current directory/latest projections have no moving-now filter. Historical creation/archive/ownership is selected at fixed replay cutoffs. Rule candidates include all captured holdings; current eligibility comes from quantity replay. Source DML still uses C1/full fingerprint checks. |
| Account values/cash, holding quantity values | Latest portfolio selection follows stored row ordering, not wall time. Explicit zero baselines use fixed origin/creation predicates. Historical amounts/quantities replay at fixed day cutoffs. Current review quantities use origin/economic activities. Existing C1 replay-row classification is unchanged. |
| Account/instrument/holding state observations and state ownership | Complete histories are captured; effective/archive instants are selected at fixed rule-day cutoffs. Passing a future state instant within today cannot change a closed day's scope. |
| Instrument/FX preference observations, provider bindings/revisions | Complete histories are captured. Effective routing uses fixed replay/market cutoffs and historical route closure. Configuration edits remain source/config revisions. Credentials never enter this metadata. |
| Instrument/FX quotes | Latest main already returns **uncut history**: both `listAllInstrumentQuotesQuery` and `listAllFXQuotesQuery` explicitly ignore cutoff. A future quote edit changes the retained full fingerprint. This stage's `quoted_at` scans are additional conservative early fences, not a claim that quote capture activates at current time. |
| Quote effective dates, value-effective/provider/fetched timestamps, basis; slots and market-day coverage/status | Market selection, FX economic-date/age admission and coverage use fixed per-rule day cutoffs/market labels. Status check/expiry instants are captured data; refresh scheduling is separate. No production wall-clock call occurs in this read closure. Future metadata fixtures compare complete financial/coverage/lifecycle output at T−1ns/T. |
| Rules, suppressions, calendars | Each rule and each suppression contributes its own next local date boundary and next timezone offset transition. Origin calendar is also bounded. Household-only midnight is insufficient. Existing config/rule/state/episode checks remain. |
| Products/operations, liquidity policies/reservations, agent records, snapshot state, configuration | C1 retains conservative household source invalidation. These add no independent moving-now eligibility predicate in review. Agent withdrawal exclusions are stored facts. Resolver/evaluator/metric versions remain in the token. No settings key/value/hash or provider credential is copied. |

The roster remains the [C1 all-writer/field contract](performance-c1-source-coverage-2026-10-19.md). Fixed-day exclusions are tested, not inferred from an empty future-event result. A changed read closure requires policy review/versioning before C3 reuse.

## Exact timestamps, calendars and rollback

The audit found that SQL text comparison of valid RFC3339 values is insufficient: whole-second timestamps, optional fractional digits and UTC offsets lack one lexicographic instant order. Review quantity capture now filters parsed instants exactly, retaining the existing quantity/correction-chain scope and hydrating only admitted activities. Parsed ordering preserves the CTE's original correction-chain tie key. Other activity readers keep their existing behavior. No stored financial timestamp is rounded or rewritten.

Tests cover whole-second UTC, nanosecond UTC and a nonzero UTC offset at T−1ns/T/T+1ns. A zero→positive→zero pair with reversed timestamp text order checks full daily replay as well as current quantity.

Review calendar loads pin immutable `time.Location` objects for the process. Each newly opened DB handle has a fresh session identity; persisted certificates never assume calendar/tzdata equivalence across restart or another handle. Legacy `Local` origins pin an initialized location copy; review schedules retain fixed-IANA validation. Hot OS tzdata changes take effect with a new process/session and full recomputation. Coarse restart invalidation is deliberate; no inferred OS version is used.

All clock observations made by review commands/read models, including preview/handling, update mutex-protected observation state. Actual observed time is stored, never `max(now,last)`. Backwards observation advances generation, even above published as-of; outstanding stages conflict. Restart, missing/incompatible checkpoint, rollback, source epoch/policy change or consumed revision below floor removes reusable evidence and takes the full oracle path. Normal forward day advancement still captures all sources and may use the existing independently checked daily-window planner; it is not a no-op.

Observations strip Go's monotonic clock component before comparison, so a still-increasing monotonic reading cannot hide wall-clock rollback. The stored representation and generation transition have their own race-tested regression.

## Schema21 and recovery

Only `review_temporal_checkpoints` is added. Verified v20 gets a flushed, verified, read-only `.nestworth-v20-recovery-*/original-v20.sqlite` before atomic upgrade. No financial facts/history are backfilled. Existing complete projections become pending for revalidation. Verified older readonly snapshots stay readable without migration. Startup verifies exact table definition and checkpoint structure. Unknown nonempty policy versions cannot establish reuse and are replaced by full publication.

Complete SQLite snapshots preserve certificates consistently. Restore retains diagnostic metadata while rotating the live source epoch before resume, invalidating its source binding. Checkpoint writes are derived/excluded from C1 and cannot feed back into source revision. JSON v2 export excludes them. Algorithm rollback retains schema21/full recomputation; an older binary requires the verified pre-upgrade copy, which lacks later financial writes and must not automatically overwrite live data.

## Validation and performance evidence

All data is synthetic/isolated: no real ledger, provider network or cloud backup. No native app is launched; later native tests retain `com.nestworth.dev`.

Gates include preexisting future exit/entry/clear-rebuy/transfer with unchanged source/closed-day vectors; complete daily/lifecycle oracles and ack retention; activation during publication and bounded recapture; logical as-of followed by a pre-commit boundary crossing; same-day/prior-day/restart/preview-observed rollback; floor/restore epoch invalidation; missing/unknown policy and damaged triggers; readonly history; per-rule/suppression calendars, DST and date-line skips; checkpoint failure; actual subprocess exit after projection writes before commit; and MCP/Wails parity for expired alert/coverage reads.

Raw logs/provenance/final timings accompany draft delivery. The first broad race run retained an incorrect new assertion that a future quote edit escaped the old-cutoff fingerprint. Application failed that assertion after596.184s; it was corrected to assert actual uncut-quote behavior. Domain/SQLite/backup/MCP passed that invocation. It is not a canonical green race result. Final focused checks are separate evidence.

New C2 before/after timing distributions are deferred to keep this slice focused on temporal correctness. Existing performance budgets are unchanged. No compute-speedup or native3s/500ms claim is made. Native build/bindings generation are blocked by missing GTK/WebKit dependencies; frontend typecheck/test/build and binding freshness therefore remain unverified. Frontend lint passed. No new CI run is triggered by this task.

Parent review precedes merge/C3. Replacing full reads with journal CAS, no-op confirmation, dirty suffixes, shared facts and analysis scope remain later stages.

## Review correction: persist the rechecked interval

At reviewed head `62ad9fb21a1ec7a46a39cd2f017910302973f080`, publication checked a rebuilt frontier but saved the original captured end. The focused regression reproduces an untracked future cash activity moving from T2 to T1, with only `financial_source_activities_update` deliberately dropped. Both old-cutoff review fingerprint and source token remain equal because cash activities are outside the review quantity/cost closure. Publishing before T1 saved T2 and the read at T1 incorrectly returned `complete`. This proves a conservative certificate-bound defect; it does not prove changed review financial output from a cash activation.

The suggested quantity-only move is also tested with no holding-cost effect. It changes the retained uncut cost-event fingerprint and rejects publication even with the activity update trigger dropped. Intact tracking rejects the cash edit through its changed source token. The fix keeps both checks and persists `min(captured end, rechecked end)`. Five synthetic cases cover earlier/later untracked boundaries, exact-boundary rejection before writes, tracked-edit rejection, and the quantity fingerprint fallback. Successful cases assert `complete` at T1−1ns, `stale` at T1, the saved boundary, unchanged caller capture, and independent full-recompute oracles before/after the edit.

The [before log](performance-c2-temporal-2026-10-19/c2-interval-regression-before.log) retains the expected failure on the reviewed implementation (only the new test was added). The [after unit log](performance-c2-temporal-2026-10-19/c2-interval-regression-after-unit.log) covers every application temporal regression. These are correctness runs, not performance samples. No CI configuration, schema, budget or financial calculation changes accompany this correction.
