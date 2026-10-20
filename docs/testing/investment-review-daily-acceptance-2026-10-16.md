# Investment review daily acceptance evidence

This trace maps all 99 design 1.2 matrix IDs to evidence and records how each
item was checked. Go tests ran under the repository server profile; frontend
tests ran under Vitest. `Source mapped` means implementation was inspected but
there is no isolated test with the same scenario wording. Device-only checks are
called out explicitly and are not represented as automated passes.

## Assets and values

| ID | Evidence | Execution status |
| --- | --- | --- |
| A01 | `TestReviewCoverageIncludesUnknownWithoutAnAlertAndExcludesZeroHolding`; `TestReviewDailyEntirePredicateUsesSameDate` | Go suite passed |
| A02 | `TestReviewDailyCloseDoesNotCarryPrices` (valid zero price and no carry-forward) | Go suite passed |
| A03 | source: `projectReviewHistoricalDayFromPortfolio` aggregates holding quantities by instrument before comparison; multi-account scenario is not isolated in one review-specific test | Source mapped; no isolated test |
| A04 | `TestInvestmentReviewDuplicateDisplayAliasesKeepDistinctIDs`; `TestReviewRuleV2RejectsForeignOrUnknownTarget` | Go suite passed |
| A05 | `TestReviewCoverageIncludesUnknownWithoutAnAlertAndExcludesZeroHolding` (archived positive holding remains visible) | Go suite passed |
| A06 | `TestReviewPositionEpochsUseAtomicQuantityEffects`; `TestReviewAtomicSameDayTransferKeepsEpisodeAndAcknowledgement` | Go suite passed |
| A07 | `TestReviewPositionEpochCannotInventOriginQuantity`; source: historical projection rejects negative/unproven quantities and never uses current balances as a baseline | Go suite passed |

## Money and FX

| ID | Evidence | Execution status |
| --- | --- | --- |
| M01 | `TestReviewDailyEntirePredicateUsesSameDate` | Go suite passed |
| M02 | `TestReviewDailyEntirePredicateUsesSameDate`; `TestInvestmentReviewLargeDecimals` | Go suite passed |
| M08 | `TestInvestmentReviewLargeDecimals`; source: canonical decimal comparison in `review_daily_evidence.go` | Go suite passed |
| M03 | `TestInvestmentReviewLargeDecimals`; source: holdings aggregate before final rule comparison in `review_historical_inputs.go` | Go suite passed |
| M04 | `TestReviewHistoricalBatchDoesNotBackfillCurrentHolding` | Go suite passed |
| M05 | `TestReviewFXEconomicDateBranches`; `TestReviewFXReferenceAgesAndReciprocal` | Go suite passed |
| M06 | `TestReviewFXReferenceAgesAndReciprocal`; `TestReviewFXEconomicDateBranches` | Go suite passed |
| M07 | `TestReviewFXEconomicDateBranches` (same-currency identity and directional quote admission) | Go suite passed |
| M09 | `TestReviewFXEconomicDateBranches` (late daily reference follows economic date) | Go suite passed |
| M10 | `TestReviewFXEconomicDateBranches` (cutoff equality, late timestamp, missing timestamp) | Go suite passed |

## Performance history

| ID | Evidence | Execution status |
| --- | --- | --- |
| P01 | `TestReviewDailyPerformanceNeedsTrueEndpointAndBoundedBaseline`; source: daily performance projection consumes quotes, not ledger cash flows | Go suite passed |
| P02 | source: `ResolveReviewDailyPerformance` in `internal/domain/review_daily_evidence.go` has no account/holding repository dependency | Source mapped; no isolated test |
| P03 | `TestReviewDateSlotsAreIdempotentAndAdvanceWithoutQuotes` | Go suite passed |
| P04 | `TestReviewDailyPerformanceNeedsTrueEndpointAndBoundedBaseline` | Go suite passed |
| P05 | `TestReviewDailyPerformanceNeedsTrueEndpointAndBoundedBaseline` (7/8-day baseline boundary) | Go suite passed |
| P06 | `TestReviewDailyPerformanceNeedsTrueEndpointAndBoundedBaseline`; `TestReviewDailyCloseDoesNotCarryPrices` | Go suite passed |
| P07 | `TestReviewSpecRejectsAmbiguousOrUnsupportedSemantics` (annualized scope and instrument type validation) | Go suite passed |
| P08 | source: daily rule leaf evidence reports price change/annualized performance separately; no total-return field is produced | Source mapped; no isolated test |

