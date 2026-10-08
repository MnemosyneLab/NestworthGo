"""Exercise CI apt failure paths with synthetic commands, never system packages."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "tools/ci-install-native-deps.sh"


class NativeDependencyTests(unittest.TestCase):
    def run_install(self, scenarios, tee_failure=False):
        with tempfile.TemporaryDirectory() as directory:
            work = Path(directory)
            commands = {
                "sudo": '#!/bin/bash\n[[ $1 == -n ]] || exit 91\nshift\nexec "$@"\n',
                "sleep": '#!/bin/bash\nexit 0\n',
                # Use real GNU timeout/signals, with short synthetic budgets.
                "timeout": '''#!/bin/bash
args=()
for arg in "$@"; do
  case "$arg" in
    120s|180s) args+=(0.1s);;
    10s) args+=(0.1s);;
    *) args+=("$arg");;
  esac
done
exec /usr/bin/timeout "${args[@]}"
''',
                "apt-get": '''#!/usr/bin/env python3
import json, os, pathlib, sys, time, signal
root = pathlib.Path(os.environ['MOCK_ROOT'])
args = sys.argv[1:]
phase = 'update' if 'update' in args else 'download' if '--download-only' in args else 'configure'
with (root / 'calls').open('a') as f:
    f.write(json.dumps({'phase': phase, 'args': args, 'frontend': os.environ.get('DEBIAN_FRONTEND')}) + '\\n')
counter = root / phase
n = int(counter.read_text()) if counter.exists() else 0
counter.write_text(str(n + 1))
items = json.loads(os.environ['SCENARIOS']).get(phase, [[0, 'ok']])
status, output = items[min(n, len(items) - 1)]
print(output, flush=True)
if status in ('hang', 'kill'):
    if status == 'kill': signal.signal(signal.SIGTERM, signal.SIG_IGN)
    time.sleep(30)
else:
    sys.exit(status)
''',
                "dpkg": '#!/bin/bash\necho synthetic-dpkg-audit\nexit 7\n',
                "tail": '#!/bin/bash\necho synthetic-apt-log\nexit 8\n',
                "ps": '#!/bin/bash\necho synthetic-process-list\nexit 9\n',
            }
            if tee_failure:
                commands['tee'] = '#!/bin/bash\ncat > "$1"\nexit 23\n'
            for name, content in commands.items():
                path = work / name
                path.write_text(content)
                path.chmod(0o755)
            env = dict(os.environ, PATH=f"{work}:{os.environ['PATH']}",
                       MOCK_ROOT=directory, SCENARIOS=json.dumps(scenarios))
            result = subprocess.run(['bash', str(SCRIPT)], env=env, cwd=ROOT,
                                    capture_output=True, text=True, timeout=5)
            calls = [json.loads(line) for line in (work / 'calls').read_text().splitlines()]
            return result, calls

    def test_success_and_acquisition_configuration_separation(self):
        result, calls = self.run_install({})
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertEqual([c['phase'] for c in calls], ['update', 'download', 'configure'])
        self.assertIn('APT::Update::Error-Mode=any', calls[0]['args'])
        for call in calls:
            self.assertEqual(call['frontend'], 'noninteractive')
            self.assertIn('Acquire::Retries=0', call['args'])
            self.assertIn('DPkg::Lock::Timeout=30', call['args'])
        for call in calls[1:]:
            for package in ('gcc', 'pkg-config', 'libgtk-4-dev', 'libwebkitgtk-6.0-dev', 'libsoup-3.0-dev'):
                self.assertIn(package, call['args'])

    def test_transient_update_and_download_retry_then_success(self):
        for phase in ('update', 'download'):
            with self.subTest(phase=phase):
                result, calls = self.run_install({phase: [[100, 'E: Failed to fetch https://example.invalid Temporary failure resolving'], [0, 'ok']]})
                self.assertEqual(result.returncode, 0, result.stdout)
                self.assertEqual(sum(c['phase'] == phase for c in calls), 2)
                self.assertIn('::warning::', result.stdout)

    def test_acquisition_timeout_and_forced_kill_retry_then_success(self):
        for phase in ('update', 'download'):
            for status in ('hang', 'kill'):
                with self.subTest(phase=phase, status=status):
                    result, calls = self.run_install({phase: [[status, 'synthetic stall'], [0, 'ok']]})
                    self.assertEqual(result.returncode, 0, result.stdout)
                    self.assertEqual(sum(c['phase'] == phase for c in calls), 2)
                    self.assertIn('exit=' + ('124' if status == 'hang' else '137'), result.stdout)

    def test_exhaustion_preserves_last_exit_and_stops(self):
        for status, output in ((100, 'E: Failed to fetch https://example.invalid 503 Service Unavailable'), ('hang', 'stall'), ('kill', 'stall')):
            with self.subTest(status=status):
                result, calls = self.run_install({'download': [[status, output]]})
                self.assertEqual(result.returncode, {'hang': 124, 'kill': 137}.get(status, status))
                self.assertEqual([c['phase'] for c in calls], ['update', 'download', 'download'])
                self.assertIn('synthetic-dpkg-audit', result.stdout)
                self.assertIn('::error::', result.stdout)

    def test_unknown_and_permanent_errors_never_retry(self):
        for status, output in (
            (42, 'arbitrary failure'),
            (100, 'E: Unable to locate package libgtk-4-dev'),
            (100, 'E: The repository is not signed.'),
            (100, 'E: Could not get lock /var/lib/dpkg/lock'),
            (100, 'E: Failed to fetch https://example.invalid 404 Not Found'),
            (100, 'E: Failed to fetch https://example.invalid Could not handshake: Certificate verification failed'),
            (100, 'E: Failed to fetch https://example.invalid Temporary failure resolving\nE: dpkg was interrupted'),
        ):
            with self.subTest(output=output):
                result, calls = self.run_install({'update': [[status, output]]})
                self.assertEqual(result.returncode, status)
                self.assertEqual(len(calls), 1)

    def test_mixed_fetch_errors_fail_without_retry(self):
        for phase in ('update', 'download'):
            for prefix in ('E:', 'W:'):
                for transient in ('Temporary failure resolving', '503 Service Unavailable'):
                    for permanent in ('404 Not Found', 'unknown acquisition error'):
                        for reverse in (False, True):
                            with self.subTest(phase=phase, prefix=prefix, transient=transient,
                                              permanent=permanent, reverse=reverse):
                                lines = [f'{prefix} Failed to fetch https://example.invalid/a {permanent}',
                                         f'{prefix} Failed to fetch https://example.invalid/b {transient}']
                                if reverse:
                                    lines.reverse()
                                lines.append('E: Unable to fetch some archives, maybe run apt-get update or try with --fix-missing?')
                                result, calls = self.run_install({phase: [[100, '\n'.join(lines)], [0, 'ok']]})
                                self.assertEqual(result.returncode, 100, result.stdout)
                                self.assertEqual(sum(c['phase'] == phase for c in calls), 1)
                                self.assertNotIn('configure', [c['phase'] for c in calls])
                                self.assertNotIn('::warning::', result.stdout)

    def test_multiple_transient_fetch_errors_retry_then_success(self):
        output = ('E: Failed to fetch https://example.invalid/a Temporary failure resolving\n'
                  'E: Failed to fetch https://example.invalid/b 503 Service Unavailable\n'
                  'E: Unable to fetch some archives, maybe run apt-get update or try with --fix-missing?')
        for phase in ('update', 'download'):
            with self.subTest(phase=phase):
                result, calls = self.run_install({phase: [[100, output], [0, 'ok']]})
                self.assertEqual(result.returncode, 0, result.stdout)
                self.assertEqual(sum(c['phase'] == phase for c in calls), 2)

    def test_configuration_never_retried_including_timeout_and_network_text(self):
        for status, output in ((100, 'E: Sub-process /usr/bin/dpkg returned an error code (1)'), (100, 'Temporary failure resolving'), ('hang', 'stall')):
            with self.subTest(status=status):
                result, calls = self.run_install({'configure': [[status, output]]})
                self.assertEqual(result.returncode, 124 if status == 'hang' else status)
                self.assertEqual([c['phase'] for c in calls], ['update', 'download', 'configure'])

    def test_tee_failure_visible_and_cannot_swallow_apt_failure(self):
        for status in (0, 42):
            result, calls = self.run_install({'update': [[status, 'synthetic']]}, tee_failure=True)
            self.assertEqual(result.returncode, status or 23)
            self.assertEqual(len(calls), 1)


if __name__ == '__main__':
    unittest.main()
