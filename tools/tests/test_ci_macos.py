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

ROOT = Path(__file__).resolve().parents[2]


def load(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / 'tools' / (name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class MacCI(unittest.TestCase):
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
        active = {p.name for p in (ROOT / '.github/workflows').glob('*.yml')}
        self.assertEqual(active, {'check.yml', 'perf.yml', 'ci-native-image.yml'})
        mac = (ROOT / '.github/workflows/check.yml').read_text()
        self.assertIn('    name: test\n', mac)
        self.assertNotIn('pull_request:', mac)
        self.assertNotIn('pull_request_target:', mac)
        self.assertIn("branches: ['**']", mac)

    def test_three_worker_experiment_is_bounded_and_low_memory_serializes(self):
        budget = load('ci-macos-environment').lane_budget
        high = budget(8, 16 * 1024**3)
        self.assertEqual(high['lanes'], 3)
        self.assertEqual(high, dict(lanes=3, go_parallel=2, gomaxprocs=2, frontend_workers=3))
        self.assertLessEqual(high['go_parallel'] * high['gomaxprocs'] + high['frontend_workers'] + 1, 8)
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
m.run_lanes(((('hang', 'echo $$ > child; sleep 60', '.'), ('after', 'touch after', '.')),), r, r / 'logs', 1)
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
                             "Path('detached.pid').write_text(str(os.getpid()))\ntime.sleep(60)\n")
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
