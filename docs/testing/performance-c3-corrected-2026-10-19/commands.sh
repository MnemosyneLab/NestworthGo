#!/usr/bin/env bash
# Linux reproduction from repository root, Go1.26 on PATH. Synthetic DBs only.
set -u
task_root=${NW_C3_REFRESH_ROOT:-/tmp/nestworth-c3-refresh-reproduction}
mkdir -p "$task_root/cache" "$task_root/mod" "$task_root/path" "$task_root/scratch" "$task_root/logs"
export GOCACHE="$task_root/cache" GOMODCACHE="$task_root/mod" GOPATH="$task_root/path" TMPDIR="$task_root/scratch" GOMAXPROCS=4
run_check() {
  check_name=$1
  shift
  "$@" > "$task_root/logs/$check_name.log" 2>&1
  check_status=$?
  printf '%s exit=%s\n' "$check_name" "$check_status" >> "$task_root/status.txt"
}
run_check small-smoke-race env NW_C3_PERF_EVIDENCE=1 NW_C3_PERF_SIZE=small NW_C3_FALLBACK_SAMPLES=1 go test -race -tags perf -p 1 ./internal/application -run 'TestReviewNoChange(FallbackPerformanceEvidence|RSSPerformanceEvidence)$' -count=1 -timeout 5m -v
run_check vet-perf go vet -tags perf ./internal/...
run_check internal-build go build ./internal/...
run_check native-build go build ./cmd/nestworth
# Formal runs are serial and separate from all validation/profile/build jobs.
git rev-parse HEAD > "$task_root/logs/source-commit.txt"
export NW_C3_PERF_EVIDENCE=1 NW_C3_FALLBACK_SAMPLES=3
unset NW_C3_PERF_SIZE NW_C3_RSS_CHILD NW_C3_RSS_DIR
run_check warm go test -tags perf -p 1 ./internal/application -run '^TestReviewNoChangePerformanceEvidence$' -count=1 -timeout 20m -v
run_check fallback go test -tags perf -p 1 ./internal/application -run '^TestReviewNoChangeFallbackPerformanceEvidence$' -count=1 -timeout 30m -v
run_check rss go test -tags perf -p 1 ./internal/application -run '^TestReviewNoChangeRSSPerformanceEvidence$' -count=1 -timeout 15m -v
python3 docs/testing/performance-c3-corrected-2026-10-19/summarize.py "$task_root/logs" > "$task_root/logs/summary.json"
