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
        (self.skill / 'VERSION').write_text('1.9.0\n')
        self.commit()
        (self.skill / 'VERSION').write_text('2.0.0\n')
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

    @contextlib.contextmanager
    def push_event(self, before, ref='refs/heads/topic'):
        event = self.root / 'event.json'
        event.write_text(json.dumps({'before': before}))
        self.git('update-ref', 'refs/remotes/origin/main', self.base)
        with patch.dict(os.environ, GITHUB_EVENT_PATH=str(event), GITHUB_EVENT_NAME='push',
                        GITHUB_REF=ref, GITHUB_SHA=self.git('rev-parse', 'HEAD')):
            yield

    def test_unrelated_followup_push_cannot_hide_missing_bump(self):
        (self.skill / 'references/analysis.md').write_text('new contract')
        self.commit()
        previous = self.git('rev-parse', 'HEAD')
        (self.root / 'app.txt').write_text('unrelated')
        self.commit()
        with self.push_event(previous), self.assertRaisesRegex(ValueError, 'bump'):
            self.check(checker.ci_base())

    def test_followup_skill_edit_keeps_pr_bump_valid(self):
        (self.skill / 'VERSION').write_text('2.1.0\n')
        (self.skill / 'references/analysis.md').write_text('new contract')
        self.commit()
        previous = self.git('rev-parse', 'HEAD')
        (self.skill / 'references/analysis.md').write_text('clarified new contract')
        self.commit()
        with self.push_event(previous):
            self.check(checker.ci_base())

    def test_whitespace_version_boundary_cannot_hide_main_drift(self):
        (self.skill / 'VERSION').write_text('2.0.0\n\n')
        (self.skill / 'references/analysis.md').write_text('unversioned main edit')
        self.commit()
        previous = self.git('rev-parse', 'HEAD')
        with self.push_event(self.base, 'refs/heads/main'):
            with self.assertRaisesRegex(ValueError, 'bump'):
                checker.check_ci_version()
        (self.root / 'app.txt').write_text('unrelated followup')
        self.commit()
        with self.push_event(previous, 'refs/heads/main'):
            with self.assertRaisesRegex(ValueError, 'boundary'):
                checker.check_ci_version()

    def test_boundary_rejects_downgrade_and_invalid_parent(self):
        (self.skill / 'VERSION').write_text('1.8.0\n')
        self.commit()
        with self.assertRaisesRegex(ValueError, 'numerically increase'):
            checker.main_version_boundary()
        (self.skill / 'VERSION').write_text('not-semver\n')
        self.commit()
        (self.skill / 'VERSION').write_text('2.1.0\n')
        self.commit()
        with self.assertRaisesRegex(ValueError, 'missing/invalid parent VERSION'):
            checker.main_version_boundary()

    def test_boundary_rejects_missing_parent_version_and_root(self):
        self.git('rm', 'skills/nestworth/VERSION')
        self.commit()
        (self.skill / 'VERSION').write_text('2.1.0\n')
        self.commit()
        with self.assertRaisesRegex(ValueError, 'missing/invalid parent VERSION'):
            checker.main_version_boundary()
        self.git('checkout', '--detach', self.git('rev-list', '--max-parents=0', 'HEAD'))
        with self.assertRaisesRegex(ValueError, 'reviewed bootstrap required'):
            checker.main_version_boundary()

    def test_boundary_search_has_a_hard_first_parent_limit(self):
        with patch.object(checker, 'MAX_VERSION_HISTORY', 3):
            for index in range(2):
                (self.root / 'app.txt').write_text(str(index))
                self.commit()
            self.assertEqual(checker.main_version_boundary(), self.base)
            (self.root / 'app.txt').write_text('third unrelated commit')
            self.commit()
            with self.assertRaisesRegex(ValueError, 'within 3 first-parent commits'):
                checker.main_version_boundary()

    def test_merge_boundary_uses_first_parent_version(self):
        self.git('checkout', '-b', 'other')
        (self.skill / 'VERSION').write_text('1.9.0\n')
        self.commit()
        other = self.git('rev-parse', 'HEAD')
        self.git('checkout', '-b', 'integration', self.base)
        (self.skill / 'VERSION').write_text('2.2.0\n')
        self.commit()
        first = self.git('rev-parse', 'HEAD')
        (self.skill / 'VERSION').write_text('2.0.0\n')
        self.git('add', '.')
        tree = self.git('write-tree')
        merge = self.git('commit-tree', tree, '-p', first, '-p', other, '-m', 'merge')
        self.git('checkout', '--detach', merge)
        with self.assertRaisesRegex(ValueError, 'numerically increase'):
            checker.main_version_boundary()

    def test_main_unrelated_followup_cannot_hide_failed_version_check(self):
        (self.skill / 'references/analysis.md').write_text('unversioned main edit')
        self.commit()
        previous = self.git('rev-parse', 'HEAD')
        (self.root / 'app.txt').write_text('unrelated followup')
        self.commit()
        with self.push_event(previous, 'refs/heads/main'):
            with self.assertRaisesRegex(ValueError, 'bump'):
                checker.check_ci_version()

    def test_main_integration_can_include_multiple_branch_edits(self):
        (self.skill / 'VERSION').write_text('2.1.0\n')
        (self.skill / 'references/analysis.md').write_text('integrated guidance')
        self.commit()
        with self.push_event(self.base, 'refs/heads/main'):
            with contextlib.redirect_stdout(io.StringIO()):
                checker.check_ci_version()
        previous = self.git('rev-parse', 'HEAD')
        (self.root / 'app.txt').write_text('unrelated followup')
        self.commit()
        with self.push_event(previous, 'refs/heads/main'):
            with contextlib.redirect_stdout(io.StringIO()):
                checker.check_ci_version()

    def test_ambiguous_merge_base_fails(self):
        (self.root / 'left.txt').write_text('left')
        self.commit()
        left = self.git('rev-parse', 'HEAD')
        self.git('checkout', '-b', 'right', self.base)
        (self.root / 'right.txt').write_text('right')
        self.commit()
        right = self.git('rev-parse', 'HEAD')
        tree = self.git('rev-parse', 'HEAD^{tree}')
        first = self.git('commit-tree', tree, '-p', left, '-p', right, '-m', 'left merge')
        second = self.git('commit-tree', tree, '-p', right, '-p', left, '-m', 'right merge')
        self.git('checkout', '--detach', second)
        with self.push_event(self.base):
            self.git('update-ref', 'refs/remotes/origin/main', first)
            with self.assertRaisesRegex(ValueError, 'unambiguous'):
                checker.ci_base()

    def test_first_branch_push_ignores_zero_before_and_missing_old_push(self):
        for before in ('0' * 40, 'f' * 40):
            with self.subTest(before=before), self.push_event(before):
                self.assertEqual(checker.ci_base(), self.base)

    def test_main_push_and_manual_use_integration_baseline(self):
        (self.root / 'app.txt').write_text('main integration')
        self.commit()
        with self.push_event(self.base, 'refs/heads/main'):
            self.assertEqual(checker.ci_base(), self.base)
            with patch.dict(os.environ, GITHUB_EVENT_NAME='workflow_dispatch'):
                self.assertEqual(checker.ci_base(), self.base)

    def test_initial_main_and_missing_main_push_tip_fail(self):
        root = self.git('rev-list', '--max-parents=0', 'HEAD')
        self.git('checkout', '--detach', root)
        with self.push_event('0' * 40, 'refs/heads/main'):
            with self.assertRaisesRegex(ValueError, 'no prior tip'):
                checker.ci_base()
            with patch.dict(os.environ, GITHUB_EVENT_NAME='workflow_dispatch'):
                with self.assertRaises(subprocess.CalledProcessError):
                    checker.ci_base()
        with self.push_event('f' * 40, 'refs/heads/main'):
            with self.assertRaises(subprocess.CalledProcessError):
                checker.ci_base()

    def test_pr_tags_and_wrong_event_head_are_rejected(self):
        with self.push_event(self.base):
            with patch.dict(os.environ, GITHUB_EVENT_NAME='pull_request'):
                with self.assertRaisesRegex(ValueError, 'branch push/manual'):
                    checker.ci_base()
            with patch.dict(os.environ, GITHUB_REF='refs/tags/v99.0.0'):
                with self.assertRaisesRegex(ValueError, 'branch push/manual'):
                    checker.ci_base()
            with patch.dict(os.environ, GITHUB_SHA='f' * 40):
                with self.assertRaisesRegex(ValueError, 'exact event head'):
                    checker.ci_base()

    def test_missing_main_ref_does_not_fall_back_to_tag(self):
        with self.push_event(self.base):
            self.git('tag', 'v99.0.0')
            self.git('update-ref', '-d', 'refs/remotes/origin/main')
            with self.assertRaises(subprocess.CalledProcessError):
                checker.ci_base()

    def test_shallow_clone_is_rejected_even_with_visible_merge_base(self):
        clone = self.root / 'shallow'
        self.git('clone', '--depth=1', self.root.as_uri(), str(clone))
        with self.push_event(self.base), patch.object(checker, 'ROOT', clone):
            with self.assertRaisesRegex(ValueError, 'shallow'):
                checker.ci_base()

    def test_unrelated_main_history_is_rejected(self):
        self.git('checkout', '--orphan', 'unrelated')
        (self.root / 'new.txt').write_text('new root')
        self.commit()
        with self.push_event(self.base, 'refs/heads/main'):
            with self.assertRaises(subprocess.CalledProcessError):
                checker.ci_base()
        with self.push_event(self.base):
            with self.assertRaises(subprocess.CalledProcessError):
                checker.ci_base()

    def test_concurrent_main_version_is_not_reused(self):
        self.git('checkout', '-b', 'main-update')
        (self.skill / 'VERSION').write_text('2.1.0\n')
        self.commit()
        newer_main = self.git('rev-parse', 'HEAD')
        self.git('checkout', '-b', 'topic', self.base)
        (self.skill / 'VERSION').write_text('2.1.0\n')
        (self.skill / 'references/analysis.md').write_text('topic guidance')
        self.commit()
        with self.push_event(self.base):
            self.git('update-ref', 'refs/remotes/origin/main', newer_main)
            with self.assertRaisesRegex(ValueError, 'above 2.1.0'):
                checker.check_ci_version()
            (self.skill / 'VERSION').write_text('2.2.0\n')
            with contextlib.redirect_stdout(io.StringIO()):
                checker.check_ci_version()
        self.commit()
        self.git('checkout', '-b', 'app-only', self.base)
        (self.root / 'app.txt').write_text('no skill changes')
        self.commit()
        with self.push_event(self.base):
            self.git('update-ref', 'refs/remotes/origin/main', newer_main)
            with contextlib.redirect_stdout(io.StringIO()):
                checker.check_ci_version()

    def test_missing_historical_version_is_not_assumed_initial_release(self):
        self.git('rm', 'skills/nestworth/VERSION')
        self.commit()
        historical = self.git('rev-parse', 'HEAD')
        (self.skill / 'VERSION').write_text('2.1.0\n')
        with self.assertRaises(subprocess.CalledProcessError):
            self.check(historical)


if __name__ == '__main__':
    unittest.main()
