import contextlib
import io
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import preview


class PreviewTests(unittest.TestCase):
    def test_publication_preserves_unrecognized_and_linked_targets(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            staged = root / "new-package"
            staged.mkdir()
            other = root / "other-app"
            other.mkdir()
            personal = other / "keep"
            personal.write_text("original")
            target = root / preview.NAME
            target.symlink_to(other, target_is_directory=True)
            for destination in [other, target]:
                with self.subTest(destination=destination):
                    with self.assertRaises(ValueError):
                        preview.publish(staged, destination)
                    self.assertEqual(personal.read_text(), "original")
                    self.assertTrue(staged.is_dir())
            self.assertTrue(target.is_symlink())

    def test_publication_exchanges_only_the_recognized_bundle(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            staged, target = root / "new", root / preview.NAME
            staged.mkdir()
            (staged / "version").write_text("one")
            preview.publish(staged, target)
            staged.mkdir()
            (staged / "version").write_text("two")
            with patch.object(preview, "verify_package") as verify:
                preview.publish(staged, target)
            verify.assert_called_once_with(target)
            self.assertEqual((target / "version").read_text(), "two")
            self.assertEqual((staged / "version").read_text(), "one")

    def test_finder_reopens_the_default_app_and_keeps_explicit_fixtures_isolated(self):
        package = Path("/test/Nestworth Gio.app")
        root = "/test/gio data"
        for env in [{}, {"NESTWORTH_GIO_RUN_DIR": root}]:
            with self.subTest(env=env), patch.dict(os.environ, env, clear=True), \
                    patch.object(preview, "MAC", True), \
                    patch.object(preview.subprocess, "check_output", return_value=root + "\n") as output, \
                    patch.object(preview.subprocess, "run") as run, contextlib.redirect_stdout(io.StringIO()):
                preview.launch(package)
                binary = package / "Contents/MacOS/Nestworth Gio"
                output.assert_called_once_with([binary, "--print-pilot-root"], text=True)
                commands = [call.args[0] for call in run.call_args_list]
                self.assertEqual(commands[:2], [[binary, "--ensure-pilot-fixture"], [binary, "--verify-pilot-fixture"]])
                expected = ["open", package]
                if env:
                    expected = ["open", "-n", "-a", package,
                                "--env", "NESTWORTH_GIO_RUN_DIR=" + root,
                                "--env", "NESTWORTH_DATABASE_PATH=" + root + "/data/nestworth.db",
                                "--env", "NESTWORTH_SETTINGS_PATH=" + root + "/data/settings.json"]
                self.assertEqual(commands[2:], [expected])
