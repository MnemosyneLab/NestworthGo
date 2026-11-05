#!/usr/bin/env python3
"""Verify selected-source package identities and record downloadable provenance."""
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[1]


def manifest(root, sha, ref, toolchain):
    source = (root / 'internal/version/version.go').read_text()
    version = re.search(r'\bVersion\s*=\s*"v(\d+\.\d+\.\d+)"', source).group(1)
    build = re.search(r'\bBuild\s*=\s*"(\d+)"', source).group(1)
    skill = (root / 'skills/nestworth/VERSION').read_text().strip()
    app = root / 'dist/macos/Nestworth.app/Contents/Info.plist'
    with app.open('rb') as stream:
        identity = plistlib.load(stream)
    expected = dict(CFBundleIdentifier='com.nestworth.app',
                    CFBundleShortVersionString=version, CFBundleVersion=build)
    for key, value in expected.items():
        if identity.get(key) != value:
            raise ValueError(f'Packaged {key} differs from selected source')
    archive = root / 'dist/skills/nestworth-skill.tar.gz'
    with tarfile.open(archive, 'r:gz') as bundle:
        entry = bundle.getmember('nestworth-skill/skills/nestworth/VERSION')
        if not entry.isfile() or bundle.extractfile(entry).read().decode().strip() != skill:
            raise ValueError('Packaged skill version differs from selected source')
    paths = [f'dist/macos/Nestworth-{version}-arm64.{extension}' for extension in ('dmg', 'zip')]
    paths += ['dist/macos/SHA256SUMS', 'dist/skills/nestworth-skill.tar.gz',
              'dist/skills/nestworth-skill.tar.gz.sha256']
    artifacts = []
    for relative in paths:
        path = root / relative
        if path.is_symlink() or not path.is_file() or path.stat().st_size == 0:
            raise ValueError(f'Missing or invalid package file: {relative}')
        digest = hashlib.sha256()
        with path.open('rb') as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b''):
                digest.update(chunk)
        artifacts.append(dict(path=relative, bytes=path.stat().st_size, sha256=digest.hexdigest()))
    return dict(source_ref=ref, source_sha=sha, app_version=version, app_build=build,
                skill_version=skill, architecture='arm64', toolchain=toolchain,
                signing='ad-hoc only; no Developer ID or notarization', artifacts=artifacts)


def main():
    sha = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    if sha != os.environ['GITHUB_SHA'] or os.environ['GITHUB_REF_TYPE'] != 'branch':
        raise SystemExit('Checkout must match the manually selected branch commit')
    environment = Path(os.environ['RUNNER_TEMP']) / 'macos-environment.json'
    report = manifest(ROOT, sha, os.environ['GITHUB_REF'], json.loads(environment.read_text()))
    (ROOT / 'dist/package-manifest.json').write_text(json.dumps(report, indent=2) + '\n')
    with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as stream:
        stream.write('Built and verified DMG, ZIP and standalone skill. Download the run artifact.\n\n')
        # JSON code block keeps branch names as data rather than Markdown links.
        stream.write('```json\n' + json.dumps(report, indent=2) + '\n```\n')


if __name__ == '__main__':
    main()
