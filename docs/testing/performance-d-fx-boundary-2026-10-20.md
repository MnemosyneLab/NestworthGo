# D-FX production Agent import suffix proof (2026-10-20)

Baseline: `9c21b2de808430e7837dfccecf8edbb30fbb0113`, verified before
this independent branch; H1 main `1615057a532a9577bf53071bb00254e30fa8dfc2`
is integrated without changing its files. This D-only extension uses existing
certificate JSON, source journal and daily storage. No schema migration, new
codec, raw-rate store, H1/H2 or E/F/G changes are introduced.

## Production writer and eligible scope

`Service.ImportAgentMarketData` calls `Repository.ImportAgentQuoteBatch`.
`importAgentQuoteRecordTx` inserts an FX quote plus an `agent_quote_records`
creation record for append; correction inserts a replacement quote plus a record
that targets the prior quote; retraction inserts only a withdrawal record.
Historical and current FX queries exclude targets referenced by withdrawal or
correction provenance. Both FX facts and provenance participate in C1 tracking.
The earlier quote-only SQL UPDATE/DELETE prototype did not demonstrate an
eligible supported application writer. It has been replaced: quote-only raw
mutations now fall back and positive tests use the complete production API.

Eligibility is one existing direct base/comparison pair whose complete active
inventories contain only agent `daily_reference` candidates, valid civil dates,
nonzero value-effective timestamps and nonempty policy/timestamp-basis metadata.
Source key, source policy and timestamp basis are common and unchanged. Portfolio
FX inputs must be exact historical-candidate subsets. Captured instruments use
one of the pair's currencies; currency-bearing rule leaves name the comparison
currency. Identity conversion is harmless; inverse/cross routing is outside scope.

Only production-shaped append, same-pair/date replacement correction and
retraction qualify. Every existing audit row must remain byte-equivalent, and
each new FX provenance row must be independently associated with the exact
candidate addition/withdrawal and journal claim. Intermediate same-batch chains,
new preferences, unrelated instrument provenance, mixed source families,
manual/provider/timestamp branches, pair/date/policy moves, missing/empty
inventories and unknown cases require full preparation. Generation-only
history-state updates may accompany imports after the separate invariant agrees.

All provider preferences/leases, rules, accounts/activities, ownership, timezone,
cutoff/calendar and other non-FX captured inputs remain unchanged. The non-FX
invariant retains instrument quotes. Imports that additionally change unrelated
source/coverage/observation metadata are not granted a general exemption.

## Complete source and provenance proof

`DescribeReviewFX` versions the non-FX invariant and a complete location/pair/date
candidate inventory. Only both FX arrays and non-economic input generation are
excluded from that invariant. Candidates are ordered by ID; fingerprints include
every captured field and exact private rate scalars through the existing
historical fingerprint codec. Supersedes IDs are also exposed for independent
association with replacement provenance.

Inside the same coherent capture transaction, `captureReviewFXProvenance` reads
**all** household `agent_quote_records`, including instrument records and inactive
FX targets. Each sealed record binds all ten SQL columns (including request key,
household, operation, source title/URL, timestamps and NULL distinctions). The
certificate keeps hashes and association fields, not raw provenance text. This
inventory supplements the invariant; the provenance table is not ignored or
removed from any fingerprint. Records are separately versioned and remain bound
by the existing source-capture and certificate MACs. Unchanged authentic captures
may reuse the deterministic descriptions only with exact source-token and full
historical-fingerprint equality. Legacy/missing descriptions cannot authorize
changed-FX reuse; restart/restore/key loss cannot create historical authority.

The planner compares complete OLD/NEW inventories independently of journal
metadata. Every prior provenance record must remain identical; every added record
must have a matching C1 insert scope, target type `fx` and a supported captured
operation. C1 scopes do not contain the semantic Agent operation, so that value
comes from the independently sealed row. Active candidates require exactly one
creation record and no withdrawal. Appends/corrections require a new captured
quote, its matching quote-insert journal scope, and matching supersedes linkage;
corrections/retractions require a prior active target absent from the new active
inventory. Corrections preserve target pair/date. Every active-set difference
must be explained; every surviving candidate must remain exactly identical.
Forged/missing record claims or edited/removed/unrelated audit records fail closed.

Both portfolio inventories resolve exact IDs/payloads to their complete historical
inventories. Review reconstruction replaces portfolio FX with complete historical
facts, so legitimate active-set changes are evaluated from the complete candidate
set, including losing alternatives and previously selected targets.

