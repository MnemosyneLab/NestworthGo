"""Summarize the retained synthetic C3 test log; standard library only."""
import json
import math
import re
import sys


def duration_ns(value):
    units = {"ns": 1, "µs": 1000, "ms": 1000000, "s": 1000000000}
    match = re.fullmatch(r"([0-9.]+)(ns|µs|ms|s)", value)
    return round(float(match[1]) * units[match[2]])


def percentile(values, fraction):
    return sorted(values)[math.ceil(len(values) * fraction) - 1]


arms = {}
diagnostics = []
for line in open(sys.argv[1], encoding="utf-8"):
    if "distribution size=" in line:
        match = re.search(r"distribution size=(\w+) arm=(\w+).*rawNanoseconds=(.*)", line)
        values = json.loads(match[3])
        arms[match[1] + "/" + match[2]] = {
            "n": len(values["Timings"]),
            "median_ns": percentile(values["Timings"], .5),
            "p95_ns": percentile(values["Timings"], .95),
            "median_allocated_bytes": percentile(values["Bytes"], .5),
            "median_allocations": percentile(values["Allocs"], .5),
        }
    if "changed-workload size=" in line:
        match = re.search(r"changed-workload size=(\w+) scenario=([\w-]+) n=1 elapsed=(\S+) pairDays=(\d+)", line)
        diagnostics.append({"size": match[1], "scenario": match[2], "n": 1,
                            "elapsed_ns": duration_ns(match[3]), "pair_days": int(match[4])})

for key, summary in arms.items():
    size, arm = key.split("/")
    samples = []
    for line in open(sys.argv[1], encoding="utf-8"):
        if f"sample size={size} arm={arm} " in line:
            values = dict(re.findall(r"(queries|returnedRows|writeStatements|rowsAffected)=(\d+)", line))
            values = {k: int(v) for k, v in values.items()}
            values["reservation_ns"] = duration_ns(re.search(r"reserved-to-finish=(\S+)", line)[1])
            samples.append(values)
    assert len(samples) == summary["n"] == 30
    summary["median_write_reservation_ns"] = percentile([v["reservation_ns"] for v in samples], .5)
    summary["p95_write_reservation_ns"] = percentile([v["reservation_ns"] for v in samples], .95)
    summary["sql_sample1"] = samples[0]

assert len(arms) == 4 and len(diagnostics) == 6
print(json.dumps({"arms": arms, "single_sample_diagnostics": diagnostics}, indent=2))
