# Run from repository root with Go 1.26 and writable caches.
unset NW_REVIEW_PERF_EVIDENCE NW_REVIEW_PERF_STAGE_TRACE
export GOMAXPROCS=4
go test -count=1 -p=1 ./internal/application ./internal/domain ./internal/infrastructure/sqlite \
  -run '^(TestReview(PerformanceFullOracle|RepeatedEpochWindowOracle|ReentryEpisodeFullReplayAcrossMidnight|FullOracleRejectsDaySetMutations|ReusableWindowEquivalent|WindowCandidateUniverse)|TestHistoricalSnapshotInverseFXPrecisionAcrossStorageTrendAndHealth|TestBaseAmountExactPreservesFXPrecisionAndBounds|TestSnapshotBaseAmountSupportsMaximumSingleFXScale|TestSaveSnapshotRejectsDanglingQuoteProvenance|TestVerifyRejectsDanglingSnapshotQuoteProvenance)$' -v
go test -race -count=1 -timeout=10m -p=1 ./internal/application ./internal/domain ./internal/infrastructure/sqlite \
  -run '^(TestReview(PerformanceFullOracle|ReentryEpisodeFullReplayAcrossMidnight|ReusableWindowEquivalent|WindowCandidateUniverse)|TestHistoricalSnapshotInverseFXPrecisionAcrossStorageTrendAndHealth|TestBaseAmountExactPreservesFXPrecisionAndBounds|TestSnapshotBaseAmountSupportsMaximumSingleFXScale)$' -v
go vet ./internal/application ./internal/domain ./internal/infrastructure/sqlite
