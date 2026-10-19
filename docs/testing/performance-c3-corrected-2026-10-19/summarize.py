"""Summarize corrected C3 evidence, keeping main latency/RSS runs separate."""
import json
import math
from pathlib import Path
import re
import sys


def ns(value):
    units = {"ns": 1, "µs": 1000, "ms": 10**6, "s": 10**9}
    match = re.fullmatch(r"([0-9.]+)(ns|µs|ms|s)", value)
    if not match:
        raise ValueError(value)
    return round(float(match[1]) * units[match[2]])


def rank(values, fraction=.5):
    return sorted(values)[math.ceil(len(values) * fraction) - 1]


root = Path(sys.argv[1])
warm, fallbacks, rss, bootstrap, diagnostics = {}, {}, {}, [], []
for line in (root / "warm.log").read_text().splitlines():
    if "distribution size=" in line:
        match = re.search(r"distribution size=(\w+) arm=(\w+).*rawNanoseconds=(.*)", line)
        values = json.loads(match[3])
        assert len(values["Timings"]) == 30
        warm[f"{match[1]}/{match[2]}"] = {
            "n": 30, "median_ns": rank(values["Timings"]),
            "p95_ns": rank(values["Timings"], .95),
            "median_allocated_bytes": rank(values["Bytes"]),
            "median_allocations": rank(values["Allocs"]),
        }
    if "session-bootstrap size=" in line:
        match = re.search(r"size=(\w+) arm=(\w+) n=1 elapsed=(\S+) pairDays=(\d+)", line)
        bootstrap.append({"size": match[1], "arm": match[2], "n": 1,
                          "elapsed_ns": ns(match[3]), "pair_days": int(match[4])})
    if "changed-workload size=" in line:
        match = re.search(r"size=(\w+) scenario=([\w-]+) n=1 elapsed=(\S+) pairDays=(\d+)", line)
        diagnostics.append({"size": match[1], "scenario": match[2], "n": 1,
                            "elapsed_ns": ns(match[3]), "pair_days": int(match[4])})

for line in (root / "fallback.log").read_text().splitlines():
    match = re.search(r"fallback size=(\w+) scenario=([\w-]+) reference=([\w-]+) arm=(\w+) n=(\d+) elapsed=(\S+) pairDays=(\d+) (.*)", line)
    if not match:
        continue
    key = f"{match[1]}/{match[2]}/{match[4]}"
    entry = fallbacks.setdefault(key, {"reference": match[3], "samples": []})
    entry["samples"].append({"elapsed_ns": ns(match[6]), "pair_days": int(match[7]), "sql": match[8]})
for entry in fallbacks.values():
    times = [sample["elapsed_ns"] for sample in entry["samples"]]
    assert len(times) == 3
    entry.update(n=3, median_ns=rank(times), min_ns=min(times), max_ns=max(times))

for line in (root / "rss.log").read_text().splitlines():
    match = re.search(r"rss size=(\w+) pair=(\d+) result=(\{.*\}) \(", line)
    if not match:
        continue
    values = json.loads(match[3])
    assert len(values["Times"]) == 3
    rss.setdefault(f"{match[1]}/{values['Arm']}", {"processes": []})["processes"].append(values)
for entry in rss.values():
    processes = entry["processes"]
    assert len(processes) == 3
    peaks = [value["SampledPeakRSSKiB"] for value in processes]
    entry.update(process_count=3, operations_per_process=3,
                 median_sampled_peak_rss_kib=rank(peaks),
                 min_sampled_peak_rss_kib=min(peaks), max_sampled_peak_rss_kib=max(peaks),
                 median_ready_rss_kib=rank([v["ReadyRSSKiB"] for v in processes]),
                 median_lifetime_hwm_kib=rank([v["EndHWMKiB"] for v in processes]))

assert len(warm) == 4 and len(fallbacks) == 12 and len(rss) == 4
assert len(bootstrap) == 4 and len(diagnostics) == 6
print(json.dumps({"warm": warm, "paired_fallback_diagnostics": fallbacks,
                  "isolated_rss": rss, "session_bootstrap_diagnostics": bootstrap,
                  "single_changed_diagnostics": diagnostics}, indent=2))