## Daily policy

| ID | Evidence | Execution status |
| --- | --- | --- |
| D01 | `TestReviewDailyGoldenWindows/D01_single` | Go suite passed |
| D02 | `TestReviewDailyGoldenWindows/D02_three_dates` | Go suite passed |
| D03 | `TestReviewDateSlotsAreIdempotentAndAdvanceWithoutQuotes` | Go suite passed |
| D04 | `TestReviewDailyGoldenWindows/D04_gap`; `TestReviewPolicyTimelineMatchesDateKeyedEvaluation` | Go suite passed |
| D05 | `TestReviewDailyGoldenWindows/D05_false_gap` | Go suite passed |
| D06 | `TestReviewDailyGoldenWindows/D06_sufficient_known` | Go suite passed |
| D07 | `TestReviewDailyGoldenWindows/D07_impossible`; `TestReviewDailyGoldenWindows/D07_uncertain` | Go suite passed |
| D08 | `TestReviewDateSlotsAreIdempotentAndAdvanceWithoutQuotes` | Go suite passed |
| D09 | `TestReviewDailyGoldenWindows/D28_false_does_not_cancel_hits`; source: policy evaluation counts the closed window independently from the latest daily vote | Go suite passed |
| D10 | `TestReviewDailyEntirePredicateUsesSameDate`; `TestReviewHistoricalBatchDoesNotBackfillCurrentHolding` | Go suite passed |
| D11 | `TestReviewDailyEntirePredicateUsesSameDate` | Go suite passed |
| D12 | `TestReviewHistoricalBatchDoesNotBackfillCurrentHolding`; `TestReviewHistoricalDayBeforeOriginIsUnknown` | Go suite passed |
| D13 | `TestReviewDailyBooleanGate`; `TestReviewThreeValuedPoliciesAgreeWithAllCompletions` | Go suite passed |
| D14 | `TestReviewDailyCloseDoesNotCarryPrices`; `TestReviewDailyPerformanceNeedsTrueEndpointAndBoundedBaseline` | Go suite passed |
| D15 | `TestReviewFXEconomicDateBranches` | Go suite passed |
| D16 | `TestReviewCivilDateBoundaries`; `TestReviewDateSlotsAreIdempotentAndAdvanceWithoutQuotes` | Go suite passed |
| D17 | `TestReviewCivilDateBoundaries` (23/25-hour DST dates); `TestReviewFXEconomicDateBranches` | Go suite passed |
| D18 | `TestReviewLifecycleKeepsEpisodeWhenTimezoneMovesLatestBeforeTrigger`; `TestReviewTimezoneEditThroughMutationAndPublicationKeepsAcknowledgement` explicitly asserts the edited timezone's latest closed date is earlier than the stored trigger; `TestReviewCivilDateBoundaries` | Focused Go test passed |
| D19 | `TestReviewPolicyTimelineMatchesDateKeyedEvaluation`; source: `PlanReviewRuleInstrument` replays each closed date from the saved history origin | Go suite passed |
| D20 | `TestReviewHistoricalDayBeforeOriginIsUnknown`; `TestReviewHistoricalBatchDoesNotBackfillCurrentHolding` | Go suite passed |
| D21 | `TestReviewPolicyValidationAndNormalization` | Go suite passed |
| D22 | `TestReviewRulePreviewIsReadOnlyAndDateKeyed`; `TestReviewMaterialEditKeepsAcknowledgedPublishedEpisodeWhenIntervalStaysTrue` | Go suite passed |
| D23 | `TestReviewHistoricalDayBeforeOriginIsUnknown`; `TestReviewCivilDateBoundaries` | Go suite passed |
| D24 | `TestReviewDateSlotsAreIdempotentAndAdvanceWithoutQuotes` | Go suite passed |
| D25 | `TestReviewLifecycleRelatesEditedConditionByProvenPolicyInterval`; `TestReviewDailyEvidencePagesClosedVotesAndExpiresOnRevision` | Go suite passed |
| D26 | `TestReviewSameDayClearAndRebuyUsesNewEpisodeWithClosedVotes`; `TestReviewSameDayClearClosesEpisodeBeforeNextDailyVote` | Go suite passed |
| D27 | `TestInvestmentReviewActualDaysAndLatestObservation`; `TestReviewLegacyConversionPreviewsThenAtomicallyArchivesAndCreatesTwoRules` | Go suite passed |
| D28 | `TestReviewDailyGoldenWindows/D28_false_does_not_cancel_hits` | Go suite passed |

