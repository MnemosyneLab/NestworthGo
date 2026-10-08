"""Narrow checks for the race-shard selector. No race suite, no ledger."""
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "tools/race-shard-select.sh"
WORKFLOW = ROOT / ".github/workflows/check.yml"

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
        workflow = WORKFLOW.read_text()
        script = SCRIPT.read_text()
        self.assertNotIn("go test -list .", workflow)
        self.assertNotIn("grep '^Test'", workflow)
        self.assertIn("bash tools/race-shard-select.sh", workflow)
        self.assertIn('go test -race -list . "$1"', script)
        self.assertIn("discarding partial list", script)
