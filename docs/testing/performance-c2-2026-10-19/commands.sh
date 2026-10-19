export PATH=/tmp/go/bin:$PATH
export GOMODCACHE=/tmp/nestworth-gomod
export GOCACHE=/tmp/nestworth-perf-go
export GOPATH=/tmp/nestworth-gopath
export GOMAXPROCS=4
# Ordinary checks below ran after the controlled C1 comparisons completed.
go test ./internal/domain ./internal/infrastructure/sqlite ./internal/application -run 'TestReviewSourceShadow|TestSourceJournal|TestReviewProjectionCapture|TestReviewPublication' -count=1 -v
go test ./internal/domain ./internal/infrastructure/sqlite ./internal/application -run 'TestReview|TestSourceJournal|TestJSONExport' -count=1
go test -race ./internal/domain ./internal/infrastructure/sqlite ./internal/application -run 'TestReviewSourceShadow|TestSourceJournal|TestReviewProjectionCapture|TestReviewPublicationIsAtomic|TestReviewPublicationRejectsConcurrent' -count=1 -v
go vet ./internal/...
go build ./internal/...
gofmt -l cmd internal
git diff --check
go test ./internal/mcpserver ./internal/wailsapi/... -run 'Review|Reconcile' -count=1
