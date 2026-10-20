#!/bin/bash
# Decide whether a perf.yml run should execute the perf and race jobs.
#
#   EVENT_NAME=schedule CURRENT_SHA=... RUN_ID=... \
#     [DEFAULT_BRANCH=main] bash tools/perf-schedule-gate.sh runs.json
#
# stdout is exactly should_run=true or should_run=false. Reasons go to stderr.
# A non-zero status is a gate failure, not a skip: the caller must not treat
# it as "no changes".
#
# Scheduled runs compare CURRENT_SHA with the newest previous successful
# scheduled run on the default branch. workflow_dispatch and any other event
# always run. The baseline is scheduled successes only, so a manual run cannot
# move it and a run on another branch cannot cover the default branch.
# No-change skips are themselves successful scheduled runs; their head SHA is
# a commit an earlier scheduled run already tested, so they are the same
# baseline as that execution.
#
# Failed and cancelled runs are not successes. They never become the baseline,
# so a failed or cancelled attempt does not mark its commit done. An older
# success on a different commit does not cover the new one. An older success
# on the same commit still counts: the later failure did not erase it.
#
# runs.json is the Actions "list workflow runs" payload, newest-first or not.
# The script orders by run_started_at / created_at / updated_at. It ignores
# the current RUN_ID so an in-progress row cannot suppress itself.

set -euo pipefail

if [ "${EVENT_NAME:-}" != "schedule" ]; then
  echo "non-schedule event ${EVENT_NAME:-<empty>} always executes" >&2
  echo "should_run=true"
  exit 0
fi

if [ -z "${CURRENT_SHA:-}" ]; then
  echo "scheduled run requires CURRENT_SHA" >&2
  exit 1
fi

if [ "${1:-}" = "" ] || [ ! -f "$1" ]; then
  echo "scheduled run requires workflow-runs JSON" >&2
  exit 1
fi

python3 - "$1" "${RUN_ID:-}" "$CURRENT_SHA" "${DEFAULT_BRANCH:-}" <<'PY'
import json
import sys

path, run_id, current, branch = sys.argv[1:]
with open(path, encoding="utf-8") as handle:
    payload = json.load(handle)
runs = payload.get("workflow_runs") or []

def stamp(run):
    return run.get("run_started_at") or run.get("created_at") or run.get("updated_at") or ""

previous = ""
for run in sorted(runs, key=stamp, reverse=True):
    if str(run.get("id", "")) == str(run_id):
        continue
    if run.get("event") != "schedule":
        continue
    if run.get("conclusion") != "success":
        continue
    head_branch = run.get("head_branch") or ""
    if branch and head_branch and head_branch != branch:
        continue
    sha = run.get("head_sha") or ""
    if not sha:
        continue
    previous = sha
    break

if not previous:
    print("no previous successful scheduled run; executing", file=sys.stderr)
    print("should_run=true")
elif previous == current:
    print(
        f"default branch {current} matches the last successful scheduled run; skipping",
        file=sys.stderr,
    )
    print("should_run=false")
else:
    print(
        f"default branch moved from {previous} to {current}; executing",
        file=sys.stderr,
    )
    print("should_run=true")
PY