## Episode lifecycle

| ID | Evidence | Execution status |
| --- | --- | --- |
| L01 | `TestReviewPublicationIsAtomicAndSameDayIdempotent` | Go suite passed |
| L02 | `TestReviewAcknowledgementV2MutatesOnlyEpisodeHandling`; `TestReviewDailyEvidencePagesClosedVotesAndExpiresOnRevision` | Go suite passed |
| L03 | `TestReviewLifecycleRelatesEditedConditionByProvenPolicyInterval` (proven recovery splits the interval) | Go suite passed |
| L04 | `TestReviewPublicationPreservesAcknowledgementAcrossUnknownDay`; `TestReviewCoveragePausePreservesUnknownDailyTruth` | Go suite passed |
| L05 | `TestReviewEpisodeProvenFalseSplitsHiddenRecovery`; `TestReviewLifecycleRelatesEditedConditionByProvenPolicyInterval` | Go suite passed |
| L06 | `TestReviewLifecycleRebuyUsesNewEpochButKeepsRollingVotesAndIgnore`; `TestReviewSameDayClearAndRebuyUsesNewEpisodeWithClosedVotes`; `TestReviewCorrectedPositionHistoryDropsObsoleteEpochStreak` | Go suite passed |
| L07 | `TestReviewRuleV2MutationCASCanonicalVersionAndReceipt`; `TestReviewMaterialEditKeepsAcknowledgedPublishedEpisodeWhenIntervalStaysTrue` | Go suite passed |
| L08 | `TestReviewLifecycleKeepsEpisodeWhenTimezoneMovesLatestBeforeTrigger`; `TestReviewTimezoneEditThroughMutationAndPublicationKeepsAcknowledgement`; `TestReviewRuleV2MutationCASCanonicalVersionAndReceipt` | Go suite passed |
| L09 | `TestReviewLifecycleKeepsAcknowledgementThroughUnknownAndReturn`; `TestReviewMaterialEditKeepsAcknowledgedPublishedEpisodeWhenIntervalStaysTrue` | Go suite passed |
| L10 | `TestReviewPositionEpochsUseAtomicQuantityEffects`; `TestReviewSameDayClearAndRebuyUsesNewEpisodeWithClosedVotes` | Go suite passed |
| L11 | `TestReviewLifecycleRebuyUsesNewEpochButKeepsRollingVotesAndIgnore`; `TestReviewPublicationRebuyOpensNewEpisodeWithoutDiscardingRollingVotes` | Go suite passed |
| L12 | `TestReviewDailyGoldenWindows/D28_false_does_not_cancel_hits`; source: one publication creates one current episode per rule/instrument/position epoch | Go suite passed |
| L13 | `TestReviewEpisodeProvenFalseSplitsHiddenRecovery`; `TestReviewDailyEvidencePagesClosedVotesAndExpiresOnRevision` | Go suite passed |
| L14 | `TestReviewEpisodeHistoryPagesResolvedAcknowledgedAndReboughtEpisodes`; `TestReviewPublicationPreservesAcknowledgementAcrossUnknownDay` | Go suite passed |
| L15 | `TestReviewLifecycleRelatesEditedConditionByProvenPolicyInterval`; `TestReviewEpisodeHistoryPagesResolvedAcknowledgedAndReboughtEpisodes` | Go suite passed |

