"""Verify vendored artwork and fail-closed SVG screening without network access."""
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('bank_logos', ROOT / 'tools/bank-logos/import_logos.py')
logos = importlib.util.module_from_spec(spec)
spec.loader.exec_module(logos)


class BankLogoTests(unittest.TestCase):
    def test_catalog_provenance_and_backend_allowlist(self):
        manifest = json.loads((ROOT / 'tools/bank-logos/manifest.json').read_text())
        catalog = json.loads((ROOT / 'frontend/src/lib/bankLogos.json').read_text())
        directory = ROOT / 'frontend/public/bank-logos'
        self.assertEqual(manifest['revision'], logos.REVISION)
        self.assertEqual(len(catalog), 608)
        self.assertEqual(len(list(directory.glob('*.svg'))), 609)
        self.assertEqual(len({x['key'] for x in catalog}), 608)
        self.assertEqual({x['key'] for x in catalog}, {x['key'] for x in manifest['assets']})
        backend = (ROOT / 'internal/domain/bank_logos_generated.go').read_text()
        self.assertEqual(set(re.findall(r'"(bank-logo:[^"]+)"', backend)), {x['key'] for x in catalog})
        hashes = {x['key']: x['sha256'] for x in manifest['assets']}
        for entry in catalog:
            self.assertRegex(entry['file'], r'^[a-z0-9-]+\.svg$')
            raw = (directory / entry['file']).read_text()
            self.assertEqual(logos.sanitize_svg(raw), raw)
            self.assertEqual(hashlib.sha256(raw.encode()).hexdigest(), hashes[entry['key']])
            self.assertTrue(entry['name'])
            self.assertTrue(entry['aliases'])
        self.assertIn('Copyright (c) 2022 IconGo', (directory / 'LICENSE').read_text())

    def test_display_assets_only_change_canvas_and_cover_all_saved_ids(self):
        import xml.etree.ElementTree as ET
        import math
        catalog = json.loads((ROOT / 'frontend/src/lib/bankLogos.json').read_text())
        reviewed = json.loads((ROOT / 'tools/bank-logos/presentation.json').read_text())
        display = json.loads((ROOT / 'frontend/src/lib/bankLogoPresentation.json').read_text())
        regional = json.loads((ROOT / 'frontend/src/lib/regionalBankLogos.json').read_text())
        replaced = {key for item in regional for key in item['legacyKeys']}
        reviewed = [item for item in reviewed if item['key'] not in replaced] + [{k: item[k] for k in ('key', 'legacyKeys', 'kind')} for item in regional]
        catalog += regional
        self.assertEqual(len(display), 305)
        keys = [key for item in display for key in [item['key'], *item['legacyKeys']]]
        self.assertEqual(len(keys), len(set(keys)))
        self.assertEqual(set(keys), {item['key'] for item in catalog})
        original = {item['key']: item for item in catalog}
        for choice, review in zip(display, reviewed):
            self.assertEqual({k: choice[k] for k in review}, review)
            self.assertIn(choice['kind'], ['symbol', 'wordmark'])
            path = ROOT / 'frontend/public/bank-logos' / choice['file']
            raw = path.read_text()
            self.assertEqual(logos.sanitize_svg(raw), raw)
            before = ET.parse(ROOT / 'frontend/public/bank-logos' / original[choice['key']]['file']).getroot()
            after = ET.fromstring(raw)
            self.assertEqual([node.attrib for node in before], [node.attrib for node in after])
            box = [float(n) for n in after.attrib['viewBox'].split()]
            self.assertTrue(all(math.isfinite(n) for n in box))
            self.assertTrue(box[2] > 0 and box[3] > 0)
            self.assertTrue(choice['name'])

    def test_regional_source_hash_and_identity(self):
        sources = json.loads((ROOT / 'tools/bank-logos/regional-sources.json').read_text())
        raw = (ROOT / 'frontend/public/bank-logos/standard-chartered.svg').read_text()
        self.assertEqual(logos.sanitize_svg(raw), raw)
        self.assertEqual(hashlib.sha256(raw.encode()).hexdigest(), sources['standardChartered']['sha256'])
        self.assertIn('PD-textlogo', sources['standardChartered']['license'])
        regional = json.loads((ROOT / 'frontend/src/lib/regionalBankLogos.json').read_text())
        self.assertEqual({item['key'] for item in regional}, {'bank-logo:standard-chartered', 'bank-logo:bochk'})
        self.assertEqual(regional[1]['legacyKeys'], [])

    def test_unsafe_svg_is_rejected(self):
        bad = [
            '<script>alert(1)</script>', '<foreignObject/>', '<image href="https://example.com/x"/>',
            '<use href="#x"/>', '<animate attributeName="fill"/>', '<style>@import "https://example.com";</style>',
            '<path d="M0 0" onload="alert(1)"/>', '<path d="M0 0" fill="url(https://example.com)"/>',
            '<path d="M0 0" style="fill:red"/>', '<path d="M0 0" xmlns="urn:evil"/>',
        ]
        for contents in bad:
            with self.subTest(contents=contents), self.assertRaises(ValueError):
                logos.sanitize_svg(f'<svg xmlns="{logos.SVG}" viewBox="0 0 10 10">{contents}</svg>')
        for raw in [
            '<!DOCTYPE svg [<!ENTITY x SYSTEM "file:///etc/passwd">]><svg/>',
            f'<svg xmlns="{logos.SVG}" viewBox="0 0 0 10"><path d="M0 0"/></svg>',
            f'<svg xmlns="{logos.SVG}" viewBox="0 0 nan 10"><path d="M0 0"/></svg>',
            f'<svg xmlns="{logos.SVG}" viewBox="0 0 10 10" onload="alert(1)"><path d="M0 0"/></svg>',
        ]:
            with self.subTest(raw=raw), self.assertRaises(ValueError):
                logos.sanitize_svg(raw)
