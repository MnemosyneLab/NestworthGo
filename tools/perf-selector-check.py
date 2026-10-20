#!/usr/bin/env python3
"""Fail before expensive perf runs when a retained selector lost its tests."""
import argparse
import re
import subprocess
import sys


def validate(selector, names):
    if not selector:
        return
    pattern = re.compile(selector)
    if not any(pattern.search(name) for name in names):
        raise ValueError(f"perf selector matches no tests: {selector}")
    # Current workflow selectors are exact named alternatives. Check each member,
    # so deleting one test cannot hide behind another successful alternative.
    expected = set(re.findall(r"\b(?:Test|Benchmark)\w+", selector))
    missing = expected - names
    if missing:
        raise ValueError(f"perf selector names missing: {', '.join(sorted(missing))}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run", required=True)
    parser.add_argument("--bench", default="")
    parser.add_argument("packages", nargs="+")
    args = parser.parse_args()
    result = subprocess.run(
        ["go", "test", "-tags", "perf", "-list", ".", *args.packages],
        text=True, capture_output=True, check=False,
    )
    if result.returncode:
        sys.stderr.write(result.stdout + result.stderr)
        return result.returncode
    names = set(re.findall(r"^(?:Test|Benchmark)\w+$", result.stdout, re.MULTILINE))
    try:
        if not args.run:
            raise ValueError("perf run selector must be explicit")
        validate(args.run, names)
        validate(args.bench, names)
    except ValueError as error:
        print(error, file=sys.stderr)
        return 1
    print("Perf selectors match retained tests and benchmarks.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
