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
        # Every Git caller (including checker subprocesses and clone children)
        # shares a scoped, hermetic configuration. Prevent automatic maintenance
        # from outliving a command and writing into a directory during cleanup.
        empty_template = self.root / '.git-template'
        empty_template.mkdir()
        git_env = {key: value for key, value in os.environ.items() if not key.startswith('GIT_')}
        git_env.update(GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL=os.devnull,
                       GIT_TEMPLATE_DIR=str(empty_template), GIT_CONFIG_COUNT='5')
        for index, (key, value) in enumerate((
                ('maintenance.auto', 'false'), ('gc.auto', '0'),
                ('gc.autoDetach', 'false'), ('maintenance.autoDetach', 'false'),
                ('core.hooksPath', str(empty_template)))):
            git_env[f'GIT_CONFIG_KEY_{index}'] = key
            git_env[f'GIT_CONFIG_VALUE_{index}'] = value
        environment = patch.dict(os.environ, git_env, clear=True)
        environment.start()
        self.addCleanup(environment.stop)
        self.skill = self.root / 'skills/nestworth'
        self.skill.mkdir(parents=True)
        for name, value in {
            'skills/nestworth/VERSION': '2.0.0\n',
            'skills/nestworth/SKILL.md': 'instructions',
            'skills/nestworth/references/analysis.md': 'analysis',
            'skills/nestworth/agents/openai.yaml': 'metadata',
            'tools/install-nestworth-skill.sh': 'installer',
            'docs/user/nestworth-skill.md': 'guide',
            'internal/agentguidance/recipes.json': '[]',
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
        for name, value in (('ROOT', self.root), ('SKILL', self.skill)):
            override = patch.object(checker, name, value)
            override.start()
            self.addCleanup(override.stop)

    def git(self, *args):
        return subprocess.check_output(['git', *args], cwd=self.root, text=True, stderr=subprocess.PIPE).strip()

    def commit(self):
        self.git('add', '.')
        self.git('commit', '-qm', 'synthetic fixture')

    def check(self, base=None):
        with contextlib.redirect_stdout(io.StringIO()):
            checker.check_version(base or self.base)

    def test_git_subprocesses_cannot_spawn_automatic_maintenance(self):
        # Host/local preferences must not reactivate maintenance. Trace actual
        # Git child launches rather than infer process lifetime from a sleep.
        self.git('config', '--local', 'maintenance.auto', 'true')
        self.git('config', '--local', 'gc.auto', '1')
        self.git('config', '--local', 'gc.autoDetach', 'true')
        self.assertEqual(self.git('config', '--get', 'maintenance.auto'), 'false')
        self.assertEqual(self.git('config', '--get', 'gc.auto'), '0')
        self.assertEqual(self.git('config', '--get', 'gc.autoDetach'), 'false')
        trace = self.root / '.git/trace2-events.json'
        with patch.dict(os.environ, GIT_TRACE2_EVENT=str(trace)):
            for index in range(3):
                (self.root / 'app.txt').write_text(str(index))
                self.commit()
            self.git('clone', '--depth=1', self.root.as_uri(), str(self.root / 'clone'))
            self.assertEqual(checker.main_version_boundary(), self.base)
        children = [event.get('argv', []) for event in map(json.loads, trace.read_text().splitlines())
                    if event.get('event') == 'child_start']
        self.assertTrue(children, 'Trace must include actual clone subprocesses')
        self.assertFalse([argv for argv in children if 'maintenance' in argv or 'gc' in argv], children)

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

    def normalization_source(self):
        (self.skill / 'VERSION').write_text('6.0.1\n')
        self.commit()
        self.base = self.git('rev-parse', 'HEAD')
        override = patch.object(checker, 'NORMALIZATION_SOURCE', self.base)
        override.start()
        self.addCleanup(override.stop)

    def test_exact_normalization_branch_and_main_then_patch(self):
        self.normalization_source()
        (self.skill / 'VERSION').write_text('3.0.1\n')
        self.check()
        self.commit()
        normalized = self.git('rev-parse', 'HEAD')
        for ref in ('refs/heads/topic', 'refs/heads/main'):
            with self.subTest(ref=ref), self.push_event(self.base, ref):
                with contextlib.redirect_stdout(io.StringIO()):
                    checker.check_ci_version()
        self.assertEqual(checker.main_version_boundary(), normalized)
        (self.root / 'app.txt').write_text('unrelated followup')
        self.commit()
        with self.push_event(normalized, 'refs/heads/main'):
            with contextlib.redirect_stdout(io.StringIO()):
                checker.check_ci_version()
        (self.skill / 'references/analysis.md').write_text('later guidance')
        self.commit()
        drift = self.git('rev-parse', 'HEAD')
        with self.push_event(normalized, 'refs/heads/main'):
            with self.assertRaisesRegex(ValueError, 'bump'):
                checker.check_ci_version()
        (self.skill / 'VERSION').write_text('3.0.2\n')
        self.commit()
        with self.push_event(drift, 'refs/heads/main'):
            with contextlib.redirect_stdout(io.StringIO()):
                checker.check_ci_version()

    def test_normalization_cannot_allow_other_decreases_or_reuse(self):
        self.normalization_source()
        (self.skill / 'references/analysis.md').write_text('normalization guidance')
        for version in ('3.0.0', '3.0.2', '6.0.0', '6.0.1'):
            with self.subTest(version=version):
                (self.skill / 'VERSION').write_text(version + '\n')
                with self.assertRaisesRegex(ValueError, 'bump'):
                    self.check()
        (self.skill / 'VERSION').write_text('3.0.1\n')
        self.commit()
        normalized = self.git('rev-parse', 'HEAD')
        (self.skill / 'references/analysis.md').write_text('unversioned edit')
        with self.assertRaisesRegex(ValueError, 'bump'):
            self.check(normalized)
        (self.skill / 'VERSION').write_text('3.0.0\n')
        with self.assertRaisesRegex(ValueError, 'bump'):
            self.check(normalized)

    def test_normalization_rejects_wrong_source_and_concurrent_main(self):
        self.normalization_source()
        source = self.base
        (self.root / 'app.txt').write_text('concurrent main')
        self.commit()
        newer_main = self.git('rev-parse', 'HEAD')
        (self.skill / 'VERSION').write_text('3.0.1\n')
        with self.assertRaisesRegex(ValueError, 'bump'):
            self.check(newer_main)
        with self.push_event(source):
            self.git('update-ref', 'refs/remotes/origin/main', newer_main)
            with self.assertRaisesRegex(ValueError, 'bump'):
                checker.check_ci_version()
        self.commit()
        with self.assertRaisesRegex(ValueError, 'numerically increase'):
            checker.main_version_boundary()

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