## Ignore scopes

| ID | Evidence | Execution status |
| --- | --- | --- |
| I01 | `TestReviewSuppressionScopesAndAcknowledgementRemainIndependent`; `TestReviewLifecycleRebuyUsesNewEpochButKeepsRollingVotesAndIgnore` | Go suite passed |
| I02 | `TestReviewSuppressionScopesAndAcknowledgementRemainIndependent`; `TestReviewSuppressionV2ReceiptKeepsAbsoluteCivilExpiry` | Go suite passed |
| I03 | `TestReviewSuppressionScopesAndAcknowledgementRemainIndependent` | Go suite passed |
| I04 | `TestReviewSuppressionCivilExpiryAndIdempotentAbsoluteDate`; `TestInvestmentReviewSnoozeUsesHouseholdCivilDateAcrossDST`; `TestReviewSuppressionV2ReceiptKeepsAbsoluteCivilExpiry` | Go suite passed |
| I05 | `TestReviewSuppressionV2ReceiptKeepsAbsoluteCivilExpiry`; `TestReviewSuppressionV2CanBeSetAgainAfterExpiryConcurrently` | Go suite passed |
| I06 | `TestReviewSuppressionExpirySurvivesRestoredProjection`; `TestInvestmentReviewRecoveryAndUnsupportedDataDuringSnooze` | Go suite passed |
| I07 | `TestReviewSuppressionScopesAndAcknowledgementRemainIndependent`; `TestReviewSuppressionV2CanBeSetAgainAfterExpiryConcurrently` | Go suite passed |

## Storage and compatibility

| ID | Evidence | Execution status |
| --- | --- | --- |
| S01 | `TestV18UpgradePreservesLegacyReviewAndRecovery`; `TestV19VerifierDetectsReviewSchemaTampering` | Go suite passed |
| S02 | `TestV18UpgradeQuarantinesMalformedAndOrphanReviewKeys`; `TestV18UpgradePreservesLegacyReviewAndRecovery` | Go suite passed |
| S03 | `TestReviewLegacyConfigurationRetainsFalseEnrollmentAndUnknownAcknowledgementKey`; `TestV18UpgradeQuarantinesMalformedAndOrphanReviewKeys` | Go suite passed |
| S04 | `TestReviewRuleV2MutationCASCanonicalVersionAndReceipt`; `TestReviewPublicationRejectsConcurrentIgnoreAndMidnight` | Go suite passed |
| S05 | `TestReviewRuleV2MutationCASCanonicalVersionAndReceipt`; `TestReviewSuppressionV2ReceiptKeepsAbsoluteCivilExpiry` | Go suite passed |
| S06 | `TestInvestmentReviewWorksWithReadOnlyDatabase`; `TestReviewEpisodeHistoryPagesResolvedAcknowledgedAndReboughtEpisodes` (history read has no writes) | Go suite passed |
| S07 | `TestReviewPublicationRejectsConcurrentAcknowledgement` | Go suite passed |
| S08 | `TestV18UpgradePreservesLegacyReviewAndRecovery`; `TestReviewHistoricalDayBeforeOriginIsUnknown` | Go suite passed |
| S09 | `TestReviewPositionEpochCannotInventOriginQuantity`; `TestReviewPositionEpochsUseAtomicQuantityEffects` | Go suite passed |
| S10 | `TestReviewPublicationIsAtomicAndSameDayIdempotent`; source: reconciliation checks context cancellation before publishing staged rows | Go suite passed |
| S11 | `TestV18UpgradePreservesLegacyReviewAndRecovery`; `TestInvestmentReviewThirtyDaySnoozeAndBackupPersistence` | Go suite passed |
| S12 | `TestReviewHistoricalSourceTokenDetectsQuoteDeletionWithoutGenerationBump`; `TestReviewPublicationRejectsDeletedFinancialSource`; `TestReviewHistoricalQuoteActivityAndFXCorrectionsInvalidateAndRecompute` | Go suite passed |
| S13 | `TestReviewPublicationFailureAfterFinancialCommitDoesNotHalfWrite` (synthetic SQLite trigger forces publication control-update failure after the financial transaction commits) | Go suite passed |

