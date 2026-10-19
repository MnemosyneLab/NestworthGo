# From repository root; synthetic isolated data only.
export PATH=/tmp/go/bin:$PATH
export GOMODCACHE=/tmp/nestworth-gomod
export GOCACHE=/tmp/nestworth-perf-go
export GOPATH=/tmp/nestworth-gopath
export GOMAXPROCS=4
# Reproduction was run at 2f8b4ec before changing source trigger timing:
go test ./internal/infrastructure/sqlite -run 'TestFinancialSourceInsertConflictSemantics|TestFinancialSourceVerifierRejectsBeforeInsertDefinition' -count=1 -v
# Corrected code + bounded retention checks:
go test ./internal/infrastructure/sqlite -run 'TestFinancialSource|TestV19Upgrade' -count=1 -v
# Compile all measurement executables before measurement. Code commit0a7c306.
go test -c ./internal/infrastructure/sqlite -o /tmp/nw-c1-insert-sqlite.test
go test -c ./internal/infrastructure/marketdata -o /tmp/nw-c1-insert-marketdata.test
go test -c ./internal/application -o /tmp/nw-c1-insert-app.test
# Each following measurement ran alone; no concurrent build/test/race/profiling.
NW_SOURCE_PERF=1 /tmp/nw-c1-insert-sqlite.test -test.run '^TestFinancialSourceWriteAmplificationEvidence$' -test.count=1 -test.v
NW_SOURCE_BATCH_PERF=1 /tmp/nw-c1-insert-marketdata.test -test.run '^TestSourceTrackingProviderImportBatchEvidence$' -test.count=1 -test.v
NW_SOURCE_BATCH_PERF=1 /tmp/nw-c1-insert-app.test -test.run '^TestSourceTrackingProviderRefreshEvidence$' -test.count=1 -test.v
# Validation occurred after all measurements (initial broad unit/race overlapped).
go test ./internal/... -count=1
go test -race ./internal/infrastructure/sqlite ./internal/infrastructure/backup ./internal/application -run 'TestFinancialSource|TestV19Upgrade|TestSourceTrackingRestore|TestSourceJournal' -count=1 -v
go test -race ./internal/application -run '^TestSourceJournalStaleFloorStillUsesFullFingerprintOracle$' -count=1 -v
go vet ./internal/...
go build ./internal/...
gofmt -l cmd internal
git diff --check
# Original timing gates rerun alone, after all race/build activity stops:
go test ./internal/application -run '^(TestAnalysisPerformanceBudgets|TestHistoricalOverviewDenseEntrySingleAndComparison)$' -count=1 -v
