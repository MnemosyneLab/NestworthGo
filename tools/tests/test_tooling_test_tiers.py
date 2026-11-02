import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("tooling_tiers", ROOT / "tools/run-tooling-tests.py")
tiers = importlib.util.module_from_spec(spec)
spec.loader.exec_module(tiers)


class ToolingTierTests(unittest.TestCase):
    def test_partition_is_complete_disjoint_and_keeps_fast_failure_boundaries(self):
        ids = lambda tier: {test.id() for test in tiers.selected_tests(tier)}
        fast, extended, all_tests = ids("fast"), ids("extended"), ids("all")
        self.assertFalse(fast & extended)
        self.assertEqual(fast | extended, all_tests)
        for method in (
            "test_slow_startup_keeps_calls_signals_and_retry_statuses",
            "test_transient_exhaustion_preserves_last_exit_and_stops",
            "test_unknown_and_permanent_errors_never_retry",
            "test_mixed_fetch_errors_fail_without_retry",
            "test_configuration_never_retried_including_timeout_and_network_text",
        ):
            self.assertIn(f"test_ci_install_native_deps.NativeDependencyTests.{method}", fast)
        self.assertTrue(any("PortableNativeDependencyTests." in name for name in extended))
        for method in tiers.EXTENDED_SIGNAL_CASES:
            self.assertIn(f"test_ci_install_native_deps.NativeDependencyTests.{method}", extended)
        for name in all_tests:
            if "test_ci_install_native_deps." not in name:
                self.assertIn(name, fast)

    def test_extended_checks_keep_daily_manual_trigger_and_reviewed_image(self):
        perf = (ROOT / ".github/workflows/perf.yml").read_text()
        check = (ROOT / ".github/workflows/check.yml").read_text()
        self.assertIn("python3 tools/run-tooling-tests.py --tier fast", check)
        self.assertIn("python3 tools/run-tooling-tests.py --tier extended", perf)
        self.assertIn("cron:", perf)
        self.assertIn("workflow_dispatch:", perf)
        self.assertNotIn("pull_request:", perf)
        self.assertIn("fromJSON(needs.changes.outputs.container)", perf)
