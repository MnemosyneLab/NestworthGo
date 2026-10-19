# Run from repository root. Synthetic isolated tests only.
export PATH=/tmp/go/bin:$PATH
export GOMODCACHE=/tmp/nestworth-gomod
export GOCACHE=/tmp/nestworth-perf-go
export GOPATH=/tmp/nestworth-gopath
export GOMAXPROCS=4
# Benchmark compiled and measured at dd394a11880ed2e9ecde7cb62644dfc57d020831.
go test -c ./internal/infrastructure/sqlite -o /tmp/nw-c1-sqlite.test
NW_SOURCE_PERF=1 /tmp/nw-c1-sqlite.test -test.run '^TestFinancialSourceWriteAmplificationEvidence$' -test.count=1 -test.v
# Run measurement alone, after build and before validation below.
go test ./... -count=1
go test ./internal/application -run 'TestSourceJournal|TestPopulatedV15FinancialUpgradeAndAliasReload|TestJSONExport' -count=1 -v
go test -race ./internal/infrastructure/sqlite ./internal/infrastructure/backup ./internal/application -run 'TestFinancialSource|TestV19Upgrade|TestSourceTrackingRestore|TestSourceJournal|TestJSONExportExcludes|TestPopulatedV15' -count=1 -v
go test ./internal/infrastructure/continuousbackup -count=1
go vet ./...
go vet ./internal/...
go build ./cmd/nestworth
go build ./internal/...
# Native bindings attempt requires the repository's documented GTK/WebKit dependencies.
(cd frontend && node scripts/ensure-bindings.mjs)
# pnpm wrapper could not use existing node_modules; same installed eslint ran directly.
(cd frontend && ./node_modules/.bin/eslint .)
gofmt -l cmd internal
git diff --check
# Additional post-audit/post-main integration checks:
go test -race ./internal/infrastructure/sqlite -run 'TestFinancialSourceEveryTableColumnAndOperation' -count=1 -v
go test ./internal/application -run 'TestFullRebuildReplaces|TestRebuildDerivedDataKeeps|TestSourceJournal' -count=1 -v
go test -race ./internal/application -run 'TestFullRebuildReplaces|TestRebuildDerivedDataKeeps' -count=1 -v
go vet ./internal/...
