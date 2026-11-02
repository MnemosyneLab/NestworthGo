"""Version ownership, rebuild triggers, and fail-closed tool selection."""
import importlib.util
import os
import re
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('ci_versions', ROOT / 'tools/ci-toolchain-versions.py')
ci_versions = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ci_versions)


class ToolchainTests(unittest.TestCase):
    def test_version_changes_follow_go_mod_without_duplicate_pins(self):
        text = (ROOT / 'go.mod').read_text()
        before = ci_versions.versions(text)
        for name, value in [('go', '1.99.2'), ('wails', 'v3.99.1')]:
            changed = text.replace(before[name], value)
            self.assertEqual(ci_versions.versions(changed)[name], value)
        dockerfile = (ROOT / 'build/ci/Dockerfile').read_text()
        self.assertNotIn(before['go'], dockerfile)
        self.assertNotIn(before['wails'], dockerfile)
        workflow = (ROOT / '.github/workflows/ci-native-image.yml').read_text()
        for event in ('pull_request', 'push'):
            section = re.split(r'\n  \w+:', workflow.split('\n  ' + event + ':', 1)[1], maxsplit=1)[0]
            self.assertIn('      - go.mod', section)
            self.assertIn('      - tools/ci-toolchain-versions.py', section)
            self.assertIn('      - tools/ci-use-toolchain.sh', section)
        self.assertEqual(workflow.count('for target in native toolchain; do'), 4)
        self.assertNotIn('contents: write', workflow)
        self.assertNotIn('pull-requests: write', workflow)

    def test_ambiguous_or_unpinned_versions_fail_closed(self):
        text = (ROOT / 'go.mod').read_text()
        versions = ci_versions.versions(text)
        for invalid in (text.replace(versions['go'], '1.27'),
                        text.replace(versions['wails'], 'latest'),
                        text + '\ntoolchain go1.99.1\n',
                        text + '\nreplace github.com/wailsapp/wails/v3 => ../wails\n',
                        text + '\nreplace (\n github.com/wailsapp/wails/v3 => ../wails\n)\n'):
            with self.subTest(text=invalid[-100:]), self.assertRaises(ValueError):
                ci_versions.versions(invalid)

    def test_selected_go_mismatch_fails_before_using_or_saving_build_cache(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'tools').mkdir()
            for name in ('ci-use-toolchain.sh', 'ci-toolchain-versions.py'):
                shutil.copy(ROOT / 'tools' / name, root / 'tools' / name)
            (root / 'go.mod').write_text((ROOT / 'go.mod').read_text())
            (root / 'bin').mkdir()
            go = root / 'bin/go'
            go.write_text('#!/bin/sh\nif [ "$1" = env ]; then echo "$FAKE_GO_VERSION"; else echo "go version $FAKE_GO_VERSION linux/amd64"; fi\n')
            go.chmod(0o755)
            output = root / 'output'
            env = dict(os.environ, PATH=str(root / 'bin') + os.pathsep + os.environ['PATH'],
                       PREINSTALLED='false', GITHUB_OUTPUT=str(output))
            def verify(version):
                return subprocess.run(['bash', str(root / 'tools/ci-use-toolchain.sh'), 'verify-go'],
                                      env=dict(env, FAKE_GO_VERSION=version), capture_output=True, text=True)
            failed = verify('go1.99.1')
            self.assertNotEqual(failed.returncode, 0)
            self.assertIn('Selected Go does not match go.mod', failed.stderr)
            self.assertFalse(output.exists())
            version = ci_versions.versions((ROOT / 'go.mod').read_text())['go']
            self.assertEqual(verify('go' + version).returncode, 0)
            self.assertEqual(output.read_text(), 'go-version=' + version + '\n')
            output.unlink()
            # A PATH tool that happens to match is not the selected image binary.
            refused = subprocess.run(['bash', str(root / 'tools/ci-use-toolchain.sh'), 'verify-go'],
                                     env=dict(env, PREINSTALLED='true', FAKE_GO_VERSION='go' + version),
                                     capture_output=True)
            self.assertNotEqual(refused.returncode, 0)
            self.assertFalse(output.exists())


if __name__ == '__main__':
    unittest.main()
