#!/usr/bin/env python3
"""Select explicit fast/extended tooling tiers without changing test discovery.

Full local/release discovery still runs everything. Normal CI keeps GNU timeout
signals (including delayed startup), error classification and fail-closed retry
checks. The portable-backend replay and repeated signal/exhaustion matrix run in
the existing daily/manual perf workflow as functional tooling coverage.
"""
import argparse
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
EXTENDED_SIGNAL_CASES = {
    "test_acquisition_timeout_and_forced_kill_retry_then_success",
    "test_exhaustion_preserves_last_exit_and_stops",
}


def leaves(suite):
    for item in suite:
        if isinstance(item, unittest.TestSuite):
            yield from leaves(item)
        else:
            yield item


def is_extended(test):
    module, group, method = test.id().rsplit(".", 2)
    return module == "test_ci_install_native_deps" and (
        group == "PortableNativeDependencyTests"
        or group == "NativeDependencyTests" and method in EXTENDED_SIGNAL_CASES
    )


def selected_tests(tier):
    loader = unittest.TestLoader()
    suite = loader.discover(str(ROOT / "tools/tests"))
    if loader.errors:
        raise RuntimeError("tooling discovery failed:\n" + "\n".join(loader.errors))
    return [test for test in leaves(suite)
            if tier == "all" or is_extended(test) == (tier == "extended")]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tier", choices=("fast", "extended", "all"), default="all")
    parser.add_argument("--list", action="store_true")
    args = parser.parse_args()
    tests = selected_tests(args.tier)
    if not tests:
        parser.error("tier selected no tests")
    if args.list:
        print("\n".join(test.id() for test in tests))
        return 0
    print(f"tooling tier={args.tier} selected={len(tests)}", flush=True)
    result = unittest.TextTestRunner(verbosity=2).run(unittest.TestSuite(tests))
    return 0 if result.wasSuccessful() else 1


if __name__ == "__main__":
    sys.exit(main())
