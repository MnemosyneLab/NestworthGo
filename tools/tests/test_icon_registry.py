"""Frontend registrations must match the backend stable-ID allowlist."""
from pathlib import Path
import json
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]


class IconRegistryTests(unittest.TestCase):
    def test_ui_and_backend_keys_match(self):
        frontend = (ROOT / 'frontend/src/lib/iconCatalog.ts').read_text()
        keys = re.findall(r'key: "([^"]+)"', frontend)
        self.assertEqual(len(keys), 87)
        self.assertEqual(len(set(keys)), len(keys))
        regional = json.loads((ROOT / 'frontend/src/lib/regionalBankLogos.json').read_text())
        backend = set(re.findall(r'"([^"]+)":\s*{}', (ROOT / 'internal/domain/icons.go').read_text()))
        self.assertEqual(set(keys) | {item['key'] for item in regional}, backend)
