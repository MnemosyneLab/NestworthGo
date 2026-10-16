# Investment review daily acceptance evidence

This trace maps every design 1.2 matrix ID (99 total) to its executable or
source evidence. Go test identifiers below run under the repository's server
profile; frontend identifiers run under Vitest. `source` means the behavior is
implemented in the cited path but does not have a one-case test with the same
scenario wording. It is not a claim that a native/macOS acceptance was run.

## Assets and values

| ID | Evidence |
| --- | --- |
| A01 | `TestReviewCoverageIncludesUnknownWithoutAnAlertAndExcludesZeroHolding`; `TestReviewDailyEntirePredicateUsesSameDate` |
| A02 | `TestReviewDailyCloseDoesNotCarryPrices` (valid zero price and no carry-forward) |
| A03 | source: `projectReviewHistoricalDayFromPortfolio` aggregates holding quantities by instrument before comparison; multi-account scenario is not isolated in one review-specific test |
| A04 | `TestInvestmentReviewDuplicateDisplayAliasesKeepDistinctIDs`; `TestReviewRuleV2RejectsForeignOrUnknownTarget` |
| A05 | `TestReviewCoverageIncludesUnknownWithoutAnAlertAndExcludesZeroHolding` (archived positive holding remains visible) |
| A06 | `TestReviewPositionEpochsUseAtomicQuantityEffects`; `TestReviewAtomicSameDayTransferKeepsEpisodeAndAcknowledgement` |
| A07 | `TestReviewPositionEpochCannotInventOriginQuantity`; source: historical projection rejects negative/unproven quantities and never uses current balances as a baseline |

## Money and FX

| ID | Evidence |
| --- | --- |
| M01 | `TestReviewDailyEntirePredicateUsesSameDate` |
| M02 | `TestReviewDailyEntirePredicateUsesSameDate`; `TestInvestmentReviewLargeDecimals` |
| M08 | `TestInvestmentReviewLargeDecimals`; source: canonical decimal comparison in `review_daily_evidence.go` |
| M03 | `TestInvestmentReviewLargeDecimals`; source: holdings aggregate before final rule comparison in `review_historical_inputs.go` |
| M04 | `TestReviewHistoricalBatchDoesNotBackfillCurrentHolding` |
| M05 | `TestReviewFXEconomicDateBranches`; `TestReviewFXReferenceAgesAndReciprocal` |
| M06 | `TestReviewFXReferenceAgesAndReciprocal`; `TestReviewFXEconomicDateBranches` |
| M07 | `TestReviewFXEconomicDateBranches` (same-currency identity and directional quote admission) |
| M09 | `TestReviewFXEconomicDateBranches` (late daily reference follows economic date) |
| M10 | `TestReviewFXEconomicDateBranches` (cutoff equality, late timestamp, missing timestamp) |

## Performance history

| ID | Evidence |
| --- | --- |
| P01 | `TestReviewDailyPerformanceNeedsTrueEndpointAndBoundedBaseline`; source: daily performance projection consumes quotes, not ledger cash flows |
| P02 | source: `ResolveReviewDailyPerformance` in `internal/domain/review_daily_evidence.go` has no account/holding repository dependency |
| P03 | `TestReviewDateSlotsAreIdempotentAndAdvanceWithoutQuotes` |
| P04 | `TestReviewDailyPerformanceNeedsTrueEndpointAndBoundedBaseline` |
| P05 | `TestReviewDailyPerformanceNeedsTrueEndpointAndBoundedBaseline` (7/8-day baseline boundary) |
| P06 | `TestReviewDailyPerformanceNeedsTrueEndpointAndBoundedBaseline`; `TestReviewDailyCloseDoesNotCarryPrices` |
| P07 | `TestReviewSpecRejectsAmbiguousOrUnsupportedSemantics` (annualized scope and instrument type validation) |
| P08 | source: daily rule leaf evidence reports price change/annualized performance separately; no total-return field is produced |

## Daily policy

