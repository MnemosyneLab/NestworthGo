"""Exercise cache input invalidation and complete output validation with synthetic data."""
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module

cache = load('bindings_cache', ROOT / 'tools/bindings-cache.py')
gate = load('bindings_compare_gate', ROOT / 'tools/bindings-compare-gate.py')


class BindingsCacheTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        subprocess.run(['git', 'init', '-q', str(self.root)], check=True)
        (self.root / 'go.mod').write_text('module test\n')
        (self.root / 'type.go').write_text('type A struct { Value string `json:"value"` }')
        self.cli = self.root / 'cli'
        self.cli.write_bytes(b'cli')
        self.env = {'GOOS': 'linux', 'GOARCH': 'amd64', 'GOWORK': '', 'GOFLAGS': ''}
        self.module = {'Replace': []}
        self.go_version = 'go version go1.27.3 linux/amd64'
        self.package_files = {}
        original = cache.command
        def command(*args, cwd=cache.ROOT):
            if args[:3] == ('go', 'env', '-json'):
                return json.dumps(self.env).encode()
            if args[:3] == ('go', 'mod', 'edit'):
                return json.dumps(self.module).encode()
            if args[:2] == ('go', 'version'):
                return self.go_version.encode()
            if args[:2] == ('go', 'list'):
                return json.dumps({'Dir': str(self.root.resolve()), **self.package_files}).encode()
            if args[:2] == ('node', '--version'):
                return b'v24.22.0'
            return original(*args, cwd=cwd)
        self.patches = [patch.object(cache, 'command', side_effect=command),
                        patch.object(cache.shutil, 'which', return_value=str(self.cli)),
                        patch.object(cache.subprocess, 'check_output', wraps=subprocess.check_output)]
        for item in self.patches[:2]:
            item.start(); self.addCleanup(item.stop)
        # Version is emitted on stderr by the real CLI.
        self.cli.write_text('#!/bin/sh\necho v3.0.0 >&2\n')
        self.cli.chmod(0o755)

    def test_source_added_deleted_type_tag_and_toolchain_change_key(self):
        initial = cache.input_key(self.root)
        source = self.root / 'type.go'
        source.write_text('type A struct { Value int `json:"value"` }')
        type_changed = cache.input_key(self.root)
        self.assertNotEqual(initial, type_changed)
        source.write_text('type A struct { Value int `json:"renamed"` }')
        changed = cache.input_key(self.root)
        self.assertNotEqual(type_changed, changed)
        (self.root / 'added.go').write_text('package test')
        added = cache.input_key(self.root)
        self.assertNotEqual(changed, added)
        (self.root / 'added.go').unlink()
        self.assertEqual(changed, cache.input_key(self.root))
        self.go_version += ' modified'
        self.assertNotEqual(changed, cache.input_key(self.root))
        toolchain = cache.input_key(self.root)
        self.env['GOARCH'] = 'arm64'
        self.assertNotEqual(toolchain, cache.input_key(self.root))
        arch = cache.input_key(self.root)
        self.cli.write_text('#!/bin/sh\necho v3.0.0 >&2\n# changed binary\n')
        self.assertNotEqual(arch, cache.input_key(self.root))
        subprocess.run(['git', 'add', 'type.go'], cwd=self.root, check=True)
        source.unlink()
        with self.assertRaises(ValueError): cache.input_key(self.root)

    def test_presentation_changes_reuse_but_embeds_and_inputs_invalidate(self):
        ui = self.root / 'frontend/src/App.tsx'; ui.parent.mkdir(parents=True)
        ui.write_text('before')
        docs = self.root / 'docs/readme.md'; docs.parent.mkdir(parents=True)
        docs.write_text('before')
        readme = self.root / 'README.md'; readme.write_text('before')
        initial = cache.input_key(self.root)
        ui.write_text('after'); docs.write_text('after'); readme.write_text('after')
        (self.root / 'frontend/src/New.tsx').write_text('new')
        self.assertEqual(initial, cache.input_key(self.root))
        ui.unlink(); docs.unlink(); readme.unlink()
        self.assertEqual(initial, cache.input_key(self.root))
        ui.write_text('embedded')
        self.package_files['EmbedFiles'] = ['frontend/src/App.tsx']
        embedded = cache.input_key(self.root)
        self.assertNotEqual(initial, embedded)
        ui.write_text('changed embedded bytes')
        self.assertNotEqual(embedded, cache.input_key(self.root))
        added = self.root / 'frontend/src/new.go'; added.write_text('package test')
        self.assertNotEqual(embedded, cache.input_key(self.root))
        added.unlink()
        header = self.root / 'docs/nested/header.h'; header.parent.mkdir(parents=True)
        header.write_text('header')
        before_header = cache.input_key(self.root)
        header.write_text('changed header')
        self.assertNotEqual(before_header, cache.input_key(self.root))
        config = self.root / 'frontend/package.json'; config.write_text('{}')
        before_config = cache.input_key(self.root)
        config.write_text('{"changed":true}')
        self.assertNotEqual(before_config, cache.input_key(self.root))

    def test_list_failure_disables_input_key(self):
        with patch.object(cache, 'package_source_files', side_effect=subprocess.CalledProcessError(1, 'go')):
            with self.assertRaises(subprocess.CalledProcessError): cache.input_key(self.root)

    def test_unsupported_inputs_disable_reuse(self):
        for field, value in [('GOWORK', '/outside/go.work'), ('GOFLAGS', '-tags=custom')]:
            self.env[field] = value
            with self.assertRaises(ValueError): cache.input_key(self.root)
            self.env[field] = ''
        self.module = {'Replace': [{'New': {'Path': '../outside'}}]}
        with self.assertRaises(ValueError): cache.input_key(self.root)
        self.module = {'Replace': []}
        (self.root / '.gitignore').write_text('ignored.go\n')
        (self.root / 'ignored.go').write_text('package test')
        with self.assertRaises(ValueError): cache.input_key(self.root)

    def test_corrupt_missing_extra_and_unavailable_outputs_fail_closed(self):
        bindings = self.root / 'generated'
        marker = bindings / cache.MARKER
        marker.parent.mkdir(parents=True)
        marker.write_text('generated')
        other = bindings / 'other.ts'; other.write_text('other')
        saved = self.root / 'cache'
        with self.assertRaises(OSError): cache.validate(saved, 'abc')
        cache.save(saved, 'abc', bindings)
        self.assertTrue(cache.validate(saved, 'abc'))
        self.assertFalse(cache.validate(saved, 'different'))
        saved_other = saved / 'bindings/other.ts'
        saved_other.write_text('corrupt')
        self.assertFalse(cache.validate(saved, 'abc'))
        saved_other.unlink()
        self.assertFalse(cache.validate(saved, 'abc'))
        saved_other.write_text('other')
        (saved / 'bindings/extra.ts').write_text('extra')
        self.assertFalse(cache.validate(saved, 'abc'))

    def test_compare_gate_preserves_contract_checks(self):
        self.assertFalse(gate.requires_comparison(['frontend/src/App.tsx', 'docs/readme.md']))
        for path in ['internal/wailsapi/app.go', 'frontend/src/new.go', 'go.mod',
                     'frontend/scripts/ensure-bindings.mjs', 'frontend/package.json', '.github/workflows/check.yml', 'docs/new.go']:
            with self.subTest(path=path): self.assertTrue(gate.requires_comparison([path]))


