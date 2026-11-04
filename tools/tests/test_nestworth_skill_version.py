"""Version guard regressions use only a synthetic Git repository."""
import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('skill_check', ROOT / 'tools/check-nestworth-skill.py')
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)


class SkillVersionTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory(prefix='nestworth-version-test-')
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.skill = self.root / 'skills/nestworth'
        self.skill.mkdir(parents=True)
        for name, value in {
            'skills/nestworth/VERSION': '2.0.0\n',
            'skills/nestworth/SKILL.md': 'instructions',
            'skills/nestworth/references/analysis.md': 'analysis',
            'skills/nestworth/agents/openai.yaml': 'metadata',
            'tools/install-nestworth-skill.sh': 'installer',
            'docs/user/nestworth-skill.md': 'guide',
        }.items():
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(value)
        self.git('init', '-b', 'main')
        self.git('config', 'user.name', 'Synthetic Test')
        self.git('config', 'user.email', 'test@example.invalid')
        self.commit()
        self.base = self.git('rev-parse', 'HEAD')
        self.addCleanup(patch.stopall)
        patch.object(checker, 'ROOT', self.root).start()
        patch.object(checker, 'SKILL', self.skill).start()

    def git(self, *args):
        return subprocess.check_output(['git', *args], cwd=self.root, text=True, stderr=subprocess.PIPE).strip()

    def commit(self):
        self.git('add', '.')
        self.git('commit', '-qm', 'synthetic fixture')

    def check(self, base=None):
        with contextlib.redirect_stdout(io.StringIO()):
            checker.check_version(base or self.base)

    def test_stale_installation_guide_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "Installation guide"):
            checker.check()

    def test_each_distributed_input_requires_bump(self):
        for path in sorted(self.root.rglob('*')):
            if not path.is_file() or '.git' in path.parts or path.name == 'VERSION':
                continue
            with self.subTest(path=path.relative_to(self.root)):
                old = path.read_bytes()
                path.write_bytes(old + b' changed')
                with self.assertRaisesRegex(ValueError, 'bump'):
                    self.check()
                path.write_bytes(old)

    def test_committed_multi_commit_push_cannot_hide_missing_bump(self):
        (self.skill / 'references/analysis.md').write_text('new contract')
        self.commit()
        (self.root / 'unrelated.txt').write_text('app only')
        self.commit()
        with self.assertRaisesRegex(ValueError, 'bump'):
            self.check()

    def test_added_deleted_and_renamed_references_require_bump(self):
        path = self.skill / 'references/new.md'
        path.write_text('new guidance')
        self.git('add', '.')
        with self.assertRaisesRegex(ValueError, 'bump'):
            self.check()
        self.git('reset', '-q', 'HEAD', '--', 'skills/nestworth/references/new.md')
        path.unlink()
        original = self.skill / 'references/analysis.md'
        original.rename(path)
        with self.assertRaisesRegex(ValueError, 'bump'):
            self.check()
        path.unlink()
        with self.assertRaisesRegex(ValueError, 'bump'):
            self.check()

    def test_numeric_increase_and_unrelated_change(self):
        (self.root / 'app.txt').write_text('app only')
        self.check()
        (self.skill / 'VERSION').write_text('2.10.0\n')
        self.check()
        self.commit()
        (self.skill / 'VERSION').write_text('2.9.0\n')
        with self.assertRaisesRegex(ValueError, 'bump'):
            self.check('HEAD')

    def test_invalid_versions_and_missing_history_fail(self):
        for value in ('2.01.0', '2.1', 'v2.1.0', '2.1.0-rc.1'):
            with self.subTest(value=value), self.assertRaises(ValueError):
                checker.version_tuple(value)
        with self.assertRaises(subprocess.CalledProcessError):
            self.check('missing-baseline')

    def test_ci_uses_before_tip_and_main_merge_base(self):
        event = self.root / 'event.json'
        self.git('update-ref', 'refs/remotes/origin/main', self.base)
        with patch.dict(os.environ, GITHUB_EVENT_PATH=str(event), GITHUB_EVENT_NAME='push'):
            event.write_text(json.dumps({'before': self.base}))
            self.assertEqual(checker.ci_base(), self.base)
            event.write_text(json.dumps({'before': '0' * 40}))
            self.assertEqual(checker.ci_base(), self.base)
            event.write_text('{}')
            with patch.dict(os.environ, GITHUB_EVENT_NAME='workflow_dispatch'):
                self.assertEqual(checker.ci_base(), self.base)


if __name__ == '__main__':
    unittest.main()