## API and UI

| ID | Evidence | Execution status |
| --- | --- | --- |
| C01 | `TestLegacyReviewPreferencesAdvanceCompatibilityRuleWithoutRewritingSource`; `TestReviewLegacyConversionPreviewsThenAtomicallyArchivesAndCreatesTwoRules` | Go suite passed |
| C02 | `TestReviewEpisodeHistoryPagesResolvedAcknowledgedAndReboughtEpisodes`; `TestReviewDailyEvidencePagesClosedVotesAndExpiresOnRevision`; `TestReviewAlertPageCursorExpiresOnHandlingChange` | Go suite passed |
| C03 | `TestV18UpgradePreservesLegacyReviewAndRecovery`; backup compatibility test coverage in `internal/infrastructure/backup` | Go suite passed |
| C04 | `TestJSONExportReconciliationSchemaCompatibility`; `TestJSONExportStableEmptyCollectionsAndNoWrites` | Go suite passed |
| U01 | `ReviewRuleEditor.test.tsx` nested editor and validation cases; three locale files; [Chromium editor screenshot](../qa/investment-review-daily-2026-10-16/nested-condition-editor.chromium.png) captured from the synthetic harness. Native zoom/theme/layout acceptance was not run. | Vitest passed; Chromium screenshot captured; native layout not run |
| U02 | `ReviewEpisodeHistorySheet.test.tsx`; shared Sheet keyboard/focus behavior is tested in `src/components/ui/sheet.test.tsx`; native VoiceOver/IME acceptance is not run here. | Vitest; native VoiceOver/IME not run |
| U03 | `InvestmentReviewPage.test.tsx`; `ReviewRuleEditor.test.tsx`; `ReviewEpisodeHistorySheet.test.tsx` | Vitest passed |
| U04 | owner split documented in `investment-review-daily-rollout.md`; shared App/navigation wiring is owned by PR99 and was not edited in this branch | Page contract tested; shared route owned by PR99 |
| U05 | `ReviewRuleEditor.test.tsx` (nested editor, 20/21 leaves, invalid numeric thresholds and multi-type selection); `TestReviewPredicateTwentyLeafLimitIncludesNestedGroups`; `TestReviewPolicyValidationAndNormalization` | Go + Vitest passed |
| U06 | `TestReviewCoverageIncludesUnknownWithoutAnAlertAndExcludesZeroHolding`; `TestReviewCoveragePausePreservesUnknownDailyTruth`; `InvestmentReviewPage.test.tsx` | Go + Vitest passed |

## Performance

| ID | Evidence | Execution status |
| --- | --- | --- |
| F01 | `TestReviewPublicationIsAtomicAndSameDayIdempotent`; `TestReviewProjectionReplayInvalidatesPersistedCacheOnSourceChange`; `TestReviewPolicyTimelineHandlesLongDailySuffixInOnePass`; `TestReviewIncrementalReplayPreservesFullDailyTrueSuffix`; the historical full-pipeline measurement harness has been retired; current retained checks are listed in the [performance report](../development/performance-optimization-report.md). | First-page budgets pass at 30 and 365 days; full warm and cold budgets fail as measured below |

## Performance measurement and limits

The opt-in full-pipeline test uses isolated SQLite databases and synthetic
facts only. The standard fixture contains 100 positive holdings, 20 rules,
2,000 rule/instrument pairs, 100,000 generated quote rows plus 100 baseline
manual quotes, 50,000 cash-only activities, and heterogeneous IANA timezones,
metrics and trigger policies. FX references number 120 for the 30-day mode and
1,460 for the 365-day mode. It records source counts, projection rows and
SQLite database+WAL+SHM bytes.