@unittest.skipUnless(shutil.which('go'), 'Go is required for the embed inventory test')
class GoEmbedInventoryTests(unittest.TestCase):
    def test_real_go_tool_resolves_embed_glob_and_changes(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / 'go.mod').write_text('module example.com/cachefixture\ngo 1.23\n')
            (root / 'fixture.go').write_text(
                'package fixture\nimport "embed"\n//go:embed frontend/src/assets/*.txt\nvar Assets embed.FS\n')
            assets = root / 'frontend/src/assets'; assets.mkdir(parents=True)
            one = assets / 'one.txt'; one.write_text('one')
            two = assets / 'two.txt'; two.write_text('two')
            (root / 'tagged.go').write_text(
                '//go:build custom_fixture\n\npackage fixture\nimport "embed"\n'
                '//go:embed frontend/src/assets/tagged.bin\nvar Tagged embed.FS\n')
            (assets / 'tagged.bin').write_text('only in the custom-tag build')
            with patch.dict('os.environ', {'GOWORK': 'off', 'GOFLAGS': ''}):
                self.assertEqual(cache.package_source_files(root),
                                 {'frontend/src/assets/one.txt', 'frontend/src/assets/two.txt'})
                two.unlink()
                self.assertEqual(cache.package_source_files(root), {'frontend/src/assets/one.txt'})
                one.unlink()
                with self.assertRaises(subprocess.CalledProcessError): cache.package_source_files(root)


class BindingsTierTests(unittest.TestCase):
    def test_scheduled_and_release_tiers_compare_independently(self):
        perf = (ROOT / '.github/workflows/perf.yml').read_text()
        job = perf.split('  bindings-determinism:', 1)[1].split('  perf:', 1)[0]
        self.assertIn("if: needs.changes.result == 'success'", job)
        self.assertNotIn('should_run', job)
        self.assertIn('node scripts/ensure-bindings.mjs\n', job)
        self.assertIn('node scripts/ensure-bindings.mjs --check', job)
        release = (ROOT / 'build/darwin/Taskfile.yml').read_text().split('  package:release:', 1)[1].split('  verify:package:', 1)[0]
        self.assertIn('SCRIPT: check:bindings', release)
        gate_source = (ROOT / 'tools/bindings-compare-gate.py').read_text()
        self.assertIn("'--no-renames'", gate_source)