| ID | Evidence |
| --- | --- |
| D01 | `TestReviewDailyGoldenWindows/D01_single` |
| D02 | `TestReviewDailyGoldenWindows/D02_three_dates` |
| D03 | `TestReviewDateSlotsAreIdempotentAndAdvanceWithoutQuotes` |
| D04 | `TestReviewDailyGoldenWindows/D04_gap`; `TestReviewPolicyTimelineMatchesDateKeyedEvaluation` |
| D05 | `TestReviewDailyGoldenWindows/D05_false_gap` |
| D06 | `TestReviewDailyGoldenWindows/D06_sufficient_known` |
| D07 | `TestReviewDailyGoldenWindows/D07_impossible`; `TestReviewDailyGoldenWindows/D07_uncertain` |
| D08 | `TestReviewDateSlotsAreIdempotentAndAdvanceWithoutQuotes` |
| D09 | `TestReviewDailyGoldenWindows/D28_false_does_not_cancel_hits`; source: policy evaluation counts the closed window independently from the latest daily vote |
| D10 | `TestReviewDailyEntirePredicateUsesSameDate`; `TestReviewHistoricalBatchDoesNotBackfillCurrentHolding` |
| D11 | `TestReviewDailyEntirePredicateUsesSameDate` |
| D12 | `TestReviewHistoricalBatchDoesNotBackfillCurrentHolding`; `TestReviewHistoricalDayBeforeOriginIsUnknown` |
| D13 | `TestReviewDailyBooleanGate`; `TestReviewThreeValuedPoliciesAgreeWithAllCompletions` |
| D14 | `TestReviewDailyCloseDoesNotCarryPrices`; `TestReviewDailyPerformanceNeedsTrueEndpointAndBoundedBaseline` |
| D15 | `TestReviewFXEconomicDateBranches` |
| D16 | `TestReviewCivilDateBoundaries`; `TestReviewDateSlotsAreIdempotentAndAdvanceWithoutQuotes` |
| D17 | `TestReviewCivilDateBoundaries` (23/25-hour DST dates); `TestReviewFXEconomicDateBranches` |
| D18 | `TestReviewLifecycleKeepsEpisodeWhenTimezoneMovesLatestBeforeTrigger`; `TestReviewCivilDateBoundaries` |
| D19 | `TestReviewPolicyTimelineMatchesDateKeyedEvaluation`; source: `PlanReviewRuleInstrument` replays each closed date from the saved history origin |
| D20 | `TestReviewHistoricalDayBeforeOriginIsUnknown`; `TestReviewHistoricalBatchDoesNotBackfillCurrentHolding` |
| D21 | `TestReviewPolicyValidationAndNormalization` |
| D22 | `TestReviewRulePreviewIsReadOnlyAndDateKeyed`; `TestReviewMaterialEditKeepsAcknowledgedPublishedEpisodeWhenIntervalStaysTrue` |
| D23 | `TestReviewHistoricalDayBeforeOriginIsUnknown`; `TestReviewCivilDateBoundaries` |
| D24 | `TestReviewDateSlotsAreIdempotentAndAdvanceWithoutQuotes` |
| D25 | `TestReviewLifecycleRelatesEditedConditionByProvenPolicyInterval`; `TestReviewDailyEvidencePagesClosedVotesAndExpiresOnRevision` |
| D26 | `TestReviewSameDayClearAndRebuyUsesNewEpisodeWithClosedVotes`; `TestReviewSameDayClearClosesEpisodeBeforeNextDailyVote` |
| D27 | `TestInvestmentReviewActualDaysAndLatestObservation`; `TestReviewLegacyConversionPreviewsThenAtomicallyArchivesAndCreatesTwoRules` |
| D28 | `TestReviewDailyGoldenWindows/D28_false_does_not_cancel_hits` |

## Episode lifecycle

