# Separate immutable builds, same toolchain/cache/runtime; no concurrent measurement work.
export PATH=/tmp/go/bin:$PATH
export GOMODCACHE=/tmp/nestworth-gomod
export GOCACHE=/tmp/nestworth-perf-go
export GOPATH=/tmp/nestworth-gopath
export GOMAXPROCS=4
# Base worktree63fc5e1 and failed-head worktree0cfd711 were compiled separately:
go test -c ./internal/application -o /workspace/nw-c1-base-app.test
go test -c -race ./internal/application -o /workspace/nw-c1-base-app-race.test
# Same commands at failed head, outputs nw-c1-failed-app.test / nw-c1-failed-app-race.test.
/workspace/nw-c1-base-app.test -test.run '^TestHistoricalOverviewDenseEntrySingleAndComparison$' -test.count=1 -test.v
/workspace/nw-c1-failed-app.test -test.run '^TestHistoricalOverviewDenseEntrySingleAndComparison$' -test.count=1 -test.v
/workspace/nw-c1-base-app-race.test -test.run '^TestHistoricalSeriesBenchmarkCancellationEvidence$' -test.count=1 -test.parallel=4 -test.timeout=10m -test.v
/workspace/nw-c1-failed-app-race.test -test.run '^TestHistoricalSeriesBenchmarkCancellationEvidence$' -test.count=1 -test.parallel=4 -test.timeout=10m -test.v
