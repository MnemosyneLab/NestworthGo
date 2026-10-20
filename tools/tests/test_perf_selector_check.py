import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location(
    "perf_selector_check", Path(__file__).resolve().parents[1] / "perf-selector-check.py"
)
selector = importlib.util.module_from_spec(spec)
spec.loader.exec_module(selector)


class PerfSelectorTests(unittest.TestCase):
    def test_deleted_alternative_fails_even_when_another_test_matches(self):
        with self.assertRaisesRegex(ValueError, "names missing: TestDeleted"):
            selector.validate("^(TestKept|TestDeleted)$", {"TestKept"})

    def test_zero_matches_fails(self):
        with self.assertRaisesRegex(ValueError, "matches no tests"):
            selector.validate("^TestRemoved$", {"TestKept"})

    def test_retained_alternatives_and_optional_benchmark(self):
        selector.validate("^(TestFirst|TestSecond)$", {"TestFirst", "TestSecond"})
        selector.validate("^BenchmarkKept$", {"BenchmarkKept"})
        selector.validate("", set())
