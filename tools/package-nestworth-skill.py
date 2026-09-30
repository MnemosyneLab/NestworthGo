#!/usr/bin/env python3
"""Build a standalone, reproducible user bundle. Python is maintainer-only."""
import argparse
import gzip
import hashlib
import io
from pathlib import Path
import re
import tarfile

ROOT = Path(__file__).resolve().parent.parent
SKILL = ROOT / "skills" / "nestworth"


def payload_files():
    files = {}
    for path in sorted(SKILL.rglob("*")):
        if path.is_symlink() or (not path.is_dir() and not path.is_file()):
            raise ValueError(f"Unsupported payload entry: {path}")
        if path.is_dir():
            continue
        relative = path.relative_to(SKILL).as_posix()
        if not re.fullmatch(r"SKILL\.md|VERSION|agents/openai\.yaml|references/[\w.-]+\.md", relative):
            raise ValueError(f"Unexpected payload file: {relative}")
        files[f"nestworth-skill/skills/nestworth/{relative}"] = path.read_bytes()
    for relative in ("SKILL.md", "VERSION", "agents/openai.yaml"):
        if f"nestworth-skill/skills/nestworth/{relative}" not in files:
            raise ValueError(f"Missing {relative}")
    version = (SKILL / "VERSION").read_text().strip()
    if not re.fullmatch(r"\d+\.\d+\.\d+(?:[.-][A-Za-z0-9.-]+)?", version):
        raise ValueError("Invalid skill version")
    files["nestworth-skill/install.sh"] = (ROOT / "tools/install-nestworth-skill.sh").read_bytes()
    files["nestworth-skill/INSTALL.md"] = (ROOT / "docs/user/nestworth-skill.md").read_bytes()
    return files, version


def package(output: Path):
    files, version = payload_files()
    output.mkdir(parents=True, exist_ok=True)
    archive = output / "nestworth-skill.tar.gz"
    # Zero times/owners and sorted members make checksums reproducible.
    with archive.open("wb") as raw, gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=0) as zipped:
        with tarfile.open(fileobj=zipped, mode="w") as bundle:
            for name, data in sorted(files.items()):
                entry = tarfile.TarInfo(name)
                entry.size, entry.mtime, entry.uid, entry.gid = len(data), 0, 0, 0
                entry.mode = 0o755 if name.endswith("/install.sh") else 0o644
                bundle.addfile(entry, io.BytesIO(data))
    digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    (output / (archive.name + ".sha256")).write_text(f"{digest}  {archive.name}\n")
    print(f"Nestworth skill {version}: {archive}")
    return archive


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / "dist/skills")
    args = parser.parse_args()
    package(args.output)
