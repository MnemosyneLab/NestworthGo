"""Pin the four selected brands and reject active or external SVG content."""
import hashlib
import importlib.util
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('crypto_logos', ROOT / 'tools/crypto-logos/import_logos.py')
logos = importlib.util.module_from_spec(spec)
spec.loader.exec_module(logos)


class CryptoLogoTests(unittest.TestCase):
    def test_fixed_assets_and_license(self):
        manifest = json.loads((ROOT / 'tools/crypto-logos/manifest.json').read_text())
        self.assertEqual(manifest['revision'], logos.REVISION)
        self.assertEqual({item['key'] for item in manifest['assets']}, {f'crypto-logo:{s}' for s in logos.SYMBOLS})
        directory = ROOT / 'frontend/public/crypto-logos'
        self.assertEqual(len(list(directory.glob('*.svg'))), 4)
        for entry in manifest['assets']:
            raw = (directory / entry['file']).read_bytes()
            self.assertEqual(hashlib.sha256(raw).hexdigest(), entry['sha256'])
            self.assertEqual(logos.validate(raw.decode()), raw.decode())
        self.assertIn('CC0 1.0 Universal', (directory / 'LICENSE.md').read_text())

    def test_active_content_and_external_references_rejected(self):
        for content in ['<script/>', '<foreignObject/>', '<image href="https://example.com"/>', '<use href="#x"/>', '<style/>', '<path d="M0 0" onload="alert(1)"/>', '<path d="M0 0" fill="url(#x)"/>', '<path d="M0 0" fill-opacity="nan"/>', '<g xmlns="urn:evil"/>']:
            with self.subTest(content=content), self.assertRaises(ValueError):
                logos.validate(f'<svg xmlns="{logos.SVG}" viewBox="0 0 32 32">{content}</svg>')
        with self.assertRaises(ValueError):
            logos.validate('<!DOCTYPE svg><svg/>')
