#!/usr/bin/env python3
"""Retain every gate outcome and verify the Go package population completed."""
import json
import os
from pathlib import Path

GATES = ('bindings', 'format', 'go-tests', 'vet', 'build', 'tooling', 'lint',
         'typecheck', 'frontend-build', 'frontend-tests', 'whitespace')


def check_packages(packages, events):
    expected = set(packages.splitlines())
    finished = [event['Package'] for event in events
                if event.get('Action') in ('pass', 'skip', 'fail') and 'Test' not in event]
    return bool(expected) and len(finished) == len(set(finished)) and set(finished) == expected


def main():
    steps = json.loads(os.environ['GATE_RESULTS'])
    results = {gate: steps.get(gate, {}).get('outcome', 'missing') for gate in GATES}
    root = Path(os.environ['RUNNER_TEMP'])
    try:
        events = [json.loads(line) for line in (root / 'go-results.jsonl').read_text().splitlines()]
        accounted = check_packages((root / 'go-packages.txt').read_text(), events)
    except (OSError, ValueError):
        accounted = False
    results['go-package-accounting'] = 'success' if accounted else 'failure'
    report = json.dumps(results, indent=2)
    (root / 'macos-gates.json').write_text(report + '\n')
    print(report)
    with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as stream:
        stream.write('### Native macOS correctness gates\n\n| Gate | Outcome |\n| --- | --- |\n')
        for gate, outcome in results.items():
            stream.write(f'| {gate} | {outcome} |\n')
    return 0 if all(value == 'success' for value in results.values()) else 1


if __name__ == '__main__':
    raise SystemExit(main())
