#!/usr/bin/env python3
"""Run ten alternating fresh-process samples against one synthetic DB."""
import argparse
import json
import os
import pathlib
import re
import subprocess


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("baseline_binary")
    parser.add_argument("candidate_binary")
    parser.add_argument("output_dir", type=pathlib.Path)
    parser.add_argument("database", type=pathlib.Path)
    parser.add_argument("--pairs", type=int, default=10)
    args = parser.parse_args()
    args.output_dir.mkdir(parents=True, exist_ok=True)
    args.database.parent.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ, GOMAXPROCS="4",
               NW_SERIES_EVIDENCE_DB=str(args.database),
               NW_SERIES_CPU_EVIDENCE_DIR=str(args.output_dir))
    binaries = {"baseline": args.baseline_binary,
                "shared": args.candidate_binary}
    test_args = ["-test.run", "^TestHistoricalSeriesCPUProfileEvidence$",
                 "-test.count=1", "-test.v", "-test.timeout=5m"]
    result = subprocess.run([binaries["baseline"], *test_args], env=env,
                            capture_output=True, text=True)
    (args.output_dir / "fixture.log").write_text(result.stdout + result.stderr)
    if result.returncode:
        raise SystemExit(f"synthetic fixture setup failed: {result.returncode}")
    samples = []
    fields = ["elapsed_ns", "alloc_bytes", "allocs", "facts", "bytes",
              "leaves", "queries", "points"]
    pattern = re.compile(
        r"elapsed_ns=(\d+) alloc_bytes=(\d+) allocs=(\d+) facts=(\d+) "
        r"bytes=(\d+) leaves=(\d+) queries=(\d+) points=(\d+)")
    for index in range(args.pairs):
        order = ["baseline", "shared"] if index % 2 == 0 else ["shared", "baseline"]
        for kind in order:
            result = subprocess.run([binaries[kind], *test_args], env=env,
                                    capture_output=True, text=True)
            raw = result.stdout + result.stderr
            (args.output_dir / f"{index + 1:02d}-{kind}.log").write_text(raw)
            if result.returncode:
                raise SystemExit(f"{kind} sample {index + 1} failed: {result.returncode}")
            match = pattern.search(raw)
            if not match:
                raise SystemExit(f"{kind} sample {index + 1} missing metrics")
            samples.append(dict(sample=index + 1, path=kind,
                                **dict(zip(fields, map(int, match.groups())))))
            (args.output_dir / "samples.json").write_text(
                json.dumps(samples, indent=2) + "\n")
            print(index + 1, kind, samples[-1]["elapsed_ns"], flush=True)


if __name__ == "__main__":
    main()