| ID | Evidence |
| --- | --- |
| L01 | `TestReviewPublicationIsAtomicAndSameDayIdempotent` |
| L02 | `TestReviewAcknowledgementV2MutatesOnlyEpisodeHandling`; `TestReviewDailyEvidencePagesClosedVotesAndExpiresOnRevision` |
| L03 | `TestReviewLifecycleRelatesEditedConditionByProvenPolicyInterval` (proven recovery splits the interval) |
| L04 | `TestReviewPublicationPreservesAcknowledgementAcrossUnknownDay`; `TestReviewCoveragePausePreservesUnknownDailyTruth` |
| L05 | `TestReviewEpisodeProvenFalseSplitsHiddenRecovery`; `TestReviewLifecycleRelatesEditedConditionByProvenPolicyInterval` |
| L06 | `TestReviewLifecycleRebuyUsesNewEpochButKeepsRollingVotesAndIgnore`; `TestReviewSameDayClearAndRebuyUsesNewEpisodeWithClosedVotes`; `TestReviewCorrectedPositionHistoryDropsObsoleteEpochStreak` |
| L07 | `TestReviewRuleV2MutationCASCanonicalVersionAndReceipt`; `TestReviewMaterialEditKeepsAcknowledgedPublishedEpisodeWhenIntervalStaysTrue` |
| L08 | `TestReviewLifecycleKeepsEpisodeWhenTimezoneMovesLatestBeforeTrigger`; `TestReviewRuleV2MutationCASCanonicalVersionAndReceipt` |
| L09 | `TestReviewLifecycleKeepsAcknowledgementThroughUnknownAndReturn`; `TestReviewMaterialEditKeepsAcknowledgedPublishedEpisodeWhenIntervalStaysTrue` |
| L10 | `TestReviewPositionEpochsUseAtomicQuantityEffects`; `TestReviewSameDayClearAndRebuyUsesNewEpisodeWithClosedVotes` |
| L11 | `TestReviewLifecycleRebuyUsesNewEpochButKeepsRollingVotesAndIgnore`; `TestReviewPublicationRebuyOpensNewEpisodeWithoutDiscardingRollingVotes` |
| L12 | `TestReviewDailyGoldenWindows/D28_false_does_not_cancel_hits`; source: one publication creates one current episode per rule/instrument/position epoch |
| L13 | `TestReviewEpisodeProvenFalseSplitsHiddenRecovery`; `TestReviewDailyEvidencePagesClosedVotesAndExpiresOnRevision` |
| L14 | `TestReviewEpisodeHistoryPagesResolvedAcknowledgedAndReboughtEpisodes`; `TestReviewPublicationPreservesAcknowledgementAcrossUnknownDay` |
| L15 | `TestReviewLifecycleRelatesEditedConditionByProvenPolicyInterval`; `TestReviewEpisodeHistoryPagesResolvedAcknowledgedAndReboughtEpisodes` |

## Ignore scopes

| ID | Evidence |
| --- | --- |
| I01 | `TestReviewSuppressionScopesAndAcknowledgementRemainIndependent`; `TestReviewLifecycleRebuyUsesNewEpochButKeepsRollingVotesAndIgnore` |
| I02 | `TestReviewSuppressionScopesAndAcknowledgementRemainIndependent`; `TestReviewSuppressionV2ReceiptKeepsAbsoluteCivilExpiry` |
| I03 | `TestReviewSuppressionScopesAndAcknowledgementRemainIndependent` |
| I04 | `TestReviewSuppressionCivilExpiryAndIdempotentAbsoluteDate`; `TestInvestmentReviewSnoozeUsesHouseholdCivilDateAcrossDST`; `TestReviewSuppressionV2ReceiptKeepsAbsoluteCivilExpiry` |
| I05 | `TestReviewSuppressionV2ReceiptKeepsAbsoluteCivilExpiry`; `TestReviewSuppressionV2CanBeSetAgainAfterExpiryConcurrently` |
| I06 | `TestReviewSuppressionExpirySurvivesRestoredProjection`; `TestInvestmentReviewRecoveryAndUnsupportedDataDuringSnooze` |
| I07 | `TestReviewSuppressionScopesAndAcknowledgementRemainIndependent`; `TestReviewSuppressionV2CanBeSetAgainAfterExpiryConcurrently` |

## Storage and compatibility

| ID | Evidence |
| --- | --- |
| S01 | `TestV18UpgradePreservesLegacyReviewAndRecovery`; `TestV19VerifierDetectsReviewSchemaTampering` |
| S02 | `TestV18UpgradeQuarantinesMalformedAndOrphanReviewKeys`; `TestV18UpgradePreservesLegacyReviewAndRecovery` |
| S03 | `TestReviewLegacyConfigurationRetainsFalseEnrollmentAndUnknownAcknowledgementKey`; `TestV18UpgradeQuarantinesMalformedAndOrphanReviewKeys` |
| S04 | `TestReviewRuleV2MutationCASCanonicalVersionAndReceipt`; `TestReviewPublicationRejectsConcurrentIgnoreAndMidnight` |
| S05 | `TestReviewRuleV2MutationCASCanonicalVersionAndReceipt`; `TestReviewSuppressionV2ReceiptKeepsAbsoluteCivilExpiry` |
| S06 | `TestInvestmentReviewWorksWithReadOnlyDatabase`; `TestReviewEpisodeHistoryPagesResolvedAcknowledgedAndReboughtEpisodes` (history read has no writes) |
| S07 | `TestReviewPublicationRejectsConcurrentAcknowledgement` |
| S08 | `TestV18UpgradePreservesLegacyReviewAndRecovery`; `TestReviewHistoricalDayBeforeOriginIsUnknown` |
| S09 | `TestReviewPositionEpochCannotInventOriginQuantity`; `TestReviewPositionEpochsUseAtomicQuantityEffects` |
| S10 | `TestReviewPublicationIsAtomicAndSameDayIdempotent`; source: reconciliation checks context cancellation before publishing staged rows |
| S11 | `TestV18UpgradePreservesLegacyReviewAndRecovery`; `TestInvestmentReviewThirtyDaySnoozeAndBackupPersistence` |
| S12 | `TestReviewHistoricalSourceTokenDetectsQuoteDeletionWithoutGenerationBump`; `TestReviewPublicationRejectsDeletedFinancialSource` |
| S13 | source: `ReconcileInvestmentReviewsV2` stages after financial writes and publishes through a separate repository operation; no test forces a transaction failure after an unrelated financial commit |