The records below come from separate invocations of the opt-in harness; do
not treat earlier cold-run values as controlled before/after measurements.
In particular, the archived 305.539s and 339.29s cold values came from
different runs. The current Linux run used `TMPDIR=/workspace/.tmp` because
the runner's `/tmp` volume had only 1.2GB free.

**Pre-query-rewrite 365-day clock-only baseline.** The cold full reconcile took 86.362s
(n=1), against the 3s target. After it, 30 warm reconciles had a median of
15.800s, nearest-rank p95 17.262s and max 17.431s against 500ms. First-page
reads had a median of 42.043ms, p95 47.570ms and max 60.603ms against 150ms.
After 30 increments the fixture held 790,000 daily inputs and 790,000 results;
database+WAL+SHM totaled 1,541,427,168 bytes. The first warm reconcile read
200,200 quote rows across capture and transactional recheck; its 20 reusable-
evidence window queries returned 6,000 rows. Their summed driver
Query-to-Rows.Close time was 9.551s, and the full summed driver
Query-to-Rows.Close time was 12.612s. These timings include row iteration,
scanning and JSON decoding during consumption; they are not pure SQLite
execution times. Capture and publication repeat the authoritative source read
to preserve the concurrency snapshot check.

**Pre-query-rewrite 30-day clock-only baseline.** The cold full reconcile took 10.393s
(n=1). Across 30 warm reconciles, median was 6.677s, p95 7.290s and max
7.313s against 500ms. First-page reads had a median of 19.480ms, p95
25.696ms and max 26.307ms against 150ms. After 30 increments the fixture
held 120,000 daily inputs and 120,000 results; database+WAL+SHM totaled
393,771,424 bytes. The first warm reconcile executed 371 SQL queries across
capture, recheck and publish; it read 200,200 quote rows and 6,000 reusable
evidence rows. Combined driver Query-to-Rows.Close time was 4.087s, including
row iteration and decoding rather than SQL execution alone.

**Changed-source scenarios, 30-day fixture.** A typical daily refresh appended
100 instrument quote rows and 4 FX references before one warm reconcile. A
single changed-source fingerprint then disabled evidence reuse and staged
62,000 days (2,000 rule/instrument pairs × 31 dates). Planning took 3.354s;
the atomic publish took 5.136s, with 128,241 SQL executions and 37,538,300
serialized input/result/coverage bytes. A separate historical correction
updated one existing quote's price. This also changed the source fingerprint,
disabled reuse and staged all 62,000 days; planning took 3.052s and atomic
publish 4.914s. Regression tests verify in-place changes to existing UnitPrice
and FX Rate values change the historical source token.

**30-day capture optimization evidence.** Each warm capture and publish
recheck runs one 100,100-row quote-history scan (~0.9s) and 20 reusable-evidence
queries (~1.0s total at 30 days). The two review correction-chain queries still
scan the 50k cash-only activity/effect rows while filtering them, although they
return no review activity or cost events. Review-only current-quote selection is now skipped as
redundant with full quote history. Rule revision checks were reduced from one
per pair to one per rule; existing evaluation-state epochs are fetched in one
transactional query instead of one query per pair. The review correction CTE
starts from quantity/cost-relevant activity chains rather than all activities.
An experiment combining reusable-evidence queries into one SQL statement
regressed 30-day capture from ~0.9s to ~1.5s and was reverted.

Performance acceptance remains blocked. At 365 days, a covering index for
`(household_id,input_policy,timezone_version,instrument_id,on_date)` is a
candidate for reducing reusable-evidence ranking scans, but it requires a
schema migration. Its synthetic-trial file-size and database-page deltas are
reported separately below. Reaching the 500ms
warm target also requires avoiding full 100k-quote source rescans on both
capture and publish recheck. A future design could maintain a source-change
generation with transactional triggers for every authoritative source table,
then verify that generation inside publication before reusing date slots.
This needs a completeness audit for every insert/update/delete/withdrawal path
and a write-amplification/storage review. This branch does not contain that
schema/index/generation work, so F01 is not accepted for latency. A generation
counter alone would only skip scans when no sources changed; a normal daily
quote/FX append still invalidates it and currently forces full replay. A safe
dirty-frontier path must account for every source insert/update/delete and
recompute the affected suffix. The measured first full replay also misses the
3-second target and needs background materialization or a shared-input storage
redesign before the cold budget can be accepted.

