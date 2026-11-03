#!/usr/bin/env python3
"""Run independent lanes with fixed budgets, distinct logs and fail-closed results.

Bindings and Vite finish before this scheduler: Go embed reads never race a
frontend writer. Only lint/typecheck overlap within the frontend lane on eligible
hosts; both finish before Vitest. Tooling fixtures use temporary copies.
"""
from concurrent.futures import ThreadPoolExecutor
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
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


def darwin_group_exited(pgid):
    """Confirm an EPERM group has no live members; never infer this from EPERM.

    XNU killpg1 filters SZOMB before counting signalable members, so a group
    containing only zombies returns EPERM. Query only the owned process group,
    without command lines or a host-wide process inventory. Inspection failures
    and any live/unrecognized member leave the original permission error intact.
    """
    try:
        snapshot = subprocess.run(
            ['/bin/ps', '-g', str(pgid), '-o', 'pid=,pgid=,stat='],
            capture_output=True, text=True, timeout=1,
            env=dict(os.environ, COMMAND_MODE='unix2003'))
    except (OSError, subprocess.TimeoutExpired):
        return False
    if snapshot.returncode not in (0, 1) or snapshot.stderr.strip():
        return False
    rows = snapshot.stdout.splitlines()
    if not rows:
        # The group can disappear between killpg and ps (ps exits 1 if empty).
        return True
    if snapshot.returncode != 0:
        return False
    for row in rows:
        fields = row.split()
        if (len(fields) != 3 or not fields[0].isdigit() or int(fields[0]) <= 0
                or fields[1] != str(pgid) or not fields[2].startswith('Z')):
            return False
    return True


def signal_owned_group(pgid, signum):
    try:
        os.killpg(pgid, signum)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        if sys.platform == 'darwin' and darwin_group_exited(pgid):
            return False
        raise


def terminate_group(process, grace):
    """Forward cancellation before escalation so nested owners can reap sessions."""
    if not signal_owned_group(process.pid, signal.SIGTERM):
        process.wait()
        return
    deadline = time.monotonic() + grace
    remaining = True
    while time.monotonic() < deadline:
        process.poll()
        if not signal_owned_group(process.pid, 0):
            remaining = False
            break
        time.sleep(.02)
    if remaining:
        signal_owned_group(process.pid, signal.SIGKILL)
    process.wait()


def run_lanes(lanes, root, output, workers):
    output.mkdir(parents=True, exist_ok=True)
    stopped = threading.Event()
    lock = threading.Lock()
    # Nested schedulers inherit a strictly shorter cleanup budget. Fixtures
    # forward TERM to their separately created sessions and finish before their
    # enclosing owner escalates. No host process discovery or broad pkill.
    grace = min(4.0, float(os.environ.get('NESTWORTH_CI_CANCEL_GRACE', '4')))
    child_env = dict(os.environ, NESTWORTH_CI_CANCEL_GRACE=str(grace / 2))
    results = {}

    def stop(signum, frame):
        # Only mark cancellation in the signal handler. Each owning worker
        # performs bounded cleanup; repeated signals cannot interrupt cleanup.
        stopped.set()

    previous = {sig: signal.signal(sig, stop) for sig in (signal.SIGTERM, signal.SIGINT)}

    def run_gate(gate):
        name, command, cwd = gate
        if stopped.is_set():
            return
        start = time.monotonic()
        print(f'START {name}', flush=True)
        with (output / (name + '.log')).open('w') as log:
            with lock:
                if stopped.is_set():
                    return
                process = subprocess.Popen(['bash', '-e', '-o', 'pipefail', '-c', command],
                                           cwd=root / cwd, stdout=log, stderr=subprocess.STDOUT,
                                           start_new_session=True, env=child_env)
            while process.poll() is None and not stopped.wait(.05):
                pass
            if stopped.is_set():
                terminate_group(process, grace)
            code = process.wait()
        result = dict(outcome='cancelled' if stopped.is_set() else ('success' if code == 0 else 'failure'),
                      exit_code=code, seconds=round(time.monotonic() - start, 3))
        with lock:
            results[name] = result
            (output / 'results.json').write_text(json.dumps(results, indent=2) + '\n')
        print(f'END {name}: {result}', flush=True)

    def lane(gates):
        index = 0
        while index < len(gates) and not stopped.is_set():
            pair = gates[index:index + 2]
            if workers > 1 and tuple(gate[0] for gate in pair) == ('lint', 'typecheck'):
                # Reuse the same process ownership/cancellation/results logic.
                # Do not nest run_lanes: signal handlers belong to the main thread.
                with ThreadPoolExecutor(max_workers=2) as checks:
                    pending = [checks.submit(run_gate, gate) for gate in pair]
                    try:
                        for future in pending:
                            future.result()
                    except BaseException:
                        stopped.set()
                        raise
                # Failure outcomes are retained; still run the complete tests.
                index += 2
            else:
                run_gate(gates[index])
                index += 1

    try:
        with ThreadPoolExecutor(max_workers=workers) as pool:
            futures = [pool.submit(lane, gates) for gates in lanes]
            try:
                for future in futures:
                    future.result()
            except BaseException:
                stopped.set()
                raise
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
