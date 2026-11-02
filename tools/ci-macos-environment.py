#!/usr/bin/env python3
"""Fail closed on native prerequisites; keep caches outside checkout cleanup."""
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess


def output(*args):
    return subprocess.check_output(args, text=True).strip()


def limits(cpus, memory):
    # No simultaneous frontend/Go lanes. Leave CPU and RAM for the desktop.
    slots = max(1, min(4, cpus // 2, memory // (3 * 1024**3)))
    return slots, min(2, slots)


def lane_budget(cpus, memory):
    if cpus >= 8 and memory >= 16 * 1024**3:
        # Bounded experiment: 2 Go package processes * 2 Go threads, 3 jsdom
        # workers and one tooling lane: eight worker CPU slots. No reserved
        # CPU slot on the measured eight-CPU host; observe contention/RAM.
        # These are concurrency bounds, not OS quotas or measured peak RSS.
        return dict(lanes=3, go_parallel=2, gomaxprocs=2, frontend_workers=3)
    slots, workers = limits(cpus, memory)
    return dict(lanes=1, go_parallel=slots, gomaxprocs=slots, frontend_workers=workers)


def main():
    if platform.system() != 'Darwin' or platform.machine() != 'arm64':
        raise SystemExit('This job requires native macOS ARM64')
    if os.geteuid() == 0:
        raise SystemExit('Do not run the personal runner as root')
    versions = json.loads(output('python3', 'tools/ci-toolchain-versions.py', '--json'))
    sdk = output('xcrun', '--sdk', 'macosx', '--show-sdk-path')
    sdk_version = output('xcrun', '--sdk', 'macosx', '--show-sdk-version')
    os_version = output('sw_vers', '-productVersion')
    if int(sdk_version.split('.')[0]) < 27 or int(os_version.split('.')[0]) < 27:
        raise SystemExit('Native CI needs macOS 27+ and a macOS 27+ SDK; no system changes attempted')
    go = output('go', 'env', 'GOVERSION')
    node = output('node', '--version')
    pnpm = output('pnpm', '--version')
    if (go, node, pnpm) != ('go' + versions['go'], 'v24.22.0', '12.14.0'):
        raise SystemExit('Selected Go/Node/pnpm versions do not match repository pins')
    cpus = int(output('sysctl', '-n', 'hw.logicalcpu'))
    memory = int(output('sysctl', '-n', 'hw.memsize'))
    budget = lane_budget(cpus, memory)
    slots, workers = budget['go_parallel'], budget['frontend_workers']
    evidence = dict(os=os_version, arch=platform.machine(), sdk=sdk,
                    sdk_version=sdk_version, clang=output('xcrun', 'clang', '--version'),
                    go=go, node=node, pnpm=pnpm, python=platform.python_version(),
                    wails=versions['wails'], cpus=cpus, memory_bytes=memory,
                    go_parallel=slots, frontend_workers=workers, lane_budget=budget,
                    deployment_target=os.environ['MACOSX_DEPLOYMENT_TARGET'])
    # SDK/compiler/OS/Go/flags/locks fence all native compiled cache content.
    fence = dict(evidence)
    for key in ('cpus', 'memory_bytes', 'go_parallel', 'frontend_workers', 'lane_budget'):
        fence.pop(key)
    fence['flags'] = {key: os.environ.get(key, '') for key in ('CGO_CFLAGS', 'CGO_CXXFLAGS', 'CGO_LDFLAGS')}
    fence['locks'] = [Path(p).read_text() for p in ('go.mod', 'go.sum')]
    key = hashlib.sha256(json.dumps(fence, sort_keys=True).encode()).hexdigest()[:24]
    # RUNNER_TEMP is a disposable sibling of the dedicated persistent cache root.
    cache = Path(os.environ['RUNNER_TEMP']).resolve().parent / 'nestworth-ci-cache-v1'
    native = cache / 'darwin-arm64' / key
    env = dict(SDKROOT=sdk, GOMODCACHE=str(cache / 'go-modules'),
               GOCACHE=str(native / 'go-build'), GOBIN=str(native / 'bin'),
               NESTWORTH_WAILS_GOCACHE=str(native / 'go-build'),
               NESTWORTH_PNPM_STORE=str(cache / 'pnpm-darwin-arm64-node24-pnpm12'),
               NESTWORTH_GO_PARALLEL=str(slots), GOMAXPROCS=str(budget['gomaxprocs']),
               NESTWORTH_GATE_LANES=str(budget['lanes']),
               GOFLAGS='-p=' + str(slots), NESTWORTH_FRONTEND_WORKERS=str(workers))
    for key in ('GOMODCACHE', 'GOCACHE', 'GOBIN', 'NESTWORTH_PNPM_STORE'):
        Path(env[key]).mkdir(parents=True, exist_ok=True)
    isolated = Path(os.environ['RUNNER_TEMP']) / 'nestworth-synthetic'
    isolated.mkdir(exist_ok=True)
    env.update(NESTWORTH_DATABASE_PATH=str(isolated / 'ledger.db'),
               NESTWORTH_SETTINGS_PATH=str(isolated / 'settings.json'))
    with open(os.environ['GITHUB_ENV'], 'a') as stream:
        for key, value in env.items():
            stream.write(f'{key}={value}\n')
    with open(os.environ['GITHUB_PATH'], 'a') as stream:
        stream.write(env['GOBIN'] + '\n')
    evidence['cache_root'] = str(native)
    evidence['warm_build_cache'] = any(Path(env['GOCACHE']).iterdir())
    evidence['warm_wails_cli'] = (Path(env['GOBIN']) / 'wails3').is_file()
    report = json.dumps(evidence, indent=2)
    print(report)
    (Path(os.environ['RUNNER_TEMP']) / 'macos-environment.json').write_text(report + '\n')


if __name__ == '__main__':
    main()