**Key-first reusable-evidence query comparison.** On the same isolated 30-day
fixture, the original and key-first SQL returned identical rows (6,000 total),
and each matched the typed evidence map returned by production capture. Without
an added index, the original query's QueryContext/Rows.Next/Rows.Scan/JSON
decode/Query-to-Rows.Close times were 810.4/21.2/1.5/38.8/874.0ms; the
key-first query measured 539.6/27.6/2.4/41.4/616.4ms. With a trial index,
the same split measured 237/18/1.5/39.4/298ms for the original and
246/20/1.9/43.9/316ms for key-first. The trial index built in 174ms. On this
30-day fixture, the DB+WAL+SHM file set grew by 20,844,048 bytes, while the
database `page_count` grew by 2,536 4KB pages (10,387,456 bytes, about
10.4MB). The file-set delta includes WAL and SHM and is not the persistent
index size.

On the same 365-day fixture, both statements again returned identical 6,000
rows and matched production capture. Without the trial index, the original
query measured 8.770/18.6/1.5/38.8/8.831s and key-first measured
5.285/36.6/1.6/40.5/5.366s. With the trial index the times were
2.190/17.7/1.5/37.3/2.249s and 2.050/28.8/1.6/36.4/2.119s, respectively.
The index build took 2.248s. On this 365-day fixture, the DB+WAL+SHM file set
grew by 256,042,808 bytes from a pre-index total of 1,397,895,168 bytes
(about 18.3%), while the database `page_count` grew by 31,135 4KB pages
(127,528,960 bytes, about 127.5MB). These are distinct measurements: the
file-set delta includes WAL and SHM and is not the persistent index size. The
timings separate QueryContext,
Rows.Next, Rows.Scan, and JSON decoding; Query-to-Rows.Close also includes
result construction and is not pure SQL time. Query plans show that key-first
ranks narrow instrument/date keys before fetching JSON payloads. The trial
index removes the window sort in EXPLAIN, but its storage cost is substantial;
no schema migration or index is included in this change.

In one post-rewrite 30-day warm increment, capture plus source fingerprint
took 2.740s, in-memory fingerprinting 746ms, planning 267ms, a separate read-
only recheck 2.750s, and atomic publish with its recheck/writes 2.870s. The
captured 100,100 quote rows remain the largest measured driver
Query-to-Rows.Close operation (about 0.86s); the 20 reusable-evidence queries
took 0.58s combined by the same metric. These timings include row consumption,
not only SQLite execution. These phase results confirm the 500ms warm-reconcile
budget still fails; they are a single warm increment and do not replace the
30-sample p95 runs above.

The corresponding post-rewrite 365-day warm increment took 7.938s for capture
plus source fingerprint, 640ms for the in-memory fingerprint, 287ms for
planning, 7.592s for a separate read-only recheck, and 3.136s for atomic
publish with its recheck/writes. The 20 reusable-evidence reads took 5.593s
during capture and 5.179s during the separate recheck by summed driver
Query-to-Rows.Close time; each returned 6,000 rows across all rules. The
100,100-row quote history read took 977ms and 944ms by the same metric. These
include row iteration/consumption. After this increment, database+WAL+SHM totaled
1,423,883,904 bytes with 732,000 input rows and 732,000 result rows.

Native macOS performance is not measured here.

Every matrix item has an evidence mapping above. Items with automated tests are
covered by the server-profile Go suite or Vitest suite; `Source mapped` items
are implementation checks without an isolated scenario test. The Chromium
screenshot uses the real nested editor component with synthetic hooks and is
documented alongside the image. The UI interaction and validation cases also
run under Vitest. Native macOS Wails, VoiceOver, Chinese IME, 200% zoom and
desktop backup/restore acceptance remain device-specific checks.
