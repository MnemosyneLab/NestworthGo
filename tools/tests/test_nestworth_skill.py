"""Behavioral installer/package tests; every install target is temporary."""
import hashlib
import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
INSTALLER = ROOT / "tools/install-nestworth-skill.sh"
SOURCE = ROOT / "skills/nestworth"
SOURCE_VERSION = (SOURCE / "VERSION").read_text().strip()
spec = importlib.util.spec_from_file_location("skill_package", ROOT / "tools/package-nestworth-skill.py")
packager = importlib.util.module_from_spec(spec)
spec.loader.exec_module(packager)


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="nestworth-skill-test-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.skills = self.root / "Codex profile with spaces" / "skills"
        self.target = self.skills / "nestworth"

    def run_installer(self, *args, ok=True, env=None, script=INSTALLER):
        result = subprocess.run(["bash", str(script), "--skills-dir", str(self.skills), *map(str, args)],
                                capture_output=True, text=True, env=env)
        if ok:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def archive(self):
        with contextlib.redirect_stdout(io.StringIO()):
            return packager.package(self.root / "bundle")

    def backups(self):
        root = self.skills.parent / "nestworth-skill-backups"
        return sorted(root.iterdir()) if root.exists() else []

    def checksum(self, archive):
        archive.with_name(archive.name + ".sha256").write_text(
            f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n")

    def test_dry_run_and_status_do_not_create_target(self):
        result = self.run_installer("--dry-run")
        self.assertIn("Would install", result.stdout)
        self.assertFalse(self.skills.exists())
        self.assertIn("not installed", self.run_installer("--status").stdout)
        self.assertFalse(self.skills.exists())

    def test_archive_install_is_repeatable_and_preserves_other_skills(self):
        other = self.skills / "other" / "SKILL.md"
        other.parent.mkdir(parents=True)
        other.write_text("unrelated content")
        archive = self.archive()
        self.run_installer("--source", archive)
        self.assertEqual((self.target / "SKILL.md").read_bytes(), (SOURCE / "SKILL.md").read_bytes())
        self.assertIn(SOURCE_VERSION, self.run_installer("--status").stdout)
        self.assertIn("already current", self.run_installer("--source", archive).stdout)
        self.assertEqual(self.backups(), [])
        self.assertEqual(other.read_text(), "unrelated content")
        self.assertFalse((self.skills / ".nestworth-install.lock").exists())

    def test_packaged_context_examples_survive_install(self):
        archive = self.archive()
        with tarfile.open(archive, "r:gz") as bundle:
            prefix = "nestworth-skill/skills/nestworth/"
            self.assertEqual(bundle.extractfile(prefix + "VERSION").read().decode().strip(), SOURCE_VERSION)
            analysis = bundle.extractfile(prefix + "references/analysis.md").read()
            connection = bundle.extractfile(prefix + "references/connection.md").read()
        examples = {name: json.loads(raw) for name, raw in re.findall(
            r"<!-- example: ([a-z-]+) -->\s*```json\n(.*?)\n```", analysis.decode(), re.S)}
        self.assertEqual(examples["financial-context"]["disclosure"], "minimal")
        self.assertEqual(examples["financial-context-named"]["disclosure"], "named")
        for section in ("positions", "gaps", "evidence"):
            arguments = examples[f"financial-context-{section}-page"]
            self.assertEqual(arguments["section"], section)
            self.assertEqual(arguments["contextId"], "${contextId}")
            self.assertEqual(arguments["cursor"], "${cursor}")
        config_match = re.search(r"<!-- example: inspector-http-config -->\s*```json\n(.*?)\n```",
                                 connection.decode(), re.S)
        self.assertIsNotNone(config_match)
        server = json.loads(config_match.group(1))["mcpServers"]["nestworth"]
        self.assertEqual(server["type"], "http")
        self.assertEqual(server["url"], "${endpoint}")
        self.assertEqual(server["headers"]["Authorization"], "Bearer ${token}")
        self.run_installer("--source", archive)
        self.assertEqual((self.target / "references/analysis.md").read_bytes(), analysis)
        self.assertEqual((self.target / "references/connection.md").read_bytes(),
                         connection)
        self.assertIn(SOURCE_VERSION, self.run_installer("--status").stdout)

    def test_update_backs_up_edits_and_backup_is_outside_discovery(self):
        self.run_installer()
        (self.target / "SKILL.md").write_text("user-modified skill")
        (self.target / "my notes.txt").write_text("user notes")
        updated = self.root / "updated"
        shutil.copytree(SOURCE, updated)
        (updated / "VERSION").write_text("99.0.0\n")  # Synthetic next version, independent of the shipped version.
        self.run_installer("--source", updated, "--dry-run")
        self.assertEqual((self.target / "SKILL.md").read_text(), "user-modified skill")
        self.assertEqual(self.backups(), [])
        self.run_installer("--source", updated)
        self.assertEqual((self.target / "VERSION").read_text().strip(), "99.0.0")
        backups = self.backups()
        self.assertEqual(len(backups), 1)
        self.assertFalse(backups[0].is_relative_to(self.skills))
        self.assertEqual((backups[0] / "SKILL.md").read_text(), "user-modified skill")
        self.assertEqual((backups[0] / "my notes.txt").read_text(), "user notes")
        self.assertFalse((self.target / "my notes.txt").exists())

    def test_unmanaged_directory_requires_explicit_replacement(self):
        self.target.mkdir(parents=True)
        (self.target / "SKILL.md").write_text("unmanaged")
        self.run_installer(ok=False)
        self.assertEqual((self.target / "SKILL.md").read_text(), "unmanaged")
        self.run_installer("--replace-unmanaged")
        self.assertEqual((self.backups()[0] / "SKILL.md").read_text(), "unmanaged")

    def test_reject_destination_symlink(self):
        original = self.root / "untouched"
        original.mkdir()
        self.skills.mkdir(parents=True)
        self.target.symlink_to(original, target_is_directory=True)
        self.run_installer(ok=False)
        self.assertEqual(list(original.iterdir()), [])
        self.assertTrue(self.target.is_symlink())

    def test_archive_checksum_failure_has_no_install_side_effect(self):
        archive = self.archive()
        with archive.open("ab") as output:
            output.write(b"tampered")
        result = self.run_installer("--source", archive, ok=False)
        self.assertIn("checksum mismatch", result.stderr)
        self.assertFalse(self.skills.exists())

    def test_reject_traversal_links_and_special_members(self):
        for kind in ("traversal", "symlink", "hardlink", "fifo"):
            with self.subTest(kind=kind):
                archive = self.root / f"{kind}.tar.gz"
                with tarfile.open(archive, "w:gz") as bundle:
                    entry = tarfile.TarInfo("nestworth-skill/../../escaped" if kind == "traversal"
                                             else "nestworth-skill/skills/nestworth/SKILL.md")
                    if kind == "symlink":
                        entry.type, entry.linkname = tarfile.SYMTYPE, "/tmp/escaped"
                    elif kind == "hardlink":
                        entry.type, entry.linkname = tarfile.LNKTYPE, "../../escaped"
                    elif kind == "fifo":
                        entry.type = tarfile.FIFOTYPE
                    else:
                        entry.size = 1
                    bundle.addfile(entry, io.BytesIO(b"x") if kind == "traversal" else None)
                self.checksum(archive)
                self.run_installer("--source", archive, ok=False)
                self.assertFalse((self.root / "escaped").exists())
                self.assertFalse(self.skills.exists())

    def test_source_validation_rejects_links_and_unrelated_files(self):
        source = self.root / "invalid"
        shutil.copytree(SOURCE, source)
        (source / "secrets.env").write_text("not a skill file")
        self.run_installer("--source", source, ok=False)
        (source / "secrets.env").unlink()
        (source / "references" / "linked.md").symlink_to(SOURCE / "SKILL.md")
        self.run_installer("--source", source, ok=False)
        self.assertFalse(self.skills.exists())

    def test_lock_owned_by_other_install_is_preserved(self):
        lock = self.skills / ".nestworth-install.lock"
        lock.mkdir(parents=True)
        self.run_installer(ok=False)
        self.assertTrue(lock.is_dir())
        self.assertFalse(self.target.exists())

    def test_failed_replacement_restores_previous_directory(self):
        self.run_installer()
        (self.target / "SKILL.md").write_text("preserve on failed rename")
        fake_bin = self.root / "bin"
        fake_bin.mkdir()
        fake_mv = fake_bin / "mv"
        fake_mv.write_text('#!/bin/bash\ncase "$1" in *.nestworth-stage.*/nestworth) exit 1;; esac\nexec /bin/mv "$@"\n')
        fake_mv.chmod(0o755)
        environment = {**os.environ, "PATH": str(fake_bin) + os.pathsep + os.environ["PATH"]}
        self.run_installer(env=environment, ok=False)
        self.assertEqual((self.target / "SKILL.md").read_text(), "preserve on failed rename")
        self.assertFalse((self.skills / ".nestworth-install.lock").exists())

    def test_bundle_installer_works_without_checkout_and_is_reproducible(self):
        first = self.archive()
        with contextlib.redirect_stdout(io.StringIO()):
            second = packager.package(self.root / "second bundle")
        self.assertEqual(first.read_bytes(), second.read_bytes())
        extracted = self.root / "extracted"
        extracted.mkdir()
        # This is our freshly built trusted bundle, not an unvalidated download.
        with tarfile.open(first) as bundle:
            for member in bundle:
                target = extracted / member.name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(bundle.extractfile(member).read())
        script = extracted / "nestworth-skill/install.sh"
        self.run_installer(script=script)
        self.assertEqual((self.target / "VERSION").read_text().strip(), SOURCE_VERSION)

    def test_release_download_selects_tag_and_checks_hash(self):
        archive = self.archive()
        fake_bin = self.root / "bin"
        fake_bin.mkdir()
        fake_curl = fake_bin / "curl"
        fake_curl.write_text('''#!/bin/bash
set -eu
out='' url=''
while [ "$#" -gt 0 ]; do
 case "$1" in -o) out=$2; shift 2;; https:*) url=$1; shift;; *) shift;; esac
done
printf '%s\\n' "$url" >> "$NESTWORTH_TEST_DOWNLOAD_LOG"
case "$url" in *.sha256) cp "$NESTWORTH_TEST_ARCHIVE.sha256" "$out";; *) cp "$NESTWORTH_TEST_ARCHIVE" "$out";; esac
''')
        fake_curl.chmod(0o755)
        log = self.root / "downloads"
        environment = {**os.environ, "PATH": str(fake_bin) + os.pathsep + os.environ["PATH"],
                       "NESTWORTH_TEST_ARCHIVE": str(archive), "NESTWORTH_TEST_DOWNLOAD_LOG": str(log)}
        self.run_installer("--version", "v9.8.7", env=environment)
        self.assertIn("/releases/download/v9.8.7/nestworth-skill.tar.gz", log.read_text())
        self.assertEqual(len(log.read_text().splitlines()), 2)
        self.run_installer("--version", "../invalid", env=environment, ok=False)
        self.assertEqual(len(log.read_text().splitlines()), 2)


if __name__ == "__main__":
    unittest.main()
