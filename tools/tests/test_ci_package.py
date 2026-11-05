"""Packaging provenance rejects stale outputs without running native builds."""
import importlib.util
import io
from pathlib import Path
import plistlib
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('package_manifest', ROOT / 'tools/ci-package-manifest.py')
package = importlib.util.module_from_spec(spec)
spec.loader.exec_module(package)


class PackageManifestTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        files = {
            'internal/version/version.go': 'var ( Version = "v1.2.3"; Build = "4" )',
            'skills/nestworth/VERSION': '9.8.7\n',
            'dist/macos/Nestworth-1.2.3-arm64.dmg': 'synthetic dmg',
            'dist/macos/Nestworth-1.2.3-arm64.zip': 'synthetic zip',
            'dist/macos/SHA256SUMS': 'synthetic checksum',
            'dist/skills/nestworth-skill.tar.gz.sha256': 'synthetic checksum',
        }
        for name, content in files.items():
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content)
        self.plist = self.root / 'dist/macos/Nestworth.app/Contents/Info.plist'
        self.plist.parent.mkdir(parents=True)
        self.plist.write_bytes(plistlib.dumps(dict(CFBundleIdentifier='com.nestworth.app',
                                                 CFBundleShortVersionString='1.2.3', CFBundleVersion='4')))
        self.archive('9.8.7')

    def archive(self, version):
        with tarfile.open(self.root / 'dist/skills/nestworth-skill.tar.gz', 'w:gz') as bundle:
            data = version.encode()
            entry = tarfile.TarInfo('nestworth-skill/skills/nestworth/VERSION')
            entry.size = len(data)
            bundle.addfile(entry, io.BytesIO(data))

    def report(self):
        return package.manifest(self.root, 'a' * 40, 'refs/heads/feature/example', {'go': 'pinned'})

    def test_independent_versions_and_exact_allowlist(self):
        report = self.report()
        self.assertEqual((report['app_version'], report['app_build'], report['skill_version']),
                         ('1.2.3', '4', '9.8.7'))
        self.assertEqual(report['source_ref'], 'refs/heads/feature/example')
        self.assertEqual(report['source_sha'], 'a' * 40)
        self.assertEqual(len(report['artifacts']), 5)
        for item in report['artifacts']:
            self.assertEqual(len(item['sha256']), 64)
            self.assertGreater(item['bytes'], 0)

    def test_stale_skill_and_app_versions_fail(self):
        self.archive('9.8.6')
        with self.assertRaisesRegex(ValueError, 'skill version'):
            self.report()
        self.archive('9.8.7')
        identity = plistlib.loads(self.plist.read_bytes())
        identity['CFBundleShortVersionString'] = '1.2.2'
        self.plist.write_bytes(plistlib.dumps(identity))
        with self.assertRaisesRegex(ValueError, 'CFBundleShortVersionString'):
            self.report()

    def test_missing_empty_or_symlinked_artifact_fails(self):
        path = self.root / 'dist/macos/Nestworth-1.2.3-arm64.dmg'
        path.unlink()
        with self.assertRaisesRegex(ValueError, 'invalid package file'):
            self.report()
        path.touch()
        with self.assertRaisesRegex(ValueError, 'invalid package file'):
            self.report()
        path.unlink()
        path.symlink_to(self.root / 'dist/macos/Nestworth-1.2.3-arm64.zip')
        with self.assertRaisesRegex(ValueError, 'invalid package file'):
            self.report()

    def test_workflow_keeps_manual_branch_only_and_exact_checkout(self):
        workflow = (ROOT / '.github/workflows/package-macos.yml').read_text()
        events = workflow.split('\non:\n', 1)[1].split('\npermissions:', 1)[0]
        self.assertEqual(events.strip(), 'workflow_dispatch:')
        self.assertIn("github.ref_type == 'branch'", workflow)
        self.assertIn('ref: ${{ github.sha }}', workflow)
        self.assertIn('persist-credentials: false', workflow)
        self.assertIn('contents: read', workflow)
        self.assertIn('runs-on: [self-hosted, macOS, ARM64]', workflow)
        self.assertIn('run: wails3 task package:release', workflow)
        self.assertIn('python3 tools/ci-package-manifest.py', workflow)
        self.assertNotIn('secrets.', workflow)
        paths = workflow.split('          path: |\n', 1)[1].split('          if-no-files-found:', 1)[0]
        self.assertEqual(len(paths.strip().splitlines()), 6)
        self.assertNotIn('**', paths)
