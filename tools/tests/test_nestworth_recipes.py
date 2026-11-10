"""Portable recipe rendering is checked against its single shared source."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("recipe_check", ROOT / "tools/check-nestworth-skill.py")
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)


class SharedRecipeTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory(prefix="nestworth-recipes-")
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.skill = self.root / "skills/nestworth"
        self.references = self.skill / "references"
        self.references.mkdir(parents=True)
        self.source = self.root / "internal/agentguidance/recipes.json"
        self.source.parent.mkdir(parents=True)
        self.source.write_bytes((ROOT / "internal/agentguidance/recipes.json").read_bytes())
        for path in (ROOT / "skills/nestworth/references").glob("*.md"):
            (self.references / path.name).write_bytes(path.read_bytes())
        for name, value in (("ROOT", self.root), ("SKILL", self.skill)):
            override = patch.object(checker, name, value)
            override.start()
            self.addCleanup(override.stop)

    def test_detects_source_and_reference_drift_then_syncs_idempotently(self):
        checker.shared_recipes()
        recipes = json.loads(self.source.read_text())
        recipes[0]["text"] += " Synthetic changed business meaning."
        self.source.write_text(json.dumps(recipes))
        with self.assertRaisesRegex(ValueError, "drift"):
            checker.shared_recipes()
        checker.shared_recipes(write=True)
        checker.shared_recipes()
        contents = {p.name: p.read_bytes() for p in self.references.glob("*.md")}
        checker.shared_recipes(write=True)
        self.assertEqual(contents, {p.name: p.read_bytes() for p in self.references.glob("*.md")})
        path = self.references / (recipes[0]["reference"] + ".md")
        path.write_text(path.read_text().replace(recipes[0]["text"], "synthetic reference-only change"))
        with self.assertRaisesRegex(ValueError, "drift"):
            checker.shared_recipes()

    def test_removed_recipe_does_not_leave_stale_business_guidance(self):
        recipes = json.loads(self.source.read_text())
        removed = recipes.pop(0)
        self.source.write_text(json.dumps(recipes))
        with self.assertRaisesRegex(ValueError, "drift"):
            checker.shared_recipes()
        checker.shared_recipes(write=True)
        checker.shared_recipes()
        text = (self.references / (removed["reference"] + ".md")).read_text()
        self.assertNotIn(removed["text"], text)
        self.assertNotIn("shared-recipes:start", text)

    def test_duplicate_entry_or_missing_markers_is_rejected(self):
        recipes = json.loads(self.source.read_text())
        self.source.write_text(json.dumps(recipes + [recipes[0]]))
        with self.assertRaisesRegex(ValueError, "Duplicate"):
            checker.shared_recipes()
        self.source.write_text(json.dumps(recipes))
        path = self.references / (recipes[0]["reference"] + ".md")
        path.write_text(path.read_text().replace("<!-- shared-recipes:end -->", ""))
        with self.assertRaisesRegex(ValueError, "markers"):
            checker.shared_recipes(write=True)
