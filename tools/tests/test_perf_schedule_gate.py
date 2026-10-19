"""Decision table for the daily perf schedule gate. No Actions calls."""
import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "tools/perf-schedule-gate.sh"
PERF = ROOT / ".github/workflows/perf.yml"

SHA_A = "a" * 40
SHA_B = "b" * 40


def run_record(run_id, sha, event="schedule", conclusion="success", stamp="2026-10-01T00:00:00Z", branch="main"):
    return {
        "id": run_id,
        "head_sha": sha,
        "event": event,
        "conclusion": conclusion,
        "head_branch": branch,
        "created_at": stamp,
        "run_started_at": stamp,
    }


class PerfScheduleGateTests(unittest.TestCase):
    def decide(self, event, sha, runs, run_id="9000", branch="main", path=None):
        env = os.environ.copy()
        env.update({
            "EVENT_NAME": event,
            "CURRENT_SHA": sha,
            "RUN_ID": str(run_id),
            "DEFAULT_BRANCH": branch,
        })
        if path is None:
            handle = tempfile.NamedTemporaryFile("w", suffix=".json", delete=False)
            json.dump({"workflow_runs": runs}, handle)
            handle.close()
            path = handle.name
            self.addCleanup(os.remove, path)
        return subprocess.run(
            ["bash", str(SCRIPT), path],
            env=env,
            text=True,
            capture_output=True,
            check=False,
        )

    def assert_runs(self, result):
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), "should_run=true")

    def assert_skips(self, result):
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), "should_run=false")

    def test_manual_dispatch_always_runs(self):
        result = self.decide("workflow_dispatch", SHA_A, [run_record(1, SHA_A)])
        self.assert_runs(result)

    def test_first_scheduled_run_executes(self):
        self.assert_runs(self.decide("schedule", SHA_A, []))

    def test_same_sha_as_last_scheduled_success_skips(self):
        self.assert_skips(self.decide("schedule", SHA_A, [run_record(1, SHA_A)]))

    def test_new_commit_since_last_scheduled_success_runs(self):
        self.assert_runs(self.decide("schedule", SHA_B, [run_record(1, SHA_A, stamp="2026-10-01T00:00:00Z")]))

    def test_failed_and_cancelled_runs_do_not_cover_a_commit(self):
        runs = [
            run_record(3, SHA_B, conclusion="failure", stamp="2026-10-03T00:00:00Z"),
            run_record(2, SHA_B, conclusion="cancelled", stamp="2026-10-02T00:00:00Z"),
            run_record(1, SHA_A, conclusion="success", stamp="2026-10-01T00:00:00Z"),
        ]
        self.assert_runs(self.decide("schedule", SHA_B, runs))

    def test_later_failure_does_not_erase_success_on_the_same_commit(self):
        runs = [
            run_record(2, SHA_A, conclusion="failure", stamp="2026-10-02T00:00:00Z"),
            run_record(1, SHA_A, conclusion="success", stamp="2026-10-01T00:00:00Z"),
        ]
        self.assert_skips(self.decide("schedule", SHA_A, runs))

    def test_manual_success_does_not_move_the_scheduled_baseline(self):
        runs = [
            run_record(2, SHA_B, event="workflow_dispatch", stamp="2026-10-02T00:00:00Z"),
            run_record(1, SHA_A, event="schedule", stamp="2026-10-01T00:00:00Z"),
        ]
        self.assert_runs(self.decide("schedule", SHA_B, runs))
        only_manual = [run_record(2, SHA_B, event="workflow_dispatch")]
        self.assert_runs(self.decide("schedule", SHA_B, only_manual))

    def test_current_run_is_not_its_own_baseline(self):
        runs = [
            run_record(9, SHA_A, stamp="2026-10-02T00:00:00Z"),
            run_record(1, SHA_B, stamp="2026-10-01T00:00:00Z"),
        ]
        self.assert_runs(self.decide("schedule", SHA_A, runs, run_id=9))

    def test_timestamp_order_beats_payload_order(self):
        runs = [
            run_record(1, SHA_B, stamp="2026-10-01T00:00:00Z"),
            run_record(2, SHA_A, stamp="2026-10-03T00:00:00Z"),
        ]
        self.assert_skips(self.decide("schedule", SHA_A, runs))

    def test_other_branch_success_does_not_cover_the_default_branch(self):
        runs = [run_record(1, SHA_A, branch="feature")]
        self.assert_runs(self.decide("schedule", SHA_A, runs, branch="main"))

    def test_missing_sha_or_json_fails_closed(self):
        missing_sha = self.decide("schedule", "", [run_record(1, SHA_A)])
        self.assertNotEqual(missing_sha.returncode, 0)
        self.assertNotIn("should_run=false", missing_sha.stdout)
        missing_file = self.decide("schedule", SHA_A, [], path="/tmp/does-not-exist-perf-schedule.json")
        self.assertNotEqual(missing_file.returncode, 0)
        self.assertEqual(missing_file.stdout, "")

    def test_workflow_wires_the_gate_without_failing_a_no_change_skip(self):
        workflow = PERF.read_text()
        self.assertIn('cron: "0 21 * * *"', workflow)
        self.assertNotIn("0 18 * * 0", workflow)
        self.assertIn("workflow_dispatch:", workflow)
        self.assertNotIn("push:", workflow)
        self.assertNotIn("pull_request:", workflow)
        self.assertIn("actions: read", workflow)
        self.assertIn("bash tools/perf-schedule-gate.sh", workflow)
        self.assertIn("should_run: ${{ steps.decide.outputs.should_run }}", workflow)
        job_if = "needs.changes.result == 'success' && needs.changes.outputs.should_run == 'true'"
        self.assertGreaterEqual(workflow.count(job_if), 3)
        aggregate = workflow.split("\n  race:\n", 1)[1]
        self.assertIn("needs: [changes, race-shard]", aggregate)
        self.assertIn("if: always() && " + job_if, aggregate)
        self.assertIn('if [ "${{ needs.race-shard.result }}" != "success" ]; then', aggregate)
