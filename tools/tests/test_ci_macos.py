"""Native trial safety, resource bounds and full-package accounting."""
import importlib.util
from pathlib import Path
import unittest

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

    def test_trial_does_not_accept_pr_events_or_drop_correctness_gates(self):
        workflow = (ROOT / '.github/workflows/check.yml').read_text()
        self.assertNotIn('pull_request:', workflow)
        self.assertNotIn('pull_request_target:', workflow)
        self.assertIn('branches: [main, ci/macos-arm64-trial]', workflow)
        self.assertIn("github.repository == 'MnemosyneLab/NestworthGo'", workflow)
        self.assertIn('runs-on: [self-hosted, macOS, ARM64]', workflow)
        self.assertNotIn('container:', workflow)
        self.assertNotIn('sudo ', workflow)
        self.assertIn('persist-credentials: false', workflow)
        self.assertEqual(workflow.count('node scripts/ensure-bindings.mjs'), 1)
        self.assertIn('go test -count=1 -json -timeout 20m -p "$NESTWORTH_GO_PARALLEL" ./...', workflow)
        self.assertIn('pnpm exec vitest run --maxWorkers=', workflow)
        self.assertNotIn('--shard=', workflow)
        for gate in load('ci-macos-results').GATES:
            self.assertIn('id: ' + gate + '\n', workflow)
        self.assertIn('if: always()\n        env:\n          GATE_RESULTS:', workflow)
