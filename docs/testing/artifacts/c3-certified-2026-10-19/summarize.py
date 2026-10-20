#!/usr/bin/env python3
"""Summarize raw C3 measurements; n=3 is diagnostic, never p95 acceptance."""
import json
import re
import statistics
from pathlib import Path

root = Path(__file__).parent
rows = []
pattern = re.compile(r"certified(?:-lifecycle)? size=(\S+) scenario=(\S+) arm=(\S+) n=(\d+) elapsed=(\S+) allocatedBytes=(\d+) allocs=(\d+) publishedPairDays=(\d+)")

def milliseconds(text):
    units = {"h": 3600000, "m": 60000, "s": 1000, "ms": 1, "µs": .001, "ns": .000001}
    return sum(float(n) * units[u] for n, u in re.findall(r"([0-9.]+)(ms|µs|ns|h|m|s)", text))

for name in ("paired-small-large.log", "bootstrap-activation.log"):
    path = root / name
    if not path.exists():
        continue
    for line in path.read_text().splitlines():
        found = pattern.search(line)
        if found:
            size, scenario, arm, sample, elapsed, allocated, allocs, days = found.groups()
            rows.append(dict(log=name, size=size, scenario=scenario, arm=arm,
                             sample=int(sample), elapsed_ms=milliseconds(elapsed),
                             allocated_bytes=int(allocated), allocs=int(allocs),
                             published_pair_days=int(days)))
(root / "samples.json").write_text(json.dumps(rows, indent=2) + "\n")
print("| Size | Scenario | Arm | n | Median ms | Median allocated MiB | Published pair-days |")
print("|---|---|---|---:|---:|---:|---|")
keys = sorted({(r['size'], r['scenario'], r['arm']) for r in rows})
for size, scenario, arm in keys:
    group = [r for r in rows if (r['size'], r['scenario'], r['arm']) == (size, scenario, arm)]
    print(f"| {size} | {scenario} | {arm} | {len(group)} | "
          f"{statistics.median(r['elapsed_ms'] for r in group):.3f} | "
          f"{statistics.median(r['allocated_bytes'] for r in group)/1048576:.3f} | "
          f"{sorted({r['published_pair_days'] for r in group})} |")
