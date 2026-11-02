#!/usr/bin/env python3
"""Run independent lanes with fixed budgets, distinct logs and fail-closed results.

Bindings and Vite finish before this scheduler: Go embed reads never race a
frontend writer. Each lane is sequential. Tooling fixtures use temporary copies.
"""
from concurrent.futures import ThreadPoolExecutor
import json
import os
from pathlib import Path
import signal
import subprocess
import threading
import time

ROOT = Path(__file__).resolve().parents[1]
LANES = (
    (
        ('format', 'test -z "$(gofmt -l cmd internal)"', '.'),
        ('go-tests', 'go list ./... | tee "$RUNNER_TEMP/go-packages.txt"\n'
         'go test -count=1 -json -timeout 20m -p "$NESTWORTH_GO_PARALLEL" ./... | tee "$RUNNER_TEMP/go-results.jsonl"', '.'),
        ('vet', 'go vet -p "$NESTWORTH_GO_PARALLEL" ./...', '.'),
        ('build', 'go build -p "$NESTWORTH_GO_PARALLEL" -o "$RUNNER_TEMP/nestworth-ci" ./cmd/nestworth', '.'),
    ),
    (
        ('tooling', 'python3 tools/check-nestworth-skill.py\npython3 tools/run-tooling-tests.py --tier fast', '.'),
        ('whitespace', 'git diff --check', '.'),
    ),
    (
        ('lint', 'pnpm exec eslint .', 'frontend'),
        ('typecheck', 'pnpm exec tsc --noEmit', 'frontend'),
        ('frontend-tests', 'pnpm exec vitest run --maxWorkers="$NESTWORTH_FRONTEND_WORKERS" --reporter=default --reporter=json --outputFile.json="$RUNNER_TEMP/frontend-results.json"', 'frontend'),
    ),
)


def run_lanes(lanes, root, output, workers):
    output.mkdir(parents=True, exist_ok=True)
    stopped = threading.Event()
    lock = threading.Lock()
    active = set()
    results = {}

    def stop(signum, frame):
        stopped.set()
        with lock:
            for pid in active:
                try:
                    os.killpg(pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass

    previous = {sig: signal.signal(sig, stop) for sig in (signal.SIGTERM, signal.SIGINT)}

    def lane(gates):
        for name, command, cwd in gates:
            if stopped.is_set():
                break
            start = time.monotonic()
            print(f'START {name}', flush=True)
            with (output / (name + '.log')).open('w') as log:
                with lock:
                    if stopped.is_set():
                        break
                    process = subprocess.Popen(['bash', '-e', '-o', 'pipefail', '-c', command],
                                               cwd=root / cwd, stdout=log, stderr=subprocess.STDOUT,
                                               start_new_session=True)
                    active.add(process.pid)
                code = process.wait()
                with lock:
                    active.discard(process.pid)
            result = dict(outcome='success' if code == 0 else 'failure',
                          exit_code=code, seconds=round(time.monotonic() - start, 3))
            with lock:
                results[name] = result
                (output / 'results.json').write_text(json.dumps(results, indent=2) + '\n')
            print(f'END {name}: {result}', flush=True)

    try:
        with ThreadPoolExecutor(max_workers=workers) as pool:
            futures = [pool.submit(lane, gates) for gates in lanes]
            for future in futures:
                future.result()
    finally:
        stop(None, None)
        for sig, handler in previous.items():
            signal.signal(sig, handler)
    return results


def main():
    output = Path(os.environ['RUNNER_TEMP']) / 'macos-gate-logs'
    results = run_lanes(LANES, ROOT, output, int(os.environ['NESTWORTH_GATE_LANES']))
    # Print complete, non-interleaved logs after execution; artifacts also retain
    # individual files and seconds. No assertions are inferred from log text.
    for lane in LANES:
        for name, _, _ in lane:
            print(f'::group::{name} — {results.get(name, {"outcome": "missing"})}', flush=True)
            path = output / (name + '.log')
            if path.exists():
                print(path.read_text(errors='replace'), flush=True)
            print('::endgroup::', flush=True)
    return 0 if len(results) == sum(map(len, LANES)) and all(
        result['outcome'] == 'success' for result in results.values()) else 1


if __name__ == '__main__':
    raise SystemExit(main())
