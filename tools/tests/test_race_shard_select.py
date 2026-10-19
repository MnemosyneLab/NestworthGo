"""Narrow checks for the race-shard selector. No race suite, no ledger."""
import re
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "tools/race-shard-select.sh"
CHECK = ROOT / ".github/workflows/check.yml"
PERF = ROOT / ".github/workflows/perf.yml"

# Representative -list lines. Benchmarks and noise must stay out of -run.
LISTING = "\n".join([
    "TestAlpha",
    "ExampleBeta",
    "FuzzGamma",
    "BenchmarkSkip",
    "Example",
    "FuzzSeeded",
    "ExampleFoo_Bar",
    "ok      github.com/example      0.1s",
    "TestWith_Underscore",
    "not a test",
    "",
]) + "\n"

RUNNABLE = [
    "TestAlpha",
    "ExampleBeta",
    "FuzzGamma",
    "Example",
    "FuzzSeeded",
    "ExampleFoo_Bar",
    "TestWith_Underscore",
]


class RaceShardSelectTests(unittest.TestCase):
    def run_select(self, *args, stdin=None):
        return subprocess.run(
            ["bash", str(SCRIPT), *map(str, args)],
            input=stdin,
            capture_output=True,
            text=True,
            cwd=ROOT,
        )

    def names_from_regex(self, regex):
        self.assertTrue(regex.startswith("^("), regex)
        self.assertTrue(regex.endswith(")$"), regex)
        inner = regex[len("^("):-len(")$")]
        return inner.split("|") if inner else []

    def assigned_part(self, name, parts):
        completed = subprocess.run(
            ["cksum"],
            input=name.encode(),
            capture_output=True,
            check=True,
        )
        checksum = int(completed.stdout.split()[0])
        return checksum % parts

    def test_representative_names_partition_is_disjoint_and_complete(self):
        parts = 2
        selected = {}
        for part in range(parts):
            result = self.run_select("--names", part, parts, stdin=LISTING)
            self.assertEqual(result.returncode, 0, result.stderr)
            names = self.names_from_regex(result.stdout.strip())
            selected[part] = names
            for name in names:
                self.assertIn(name, RUNNABLE)
                self.assertEqual(self.assigned_part(name, parts), part)
            self.assertRegex(result.stdout.strip(), r"^\^[()A-Za-z0-9_|]+\$")

        union = [name for names in selected.values() for name in names]
        self.assertCountEqual(union, RUNNABLE)
        self.assertEqual(len(union), len(set(union)))
        self.assertTrue(any(name.startswith("Example") for name in union))
        self.assertTrue(any(name.startswith("Fuzz") for name in union))
        self.assertNotIn("BenchmarkSkip", union)

        covered = {prefix for name in union for prefix in ("Test", "Example", "Fuzz") if name.startswith(prefix)}
        self.assertEqual(covered, {"Test", "Example", "Fuzz"})

    def matrix_parts(self, workflow, shard):
        packed = re.findall(
            rf"(?m)^[ \t]*{shard} (\d+) (\d+)[ \t]*$",
            workflow,
        )
        if packed:
            return [(int(part), int(count), f"{shard} {int(part) + 1}/{int(count)}") for part, count in packed]
        entries = re.findall(
            rf"- shard: {shard}\s+part: (\d+)\s+parts: (\d+)\s+label: ([^\n]+)",
            workflow,
        )
        return [(int(part), int(count), label) for part, count, label in entries]

    def assert_split_covers_names(self, workflow, totals):
        # Exercise the actual matrix, including missing/duplicate parts and
        # inconsistent denominators, rather than another hardcoded 2-part run.
        listing = LISTING + "".join(f"TestGenerated{i}\n" for i in range(80))
        expected = RUNNABLE + [f"TestGenerated{i}" for i in range(80)]
        for shard, total in totals:
            parts = self.matrix_parts(workflow, shard)
            self.assertEqual(sorted(part for part, _, _ in parts), list(range(total)), shard)
            union = []
            for part, count, label in parts:
                self.assertEqual(count, total)
                self.assertEqual(label, f"{shard} {part + 1}/{total}")
                result = self.run_select("--names", str(part), str(count), stdin=listing)
                self.assertEqual(result.returncode, 0, result.stderr)
                union.extend(self.names_from_regex(result.stdout.strip()))
            self.assertCountEqual(union, expected)
            self.assertEqual(len(union), len(set(union)))

    def test_workflow_split_parts_cover_every_name_once(self):
        check = CHECK.read_text()
        self.assert_split_covers_names(check, (("application", 2), ("mcpserver", 1), ("sqlite", 1)))
        self.assertEqual(self.matrix_parts(check, "continuousbackup"), [(0, 1, "continuousbackup 1/1")])
        self.assertEqual(self.matrix_parts(check, "remainder"), [(0, 1, "remainder 1/1")])
        self.assertEqual(check.count("selections: |"), 3)
        self.assert_split_covers_names(PERF.read_text(), (("application", 8), ("mcpserver", 4), ("sqlite", 2)))

    def test_workflow_keeps_bounded_race_execution_and_fail_closed_aggregate(self):
        workflow = PERF.read_text()
        self.assertIn("timeout-minutes: 30", workflow)
        self.assertIn("fail-fast: false", workflow)
        self.assertIn("set -euo pipefail", workflow)
        self.assertIn("go test -race -count=1 -json -timeout 10m", workflow)
        self.assertIn("pkg_parallel=(-p 1)", workflow)
        self.assertIn("pkg_parallel=(-p 2)", workflow)
        self.assertNotIn("pkg_parallel=(-p 4)", workflow)
        self.assertIn('go list ./... > "$RUNNER_TEMP/race-packages.txt"', workflow)
        self.assertNotIn("< <(go list", workflow)
        self.assertIn("actions/upload-artifact@v4", workflow)
        self.assertIn("name: race-${{ matrix.shard }}-${{ matrix.part }}-attempt-${{ github.run_attempt }}", workflow)
        aggregate = workflow.split("\n  race:\n", 1)[1]
        self.assertIn("needs: [changes, race-shard]", aggregate)
        self.assertIn(
            "if: always() && needs.changes.result == 'success' && needs.changes.outputs.should_run == 'true'",
            aggregate,
        )
        self.assertIn('if [ "${{ needs.race-shard.result }}" != "success" ]; then', aggregate)
        self.assertIn("exit 1", aggregate)
        self.assertNotIn("continue-on-error", workflow)

    def test_successful_enumeration_uses_the_same_selection(self):
        result = self.run_select(
            "--enumerate", "0", "2", "--",
            "bash", "-c", "cat",
            stdin=LISTING,
        )
        direct = self.run_select("--names", "0", "2", stdin=LISTING)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, direct.stdout)
        names = self.names_from_regex(result.stdout.strip())
        self.assertTrue(names)
        self.assertTrue(all(name.startswith(("Test", "Example", "Fuzz")) for name in names))

    def test_enumeration_failure_discards_partial_list(self):
        result = self.run_select(
            "--enumerate", "0", "2", "--",
            "bash", "-c", "printf '%s\\n' TestPartial ExampleKept FuzzKept; exit 9",
        )
        self.assertEqual(result.returncode, 9, result.stderr)
        self.assertEqual(result.stdout, "")
        self.assertIn("discarding partial list", result.stderr)
        self.assertNotIn("TestPartial", result.stdout)
        self.assertNotIn("^(", result.stdout)

    def test_enumeration_failure_with_no_output_fails_closed(self):
        result = self.run_select("--enumerate", "0", "2", "--", "false")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "")
        self.assertIn("discarding partial list", result.stderr)

    def test_empty_and_benchmark_only_lists_fail_closed(self):
        empty = self.run_select("--names", "0", "2", stdin="")
        benchmarks = self.run_select("--names", "0", "2", stdin="BenchmarkSkip\nok pkg\n")
        self.assertEqual(empty.returncode, 1, empty.stderr)
        self.assertEqual(benchmarks.returncode, 1, benchmarks.stderr)
        self.assertEqual(empty.stdout, "")
        self.assertEqual(benchmarks.stdout, "")
        self.assertIn("no runnable", empty.stderr)

    def test_empty_part_fails_closed(self):
        results = [self.run_select("--names", part, "2", stdin="TestOnlyOne\n") for part in (0, 1)]
        codes = sorted(result.returncode for result in results)
        self.assertEqual(codes, [0, 1])
        for result in results:
            if result.returncode == 0:
                self.assertEqual(self.names_from_regex(result.stdout.strip()), ["TestOnlyOne"])
            else:
                self.assertEqual(result.stdout, "")
                self.assertIn("selected no names", result.stderr)

    def test_unsafe_name_fails_closed(self):
        result = self.run_select("--names", "0", "2", stdin="TestFoo.Bar\n")
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(result.stdout, "")
        self.assertIn("not a Go identifier", result.stderr)

    def test_workflow_delegates_to_race_list_selector(self):
        check = CHECK.read_text()
        perf = PERF.read_text()
        script = SCRIPT.read_text()
        self.assertNotIn("go test -race", check)
        self.assertIn("go test -count=1 -list .", check)
        self.assertNotIn("go test -list .", perf)
        self.assertNotIn("grep '^Test'", check)
        self.assertNotIn("grep '^Test'", perf)
        self.assertIn("bash tools/race-shard-select.sh --enumerate", check)
        self.assertIn('bash tools/race-shard-select.sh "$part" "$parts"', perf)
        self.assertIn('go test -race -list . "$1"', script)
        self.assertIn("discarding partial list", script)

    def test_workflow_keeps_every_gate_and_partitions_infrastructure(self):
        workflow = CHECK.read_text()
        self.assertIn("push:", workflow)
        self.assertIn("branches: [main]", workflow)
        self.assertIn("pull_request:", workflow)
        self.assertIn("workflow_dispatch:", workflow)
        self.assertIn("needs: [meta, frontend, go-shard]", workflow)
        self.assertIn("name: test", workflow)
        self.assertIn("pnpm exec vitest run --shard=${{ matrix.shard }}/${{ matrix.shards }}", workflow)
        self.assertIn("shard: 1", workflow)
        self.assertIn("shards: 2", workflow)
        self.assertIn("shard: 2", workflow)
        self.assertIn("lint: true", workflow)
        self.assertIn("build: true", workflow)
        self.assertIn("NESTWORTH_WAILS_GOCACHE=$GOCACHE", workflow)
        self.assertIn("go test -count=1 -json -timeout 20m", workflow)
        self.assertNotIn("go test ./...", workflow)
        self.assertIn("go vet ./...", workflow)
        self.assertIn("go build ./cmd/nestworth", workflow)
        self.assertIn("pnpm run lint", workflow)
        self.assertIn("pnpm run typecheck", workflow)
        self.assertIn("pnpm run build", workflow)
        self.assertIn("pnpm run generate:bindings", workflow)
        self.assertIn("pnpm run check:bindings", workflow)
        self.assertIn("python3 -m unittest discover -s tools/tests -v", workflow)
        self.assertNotIn("-tags perf", workflow)
        self.assertNotIn("NESTWORTH_PERF", workflow)
        self.assertIn("pkgs=(./internal/infrastructure/sqlite)", workflow)
        self.assertIn("pkgs=(./internal/infrastructure/continuousbackup)", workflow)
        self.assertIn("continuousbackup 0 1", workflow)
        self.assertIn("go list ./internal/infrastructure/...", workflow)
        self.assertIn("infrastructure partition does not cover go list", workflow)
        self.assertIn("infrastructure package missing from remainder", workflow)
        self.assertIn("go list does not contain exactly the four dedicated packages", workflow)
        self.assertIn('if [ "${item#*=}" != "success" ]; then', workflow)
        self.assertIn("go shard selected no packages", workflow)
        # Remainder may overlap package builds. Heavy shards stay at -p 1.
        remainder = workflow.split("remainder)", 1)[1].split("*)", 1)[0]
        self.assertIn("pkg_parallel=(-p 2)", remainder)
        self.assertNotIn("-parallel", remainder)
        perf = PERF.read_text()
        self.assertNotIn("push:", perf)
        self.assertNotIn("pull_request:", perf)
        self.assertIn('cron: "0 21 * * *"', perf)
        self.assertNotIn("NESTWORTH_PERF", perf)
        self.assertIn("go test -tags perf -count=1 -timeout", perf)
        self.assertIn("-benchtime=1x", perf)
        self.assertIn("TestReviewFullPipelinePerformanceEvidence", perf)
        self.assertIn("TestReviewPairedPerformanceEvidence", perf)
        self.assertIn("TestReviewEpochStageDiagnostics", perf)
        self.assertIn("timeout: 110", perf)
        race = perf.split("\n  race-shard:\n", 1)[1]
        self.assertNotIn("-tags perf", race)
        self.assertNotIn("-bench", race)
