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
| F01 | `TestReviewPublicationIsAtomicAndSameDayIdempotent`; `TestReviewProjectionReplayInvalidatesPersistedCacheOnSourceChange`; `TestReviewPolicyTimelineHandlesLongDailySuffixInOnePass`; `TestReviewIncrementalReplayPreservesFullDailyTrueSuffix`; see the [current performance report](../development/performance-optimization-report.md) for retained checks. | Correctness evidence is retained; historical performance trials are not current acceptance gates |

## Performance measurement and limits

The historical repeated full-pipeline, paired-sample, RSS, and long-window
trials were retired. Their detailed results remain available at the
[performance cleanup base commit `dc097f8`](https://github.com/MnemosyneLab/NestworthGo/tree/dc097f8f7232639d04fa961d1eb3d6a3e8f0f20d),
but are not current budgets, acceptance failures, or runtime behavior. Current
implementation status and retained checks are summarized in the
[performance optimization report](../development/performance-optimization-report.md)
and selected by [the perf workflow](../../.github/workflows/perf.yml).
This dated document maps design items to evidence and does not define current
performance gates.

Native macOS performance is not measured in this historical evidence record.

Every matrix item has an evidence mapping above. Items with automated tests are
covered by the server-profile Go suite or Vitest suite; `Source mapped` items
are implementation checks without an isolated scenario test. The Chromium
screenshot uses the real nested editor component with synthetic hooks and is
documented alongside the image. The UI interaction and validation cases also
run under Vitest. Native macOS Wails, VoiceOver, Chinese IME, 200% zoom and
desktop backup/restore acceptance remain device-specific checks.