## API and UI

| ID | Evidence |
| --- | --- |
| C01 | `TestLegacyReviewPreferencesAdvanceCompatibilityRuleWithoutRewritingSource`; `TestReviewLegacyConversionPreviewsThenAtomicallyArchivesAndCreatesTwoRules` |
| C02 | `TestReviewEpisodeHistoryPagesResolvedAcknowledgedAndReboughtEpisodes`; `TestReviewDailyEvidencePagesClosedVotesAndExpiresOnRevision`; `TestReviewAlertPageCursorExpiresOnHandlingChange` |
| C03 | `TestV18UpgradePreservesLegacyReviewAndRecovery`; backup compatibility test coverage in `internal/infrastructure/backup` |
| C04 | `TestJSONExportReconciliationSchemaCompatibility`; `TestJSONExportStableEmptyCollectionsAndNoWrites` |
| U01 | `ReviewRuleEditor.test.tsx` visual nested editor cases; three locale files; synthetic screenshot at 1280×1000. Native zoom/theme/layout acceptance is not represented by this screenshot. |
| U02 | `ReviewEpisodeHistorySheet.test.tsx`; shared Sheet keyboard/focus behavior is tested in `src/components/ui/sheet.test.tsx`; native VoiceOver/IME acceptance is not run here. |
| U03 | `InvestmentReviewPage.test.tsx`; `ReviewRuleEditor.test.tsx`; `ReviewEpisodeHistorySheet.test.tsx` |
| U04 | owner split documented in `investment-review-daily-rollout.md`; shared App/navigation wiring is owned by PR99 and was not edited in this branch |
| U05 | `ReviewRuleEditor.test.tsx` (nested editor and multi-type selection); `TestReviewPolicyValidationAndNormalization` |
| U06 | `TestReviewCoverageIncludesUnknownWithoutAnAlertAndExcludesZeroHolding`; `TestReviewCoveragePausePreservesUnknownDailyTruth`; `InvestmentReviewPage.test.tsx` |

## Performance

| ID | Evidence |
| --- | --- |
| F01 | `TestReviewPublicationIsAtomicAndSameDayIdempotent` asserts unchanged-source cache reuse without snapshot/input replay; `TestReviewProjectionReplayInvalidatesPersistedCacheOnSourceChange` asserts complete invalidation; `TestReviewPolicyTimelineHandlesLongDailySuffixInOnePass`; `TestReviewIncrementalReplayPreservesFullDailyTrueSuffix`; `BenchmarkReviewProjectionIncremental100x20` measures the application planning stage against the typical design fixture. SQLite capture/publication and report-page p95 remain unmeasured; see below. |

## Performance measurement and limits

`BenchmarkReviewProjectionIncremental100x20` uses 100 positive holdings, 20
rolling rules, 365 days of origin/history, 100,000 valid dated quote facts,
50,000 account-cash activity effects unrelated to the tested type/cost
predicate, and 30 reusable daily slots followed by one new closed slot. Thirty
one-shot samples (`-benchtime=1x -count=30 -benchmem`) on Go 1.27.2/Linux/AMD
EPYC 9V74 measured a median of 356.80 ms/op and nearest-rank p95 of 445.17
ms/op (max 461.51 ms/op; about 320.7 MB/op and 1.74 million allocations/op).
That meets the 500 ms target for the **application planning stage** with a
54.83 ms p95 margin on this CI-class host. It is not a full request latency
claim: SQLite capture, atomic publication, and first-page read time are outside
this benchmark. The design's 150 ms first-page p95 and 3 s cold full-evaluation
goal therefore remain unaccepted. Source corrections conservatively trigger a
full rebuild; range-level invalidation and user-visible long-gap progress are
outside this change's evidence.

The 99 matrix items are individually mapped to source or executable evidence
above. Saul completed an independent read-only review and the three reported
incremental defects now have regression coverage. Native macOS Wails,
VoiceOver, Chinese IME, 200% zoom and desktop backup-restore acceptance remain
device-specific checks before merge/release.
