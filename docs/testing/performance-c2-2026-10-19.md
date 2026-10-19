# C2a source checkpoint shadow validation

Base: merged main `176bfb0d215d06058c331f444f8a66cafc712e14` (C1/PR114 plus PR115/116). Contract: [performance plan §5.3/§5.4](../design/performance-optimization-plan.zh-CN.md#53-分阶段接入与-cas). This is a small C2 validation stage, with no schema migration, persisted token, temporal certificate, source fingerprint replacement, fast no-op or dirty planner activation. [C1 CI classification and controlled comparisons](performance-c1-ci-2026-10-19.md) are separate evidence; CI sharding belongs to parent/Luna PR117.

## Implementation and publication behavior

`SourceRevisionToken` contains household, tracking epoch/revision/version, captured resolver policy, evaluator version and metric policy. The retained journal floor is separate: pruning is not a new financial source fact. `ReviewProjectionCapture` carries an optional typed checkpoint read in the **same SQLite transaction** as all full historical sources, rules and handling. Fingerprint construction is untouched and still covers private exact decimal scalars. No journal range is read by C2.

Publication still enters the existing driver-managed immediate write transaction, reloads complete financial sources at the original capture cutoff, and recomputes their full fingerprint. It also reads the current source checkpoint there and compares both signals. A changed full fingerprint still conflicts even if the token stayed equal. A changed/lost previously known token is an **additional conservative conflict**, even if the full fingerprint stayed equal; the service's existing bounded recapture/retry handles it. Neither token equality nor the observer skips financial loading, planning, control/rule/handling checks, closed-day validation, or writes already required by that algorithm.

An optional immutable repository configuration installs a synchronous diagnostic observer. It reports only verdict/boolean comparisons, containing no fingerprints, source payload, settings values/keys, journal scopes or credentials. It must be concurrency-safe and cannot reenter the database while the publication transaction is held. No telemetry is configured by default. It is not a cache or new UI/MCP endpoint. Existing review responses, preview source tokens and public JSON exports are unchanged.

Fixed-cutoff classifications distinguish unchanged source, source change, conservative invalidation (different token/same fingerprint), and untracked change (same token/different fingerprint). The latter is a negative-control proof gate against future default switching: a synthetic test removes a source trigger on an already-open isolated database, edits a quote, observes `untracked_change`, and verifies that the **original full fingerprint check** still rejects publication. Ordinary startup verification continues to reject missing/tampered triggers.

## Unknown history and time remain unavailable as reuse proofs

Verified readonly v19 snapshots have no C1 token; missing/unknown checkpoint semantics or resolver policy likewise produce no valid checkpoint. They keep the full financial/oracle path. No current-storage verifier is relaxed, no missing metadata is repaired during capture, and a readonly database remains unable to publish. Real SQL/cancellation errors still propagate.

A comparison across different capture cutoffs is explicitly incomparable; a cutoff moving backwards is reported as clock rollback. Tests exercise a preexisting quantity event at T−1ns/T: source token and closed day are equal while full fingerprint/current position eligibility can change. Rewinding to T−1ns performs a new full capture and restores the earlier financial snapshot. A same-token verdict alone is never a no-op certificate.

Publication's shadow pair intentionally uses the original fixed cutoff, matching the retained full-source recheck. This stage does **not** claim a complete time-frontier audit or certificate covering a later publication instant, including future activity/state/quote/provider activation or post-restart rollback. Rule timezone/cutoff vectors, suppression expiry, handling revisions and the full temporal closure remain independent proof gates before any C3 shortcut. No token is persisted as a reusable checkpoint here. Nothing is enabled merely because same-cutoff samples agree.

Floor/epoch tests cover a corrected quote whose journal is completely pruned, a caller below the new floor, restore-boundary epoch reset at the same numeric revision, ordinary reopen, a complete readonly SQLite snapshot, and unchanged financial fingerprints across metadata-only reset/pruning. An empty journal never suppresses full capture. Existing C1 crash/restore/import boundaries and their verifier tests remain intact.

## Validation

New domain/application/infrastructure tests check all verdict classes and semantic version dimensions; same-transaction checkpoints; conservative conflict/fresh recapture; full-fingerprint negative control; and the existing SQLite driver reservation using a **second connection** with busy_timeout0. The external writer fails with SQLITE_BUSY inside the comparison callback and succeeds after publication commits. This confirms that the check is protected by SQLite, independent of the Go ledger mutex, without sleeps or a new transaction mechanism.

Existing no-op/append/correction full-recompute oracle, complete durable projection/lifecycle comparison, public JSON export, concurrent acknowledgement/ignore/midnight and publication atomicity tests are rerun. The source token does not replace any financial semantics or correctness assertion. No GUI component, bound service, generated artifact, budget, workflow or shard script changes. Native build/bindings remain subject to the previously documented Linux GTK/WebKit limitation; no native app is launched.

Implementation/test code is fixed at `5ababa4028fdd771d1edde95f086d2cd150604ec`. Final test outcomes are recorded below. C2 adds two scalar metadata reads per capture/recheck and a conservative publication check; it claims no latency improvement or warm500ms SLA. C1 write-cost evidence remains separate and unchanged. Parent reviews this draft before any merge or further dependent protocol stage.

- Focused new source-shadow/capture/publication plus retained C1 oracle tests: domain/SQLite/application all passed; application3.819s.
- Broader domain/SQLite/application review, correction/lifecycle/oracle and JSON export tests passed0.032s /0.600s /35.674s. This includes the strict durable no-op/append/correction oracle already established in PR-A/B/C1.
- Focused race suite passed: domain, SQLite5.495s and application89.698s. Final domain namespace/floor guard race rerun passed1.018s. No assertion or detector finding.
- Backend vet, internal build, gofmt and whitespace checks passed. Raw logs, commands/provenance and checksums are in [the evidence directory](performance-c2-2026-10-19/). Native GTK/WebKit/bindings limitations remain explicit; canonical CI for this new head is not replaced by these focused checks.

Remaining C2/C3 gates include the full audited effective-time closure/certificate, post-restart rollback/frontier handling, persisted source checkpoints and the final transactional CAS/no-op protocol. They are not declared complete by this draft. No full source recheck was dropped, no financial decimal/ownership/transfer/closed-holding/V2 handling semantics changed, and F4/F15 secret flows remain excluded.
- Shared review entry-point checks passed for MCP (0.531s) and the Wails review/reconciliation services. These are scoped backend transport checks, not a native GUI smoke or a claim that every GUI/MCP feature was reverified.