Agent daily-reference selection gates candidates by civil effective date. Under
stable direct-pair policy, the above additions/withdrawals cannot affect an earlier
day. The planner invalidates **every** known rule/instrument, including explicit,
evaluated and open-episode targets, from the earliest affected OLD/NEW date through
latest closed day. There is no early stop. Existing contiguous-prefix validation,
final prefix digest/MAC, row sealing, source/control/checkpoint CAS, physical
connection counters, pinned provider lease and final clock fences remain intact.
Policy/lifecycle replay still starts at origin with full votes and handling state.

## Synthetic production oracle evidence

`TestReviewDependencyFXProductionImportFullOracle` creates its USD holding and
FX history through production services, imports two baseline agent references,
and acknowledges an episode. Positive append/correction/retraction writes use
`ImportAgentMarketData`, including normal provenance and snapshot rebuilding.
Each case first proves an unchanged-source hit, then a nonempty changed-FX prefix,
fewer recomputed days and changed admitted financial FX evidence:

| Production operation | Reused | Recomputed / full days | Admitted rate change |
| --- | --- | --- | --- |
| Append on Oct 6 | 5 | 6 / 11 | 1 → 3.123456789123 |
| Correct Oct 8 reference | 7 | 4 / 11 | 2 → 3.123456789123 |
| Retract Oct 8 reference | 7 | 4 / 11 | 2 → 1 |

The same API fixture requires fallback for date-moving correction, an unrelated
instrument import, an edit of prior provenance and a forged provenance journal
quote ID. It compares the full Agent audit list before/after review publication,
including preserved superseded/withdrawn quotes. Raw-SQL quote mutations have
separate negative full-oracle tests. Domain tests cover canonical inventories,
both captured collections, exact scalars, missing candidates/records/journal,
changed old provenance, unrelated records and unsupported operations/policies.
Final-clock mutation of captured provenance must reject atomically.

`assertSuffixOracle` compares preparation with the full pipeline oracle and
publishes a separate synthetic `VACUUM INTO` copy with `WithFullReviewRecompute`.
Durable comparison covers daily financial/predicate evidence, episodes, events,
handling and identities. Existing identities stay exact; only newly allocated
independent UUIDs map through complete semantic keys. The shared row helper strips
`computed_at`, `evaluated_at`, `revised_at`; `publicationDailyTimes` separately
compares exactly daily-input `computed_at` and daily-result
`evaluated_at`/`revised_at`. No broader timestamp equality is claimed.

## Validation and performance limits

Focused Linux checks use Go 1.26.1 with isolated toolchain/cache paths under
`/tmp`; final commands/results are recorded in the PR. Scope is the production
FX matrix, raw-mutation fallback, provenance final fence, existing suffix/oracle
and source-description guards, domain FX proof, SQLite Agent batch
correction/retraction/export/rollback and scoped vet/formatting/diff checks.
No broad suite, race/performance matrix or native gates are added.

Passed commands (isolated `GOPATH`/`GOCACHE` under `/tmp`):

```sh
go test ./internal/application -run '^TestReviewDependency(FXProductionImportFullOracle|FXRawMutationFullFallback|FXProvenanceFinalFence|SuffixExactFXPrefix|SuffixAuthorityAndRollback|SuffixFullDurableOracle|SourceDescriptionMatchesFullDerivation)$' -count=1 -v
go test ./internal/domain ./internal/infrastructure/sqlite -run 'Test(ReviewFX|PlanReviewInvalidationOldNewAndFallback|ReviewSuffixFingerprintIncludesPrivateFXScalar|ReviewSuffixAttestationRequiresCompleteTypedOutput|AgentQuoteBatch)' -count=1
go test ./internal/application -run '^TestAgent(MarketDataCorrectionRetractionAndSourceOnlyHistory|FXRetractionRestoresConvertedMetalRevision|LastFXRetractionMakesConvertedMetalUnavailable|MixedFXAgreeAcrossCurrentAndPortfolio)$' -count=1
go test ./internal/infrastructure/sqlite -run '^TestReviewCertifiedRowEncodingPreservesTypesAndBoundaries$' -count=1
go vet ./internal/application ./internal/domain ./internal/infrastructure/sqlite
gofmt -l cmd internal
git diff --check
```

The broader focused application group passed in 12.107s. After switching the
provenance digest to the existing exact SQL-row encoder, the production/raw/final-
fence group was rerun and passed in 8.412s. Domain/SQLite proof and Agent batch
tests passed in 0.034s/0.237s; existing application Agent regressions in 0.313s.
These are harness durations, not performance benchmarks.

The counts establish application-path computation savings, not latency, p95 or
RSS acceptance. Full capture/fingerprinting, complete provenance reads/hashing,
protected-row sealing, prefix decoding, SQL-image comparison and origin policy
replay remain costs. All instruments are conservatively invalidated, and an
early correction can still require nearly full history. Full recomputation
remains the process rollback path. No broad source/routing or mixed-import
optimization is claimed.
