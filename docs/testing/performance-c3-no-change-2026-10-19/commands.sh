#!/usr/bin/env bash
# Run from the repository root with a Go 1.26 toolchain. Only synthetic test DBs.
# This reproduction is sequential. Original validation jobs overlapped; the
# formal performance process ran only after all validation jobs exited.
set -u
task_root=${NW_C3_EVIDENCE_ROOT:-/tmp/nestworth-c3-reproduction}
mkdir -p "$task_root/cache" "$task_root/mod" "$task_root/path" "$task_root/scratch" "$task_root/logs"
export GOCACHE="$task_root/cache" GOMODCACHE="$task_root/mod" GOPATH="$task_root/path" TMPDIR="$task_root/scratch" GOMAXPROCS=4
run_check() {
  check_name=$1
  shift
  "$@" > "$task_root/logs/$check_name.log" 2>&1
  check_status=$?
  printf '%s exit=%s\n' "$check_name" "$check_status" >> "$task_root/status.txt"
}
run_check focused-unit go test -p 1 ./internal/application -run '^TestReviewNoChange' -count=1 -v
run_check focused-race go test -race -p 1 ./internal/domain ./internal/infrastructure/sqlite ./internal/application -run 'TestReview(NoChange|Temporal|Source|Publication|CivilDate|FXEconomic)' -count=1 -v
run_check mcp-wails-race go test -race -p 1 ./internal/mcpserver -run 'TestReview(NoChangeMCPWailsParity|V2FundedTradeHTTPPreviewAndRefresh)' -count=1 -v
run_check fallback-race go test -race -p 1 ./internal/application -run '^TestReviewNoChange(DamagedTrackingRequiresFullRebuild|InvalidatesOutstandingFullPublication)$' -count=1 -v
run_check full-unit go test -p 1 ./...
run_check internal-vet go vet ./internal/...
run_check full-vet go vet ./...
run_check internal-build go build ./internal/...
run_check native-build go build ./cmd/nestworth
run_check format gofmt -l cmd internal
run_check whitespace git diff --check
# Run alone: no builds, other tests, race jobs, profiles or provider processes.
run_check performance env NW_C3_PERF_EVIDENCE=1 go test -p 1 ./internal/application -run '^TestReviewNoChangePerformanceEvidence$' -count=1 -timeout 20m -v
python3 docs/testing/performance-c3-no-change-2026-10-19/summarize.py "$task_root/logs/performance.log" > "$task_root/logs/summary.json"
