"""Native trial safety, resource bounds and full-package accounting."""
import importlib.util
import json
from pathlib import Path
import unittest
import tempfile
import time
import subprocess
import os
import sys
import signal
import shlex
from unittest.mock import Mock, patch

ROOT = Path(__file__).resolve().parents[2]


def load(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / 'tools' / (name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class MacCI(unittest.TestCase):
    def test_darwin_eperm_requires_verified_zombie_only_owned_group(self):
        runner = load('ci-macos-gates')
        pgid = 43210
        for signum in (signal.SIGTERM, 0, signal.SIGKILL):
            for output, code, error, exited in (
                ('43210 43210 Z\n43211 43210 Z+\n', 0, '', True),
                ('', 1, '', True),
                ('43210 43210 S\n', 0, '', False),
                ('43210 43210 Z\n43211 43210 R\n', 0, '', False),
                ('43210 99 Z\n', 0, '', False),
                ('malformed\n', 0, '', False),
                ('', 1, 'permission denied', False),
                ('43210 43210 Z\n', 1, '', False),
                ('', 2, '', False),
            ):
                with self.subTest(signal=signum, output=output, code=code, error=error):
                    denied = PermissionError(1, 'Operation not permitted')
                    snapshot = subprocess.CompletedProcess([], code, output, error)
                    with patch.object(runner.sys, 'platform', 'darwin'), \
                            patch.object(runner.os, 'killpg', side_effect=denied) as send, \
                            patch.object(runner.subprocess, 'run', return_value=snapshot) as inspect:
                        if exited:
                            self.assertFalse(runner.signal_owned_group(pgid, signum))
                        else:
                            with self.assertRaises(PermissionError) as raised:
                                runner.signal_owned_group(pgid, signum)
                            self.assertIs(raised.exception, denied)
                        send.assert_called_once_with(pgid, signum)
                        self.assertEqual(inspect.call_args.args[0],
                                         ['/bin/ps', '-g', str(pgid), '-o', 'pid=,pgid=,stat='])
                        self.assertEqual(inspect.call_args.kwargs['timeout'], 1)
                        self.assertEqual(inspect.call_args.kwargs['env']['COMMAND_MODE'], 'unix2003')

    def test_unverifiable_or_non_darwin_permission_errors_remain_failures(self):
        runner = load('ci-macos-gates')
        for platform, inspection_error in (
                ('linux', None), ('darwin', OSError('ps unavailable')),
                ('darwin', subprocess.TimeoutExpired('ps', 1))):
            with self.subTest(platform=platform, error=inspection_error):
                process = Mock(pid=43210)
                with patch.object(runner.sys, 'platform', platform), \
                        patch.object(runner.os, 'killpg', side_effect=PermissionError(1, 'denied')), \
                        patch.object(runner.subprocess, 'run', side_effect=inspection_error) as inspect:
                    with self.assertRaises(PermissionError):
                        runner.terminate_group(process, .1)
                    process.wait.assert_not_called()
                    if platform != 'darwin':
                        inspect.assert_not_called()

    def test_real_zombie_group_probe_and_cleanup(self):
        runner = load('ci-macos-gates')
        process = subprocess.Popen([sys.executable, '-c', 'pass'], start_new_session=True)
        try:
            # Observe exit without wait/poll, deliberately retaining our own
            # unreaped child. XNU skips SZOMB in killpg1; Linux counts it.
            deadline = time.monotonic() + 5
            while True:
                state = subprocess.run(['/bin/ps', '-p', str(process.pid), '-o', 'stat='],
                                       capture_output=True, text=True, check=True).stdout.strip()
                if state.startswith('Z'):
                    break
                self.assertLess(time.monotonic(), deadline)
                time.sleep(.01)
            probe_errno = 0
            try:
                os.killpg(process.pid, 0)
            except PermissionError as error:
                probe_errno = error.errno
            self.assertEqual(probe_errno, 1 if sys.platform == 'darwin' else 0)
            print(f'zombie-group platform={sys.platform} state={state} probe_errno={probe_errno}', flush=True)
            runner.terminate_group(process, .1)
            self.assertEqual(process.returncode, 0)
            with self.assertRaises(ProcessLookupError):
                os.kill(process.pid, 0)
        finally:
            if process.poll() is None:
                process.kill()
            process.wait()

    def test_live_descendant_is_killed_after_group_leader_exits(self):
        runner = load('ci-macos-gates')
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            child = root / 'child.py'
            child.write_text("import os, signal, time\nfrom pathlib import Path\n"
                             "signal.signal(signal.SIGTERM, signal.SIG_IGN)\n"
                             # Publish readiness only after the PID is written and closed.
                             "Path('child.pid.tmp').write_text(str(os.getpid()))\n"
                             "Path('child.pid.tmp').replace('child.pid')\ntime.sleep(60)\n")
            leader = ("import subprocess, sys, time\n"
                      f"subprocess.Popen([sys.executable, {str(child)!r}])\ntime.sleep(60)\n")
            process = subprocess.Popen([sys.executable, '-c', leader], cwd=root, start_new_session=True)
            unrelated = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(60)'],
                                         start_new_session=True)
            try:
                deadline = time.monotonic() + 5
                while not (root / 'child.pid').exists():
                    self.assertLess(time.monotonic(), deadline)
                    time.sleep(.01)
                pid = int((root / 'child.pid').read_text())
                runner.terminate_group(process, .15)
                self.assertIsNotNone(process.returncode)
                # The orphan may remain a zombie until init reaps it. It must
                # have stopped executing even though its leader already exited.
                deadline = time.monotonic() + 2
                while True:
                    state = subprocess.run(['/bin/ps', '-p', str(pid), '-o', 'stat='],
                                           capture_output=True, text=True).stdout.strip()
                    if not state or state.startswith('Z'):
                        break
                    self.assertLess(time.monotonic(), deadline, state)
                    time.sleep(.01)
                self.assertIsNone(unrelated.poll())
            finally:
                runner.signal_owned_group(process.pid, signal.SIGKILL)
                process.wait()
                unrelated.terminate()
                unrelated.wait(timeout=5)

    def test_memory_and_cpu_bound_workers(self):
        limits = load('ci-macos-environment').limits
        self.assertEqual(limits(1, 2 * 1024**3), (1, 1))
        self.assertEqual(limits(10, 8 * 1024**3), (2, 2))
        self.assertEqual(limits(16, 24 * 1024**3), (4, 2))

    def test_accounting_rejects_missing_duplicate_and_unexpected_packages(self):
        check = load('ci-macos-results').check_packages
        events = [{'Package': 'a', 'Action': 'pass'}, {'Package': 'b', 'Action': 'skip'}]
        self.assertTrue(check('a\nb\n', events))
        self.assertFalse(check('a\nb\nc\n', events))
        self.assertFalse(check('a\n', events))
        self.assertFalse(check('a\nb\n', events + events[:1]))
        self.assertFalse(check('', []))

    def test_origin_pushes_cover_internal_pr_heads_without_accepting_fork_events(self):
        workflow = (ROOT / '.github/workflows/check.yml').read_text()
        self.assertNotIn('pull_request:', workflow)
        self.assertNotIn('pull_request_target:', workflow)
        self.assertIn("branches: ['**']", workflow)
        self.assertIn('    name: test\n', workflow)
        self.assertIn("github.repository == 'MnemosyneLab/NestworthGo'", workflow)
        self.assertIn('runs-on: [self-hosted, macOS, ARM64]', workflow)
        self.assertNotIn('container:', workflow)
        self.assertNotIn('sudo ', workflow)
        self.assertIn('persist-credentials: false', workflow)
        self.assertEqual(workflow.count('node scripts/ensure-bindings.mjs'), 1)
        scheduler = (ROOT / 'tools/ci-macos-gates.py').read_text()
        self.assertIn('go test -count=1 -json -timeout 20m -p "$NESTWORTH_GO_PARALLEL" ./...', scheduler)
        self.assertIn('pnpm exec vitest run --maxWorkers=', scheduler)
        self.assertNotIn('--shard=', workflow)
        selected = [name for lane in load('ci-macos-gates').LANES for name, _, _ in lane]
        self.assertEqual(len(selected), len(set(selected)))
        self.assertEqual(set(selected) | {'bindings', 'frontend-build'}, set(load('ci-macos-results').GATES))
        self.assertLess(workflow.index('id: frontend-build'), workflow.index('id: lanes'))
        self.assertIn('if: always()\n        env:\n          GATE_RESULTS:', workflow)

    def test_linux_rollback_is_nonrunning_source_only(self):
        self.assertFalse((ROOT / '.github/workflows/check-linux.yml').exists())
        archive = ROOT / '.github/workflow-archive/check-linux.yml.disabled'
        self.assertTrue(archive.is_file())
        self.assertNotEqual(archive.parent, ROOT / '.github/workflows')
        self.assertIn('name: test', archive.read_text())
        active = {p.name for p in (ROOT / '.github/workflows').iterdir()
                  if p.suffix in {'.yml', '.yaml'}}
        self.assertEqual(active, {'check.yml', 'perf.yml', 'ci-native-image.yml',
                                  'eino-provider.yml'})
        mac = (ROOT / '.github/workflows/check.yml').read_text()
        self.assertIn('    name: test\n', mac)
        self.assertNotIn('pull_request:', mac)
        self.assertNotIn('pull_request_target:', mac)
        self.assertIn("branches: ['**']", mac)

    def test_eino_provider_requires_trusted_origin_and_hosted_linux_runner(self):
        workflow = (ROOT / '.github/workflows/eino-provider.yml').read_text()
        self.assertIn("on:\n  push:\n    branches: ['**']\n    paths:\n", workflow)
        self.assertIn('  workflow_dispatch:\n', workflow)
        for event in ('pull_request', 'pull_request_target', 'workflow_run',
                      'repository_dispatch', 'schedule'):
            self.assertNotIn(event + ':', workflow)
        guard = ("    if: github.repository == 'MnemosyneLab/NestworthGo' && "
                 "(github.event_name == 'push' || "
                 "github.event_name == 'workflow_dispatch')\n")
        self.assertIn(guard, workflow)
        self.assertEqual(workflow.count('runs-on:'), 1)
        self.assertIn('    runs-on: ubuntu-24.04\n', workflow)
        for forbidden in ('self-hosted', 'macOS', 'macos-', 'ARM64'):
            self.assertNotIn(forbidden, workflow)
        self.assertIn('permissions:\n  contents: read\n', workflow)
        self.assertNotIn('contents: write', workflow)
        self.assertNotIn('container:', workflow)
        self.assertNotIn('sudo ', workflow)
        self.assertIn('persist-credentials: false', workflow)
        self.assertIn('working-directory: providers/einoopenai\n', workflow)
        self.assertIn('go-version-file: providers/einoopenai/go.mod\n', workflow)
        self.assertIn('          cache: false\n', workflow)
        self.assertIn('    timeout-minutes: 10\n', workflow)
        for path in ('providers/einoopenai/**', 'internal/**',
                     'go.mod', 'go.sum', '.github/workflows/eino-provider.yml'):
            self.assertIn("      - '" + path + "'\n", workflow)
        self.assertIn('run: ./check.sh | tee ', workflow)
        checks = (ROOT / 'providers/einoopenai/check.sh').read_text()
        for command in ('set -euo pipefail', 'export GOWORK=off',
                        'test -z "$(gofmt -l .)"',
                        'go test -count=1 -timeout=120s ./...',
                        'go vet ./...', 'go build ./...'):
            self.assertIn(command, checks)
        self.assertNotIn('-race', checks)

    def test_increased_frontend_and_go_parallelism_preserves_smaller_host_limits(self):
        budget = load('ci-macos-environment').lane_budget
        high = budget(8, 16 * 1024**3)
        self.assertEqual(high['lanes'], 3)
        self.assertEqual(high, dict(lanes=3, go_parallel=3, gomaxprocs=2, frontend_workers=4))
        self.assertEqual(high['go_parallel'] * high['gomaxprocs'] + high['frontend_workers'] + 1, 11)
        self.assertEqual(budget(8, 8 * 1024**3)['frontend_workers'], 2)
        self.assertEqual(budget(4, 32 * 1024**3)['frontend_workers'], 2)
        self.assertEqual(budget(8, 8 * 1024**3)['lanes'], 1)
        self.assertEqual(budget(4, 32 * 1024**3)['lanes'], 1)

    def test_lane_failures_do_not_suppress_other_gates_and_logs_are_separate(self):
        runner = load('ci-macos-gates')
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            lanes = ((('fails', 'echo expected-failure; exit 9', '.'),
                      ('after', 'echo still-runs', '.')),
                     (('independent', 'echo other-lane', '.'),))
            results = runner.run_lanes(lanes, root, root / 'logs', 2)
            self.assertEqual(results['fails']['exit_code'], 9)
            self.assertEqual(results['after']['outcome'], 'success')
            self.assertEqual(results['independent']['outcome'], 'success')
            self.assertEqual((root / 'logs/after.log').read_text(), 'still-runs\n')
            self.assertNotIn('other-lane', (root / 'logs/fails.log').read_text())

    def test_cancellation_kills_child_group_and_stops_later_gates(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            script = f"""import importlib.util
import json
from pathlib import Path
s = importlib.util.spec_from_file_location('gates', {str(ROOT / 'tools/ci-macos-gates.py')!r})
m = importlib.util.module_from_spec(s); s.loader.exec_module(m)
r = Path({directory!r})
m.run_lanes(((('hang', 'echo $$ > child.tmp && mv child.tmp child && sleep 60', '.'), ('after', 'touch after', '.')),), r, r / 'logs', 1)
"""
            process = subprocess.Popen([sys.executable, '-c', script], stdout=subprocess.DEVNULL)
            try:
                deadline = time.monotonic() + 5
                while not (root / 'child').exists() and time.monotonic() < deadline:
                    time.sleep(.02)
                self.assertTrue((root / 'child').exists())
                child = int((root / 'child').read_text())
                process.send_signal(signal.SIGTERM)
                process.wait(timeout=5)
                self.assertEqual(process.returncode, 0)
                self.assertFalse((root / 'after').exists())
                with self.assertRaises(ProcessLookupError):
                    os.kill(child, 0)
            finally:
                if process.poll() is None:
                    process.kill()
                    process.wait()

    def test_pnpm_warm_bootstrap_avoids_npm(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            binaries = root / 'bin'; binaries.mkdir()
            (root / '_temp').mkdir()
            node = binaries / 'node'
            node.write_text('#!/bin/sh\necho v24.22.0\n'); node.chmod(0o755)
            npm = binaries / 'npm'
            npm.write_text('#!/bin/bash\necho called >> "$CALLS"\n'
                           'prefix="$3"\nmkdir -p "$prefix/node_modules/.bin"\n'
                           'printf "#!/bin/sh\\necho 12.14.0\\n" > "$prefix/node_modules/.bin/pnpm"\n'
                           'chmod +x "$prefix/node_modules/.bin/pnpm"\n')
            npm.chmod(0o755)
            env = dict(os.environ, PATH=str(binaries) + ':' + os.environ['PATH'],
                       RUNNER_TEMP=str(root / '_temp'), GITHUB_PATH=str(root / 'path'), CALLS=str(root / 'calls'))
            for _ in range(2):
                subprocess.run(['bash', str(ROOT / 'tools/ci-macos-pnpm.sh')], env=env, check=True, capture_output=True)
            self.assertEqual((root / 'calls').read_text().splitlines(), ['called'])

    def test_aggregate_rejects_missing_and_failed_lane_results(self):
        gates = load('ci-macos-results').GATES
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'macos-gate-logs').mkdir()
            (root / 'go-packages.txt').write_text('example/package\n')
            (root / 'go-results.jsonl').write_text(json.dumps({'Package': 'example/package', 'Action': 'pass'}) + '\n')
            steps = {gate: {'outcome': 'success'} for gate in ('bindings', 'frontend-build', 'lanes')}
            env = dict(os.environ, RUNNER_TEMP=directory, GATE_RESULTS=json.dumps(steps), GITHUB_STEP_SUMMARY=str(root / 'summary'))
            for variant in ('success', 'missing', 'failure'):
                results = {gate: {'outcome': 'success', 'seconds': 1} for gate in gates}
                if variant == 'missing':
                    del results['lint']
                elif variant == 'failure':
                    results['lint']['outcome'] = 'failure'
                (root / 'macos-gate-logs/results.json').write_text(json.dumps(results))
                result = subprocess.run([sys.executable, str(ROOT / 'tools/ci-macos-results.py')], env=env, capture_output=True)
                self.assertEqual(result.returncode, 0 if variant == 'success' else 1)

    def test_cancellation_reaps_nested_scheduler_and_portable_timeout_session(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            child = root / 'detached.py'
            child.write_text("import os, signal, time\nfrom pathlib import Path\n"
                             "signal.signal(signal.SIGTERM, signal.SIG_IGN)\n"
                             # exists() must mean the complete PID is ready to read.
                             "Path('detached.pid.tmp').write_text(str(os.getpid()))\n"
                             "Path('detached.pid.tmp').replace('detached.pid')\ntime.sleep(60)\n")
            leaf = ' '.join(map(shlex.quote, [sys.executable,
                str(ROOT / 'tools/tests/fixtures/portable_timeout.py'), '-k', '1s', '60s',
                sys.executable, str(child)]))
            def scheduler(gates, logs):
                return ("import importlib.util\nfrom pathlib import Path\n"
                        f"s=importlib.util.spec_from_file_location('gates', {str(ROOT / 'tools/ci-macos-gates.py')!r})\n"
                        "m=importlib.util.module_from_spec(s); s.loader.exec_module(m)\n"
                        f"m.run_lanes({gates!r}, Path({directory!r}), Path({str(root / logs)!r}), 1)\n")
            inner = root / 'inner.py'
            inner.write_text(scheduler(((('leaf', leaf, '.'),),), 'inner-logs'))
            outer = root / 'outer.py'
            command = shlex.quote(sys.executable) + ' ' + shlex.quote(str(inner))
            outer.write_text(scheduler(((('nested', command, '.'), ('after', 'touch after', '.')),), 'outer-logs'))
            unrelated = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(60)'], start_new_session=True)
            process = subprocess.Popen([sys.executable, str(outer)], stdout=subprocess.DEVNULL, start_new_session=True)
            detached = None
            try:
                deadline = time.monotonic() + 5
                while not (root / 'detached.pid').exists() and time.monotonic() < deadline:
                    time.sleep(.02)
                self.assertTrue((root / 'detached.pid').exists())
                detached = int((root / 'detached.pid').read_text())
                self.assertNotEqual(os.getsid(detached), os.getsid(process.pid))
                process.send_signal(signal.SIGTERM)
                process.wait(timeout=8)
                self.assertEqual(process.returncode, 0)
                self.assertFalse((root / 'after').exists())
                with self.assertRaises(ProcessLookupError):
                    os.kill(detached, 0)
                self.assertIsNone(unrelated.poll(), 'unrelated process must remain alive')
                outcomes = json.loads((root / 'outer-logs/results.json').read_text())
                self.assertEqual(outcomes['nested']['outcome'], 'cancelled')
            finally:
                if process.poll() is None:
                    process.kill()
                    process.wait()
                if detached is not None:
                    try:
                        os.kill(detached, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                unrelated.terminate()
                unrelated.wait(timeout=5)

    def test_lint_and_typecheck_overlap_and_join_before_tests_even_on_failure(self):
        runner = load('ci-macos-gates')
        for lint_status in (0, 9):
            with self.subTest(lint_status=lint_status), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                def command(name, other, status):
                    code = ("from pathlib import Path; import time\n"
                            f"Path('{name}.ready').touch()\n"
                            "deadline = time.monotonic() + 5\n"
                            f"while not Path('{other}.ready').exists():\n"
                            " if time.monotonic() >= deadline: raise SystemExit(77)\n"
                            " time.sleep(.01)\n"
                            f"Path('{name}.done').touch()\nraise SystemExit({status})\n")
                    return shlex.quote(sys.executable) + ' -c ' + shlex.quote(code)
                lanes = ((('lint', command('lint', 'typecheck', lint_status), '.'),
                          ('typecheck', command('typecheck', 'lint', 0), '.'),
                          ('frontend-tests', 'test -f lint.done && test -f typecheck.done && echo complete', '.')),)
                results = runner.run_lanes(lanes, root, root / 'logs', 3)
                self.assertEqual(results['lint']['exit_code'], lint_status)
                self.assertEqual(results['typecheck']['outcome'], 'success')
                self.assertEqual(results['frontend-tests']['outcome'], 'success')
                self.assertEqual(set(results), {'lint', 'typecheck', 'frontend-tests'})

    def test_smaller_host_keeps_frontend_checks_serial(self):
        runner = load('ci-macos-gates')
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            lanes = ((('lint', 'touch lint.done', '.'),
                      ('typecheck', 'test -f lint.done && touch typecheck.done', '.'),
                      ('frontend-tests', 'test -f typecheck.done', '.')),)
            results = runner.run_lanes(lanes, root, root / 'logs', 1)
            self.assertTrue(all(result['outcome'] == 'success' for result in results.values()))

    def test_cancellation_owns_both_parallel_frontend_checks(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            # Redirecting directly to the readiness path exposes an empty file.
            lanes = ((('lint', 'echo $$ > lint.pid.tmp && mv lint.pid.tmp lint.pid && sleep 60', '.'),
                      ('typecheck', 'echo $$ > typecheck.pid.tmp && mv typecheck.pid.tmp typecheck.pid && sleep 60', '.'),
                      ('frontend-tests', 'touch tests-started', '.')),)
            script = ("import importlib.util\nfrom pathlib import Path\n"
                      f"s=importlib.util.spec_from_file_location('gates', {str(ROOT / 'tools/ci-macos-gates.py')!r})\n"
                      "m=importlib.util.module_from_spec(s); s.loader.exec_module(m)\n"
                      f"m.run_lanes({lanes!r}, Path({directory!r}), Path({str(root / 'logs')!r}), 3)\n")
            process = subprocess.Popen([sys.executable, '-c', script], stdout=subprocess.DEVNULL)
            children = []
            try:
                deadline = time.monotonic() + 5
                while not all((root / f'{name}.pid').exists() for name in ('lint', 'typecheck')):
                    self.assertLess(time.monotonic(), deadline)
                    time.sleep(.02)
                children = [int((root / f'{name}.pid').read_text()) for name in ('lint', 'typecheck')]
                process.send_signal(signal.SIGTERM)
                process.wait(timeout=8)
                self.assertEqual(process.returncode, 0)
                self.assertFalse((root / 'tests-started').exists())
                for child in children:
                    with self.assertRaises(ProcessLookupError):
                        os.kill(child, 0)
                outcomes = json.loads((root / 'logs/results.json').read_text())
                self.assertEqual({name: row['outcome'] for name, row in outcomes.items()},
                                 {'lint': 'cancelled', 'typecheck': 'cancelled'})
            finally:
                if process.poll() is None:
                    process.kill()
                    process.wait()
                for child in children:
                    try:
                        load('ci-macos-gates').signal_owned_group(child, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
