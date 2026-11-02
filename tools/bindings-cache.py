#!/usr/bin/env python3
"""Conservative CI-only cache for generated Wails bindings.

The input is the complete checked-out source inventory, not an inferred API
closure. Unsupported external Go inputs disable reuse. This is an integrity
check, not an authentication mechanism for untrusted cache writers.
"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
BINDINGS = ROOT / 'frontend/bindings'
MARKER = 'github.com/waltwang/nestworth-go/internal/wailsapi/app/index.ts'
VERSION = 1
GO_ENV = ('GOOS', 'GOARCH', 'CGO_ENABLED', 'GOEXPERIMENT', 'GOAMD64', 'GOARM64',
          'GOARM', 'GO386', 'GOMIPS', 'GOMIPS64', 'GOPPC64', 'GORISCV64',
          'GOWASM', 'GOFLAGS', 'GOTOOLCHAIN', 'CGO_CFLAGS', 'CGO_CPPFLAGS',
          'CGO_CXXFLAGS', 'CGO_FFLAGS', 'CGO_LDFLAGS', 'CC', 'CXX', 'PKG_CONFIG')


def digest(data):
    return hashlib.sha256(data).hexdigest()


def command(*args, cwd=ROOT):
    return subprocess.check_output(args, cwd=cwd, stderr=subprocess.PIPE)


def files(root):
    result = {}
    if not root.is_dir() or root.is_symlink():
        raise ValueError('missing or invalid directory')
    for path in sorted(root.rglob('*')):
        if path.is_symlink():
            raise ValueError('symlink in output')
        if path.is_file():
            result[path.relative_to(root).as_posix()] = digest(path.read_bytes())
        elif not path.is_dir():
            raise ValueError('nonregular output')
    return result


def input_key(root=ROOT):
    # go.work and local replacements can reference files outside the inventory.
    goenv = json.loads(command('go', 'env', '-json', *GO_ENV, 'GOWORK', cwd=root))
    if goenv.get('GOWORK') or goenv.get('GOFLAGS'):
        raise ValueError('go.work or GOFLAGS require fresh generation')
    module = json.loads(command('go', 'mod', 'edit', '-json', cwd=root))
    if any(not replacement.get('New', {}).get('Version') for replacement in module.get('Replace', [])):
        raise ValueError('local module replacement requires fresh generation')
    ignored_go = command('git', 'ls-files', '-z', '--others', '--ignored',
                         '--exclude-standard', '--', '*.go', 'go.work', 'go.work.sum', cwd=root)
    if ignored_go:
        raise ValueError('ignored Go inputs require fresh generation')
    names = command('git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard', cwd=root).split(b'\0')
    inventory = {}
    for raw in names:
        if not raw:
            continue
        name = os.fsdecode(raw)
        path = root / name
        # Missing tracked files, symlinks and nonregular inputs fail closed.
        if path.is_symlink() or not path.is_file():
            raise ValueError('unsupported source file: ' + name)
        inventory[name] = {'sha256': digest(path.read_bytes()), 'mode': path.stat().st_mode & 0o777}
    cli = shutil.which('wails3')
    if not cli:
        raise ValueError('pinned CLI unavailable')
    if subprocess.check_output([cli, 'version'], cwd=root, stderr=subprocess.STDOUT).decode().strip() != 'v3.0.0':
        raise ValueError('unexpected CLI version')
    payload = {'version': VERSION, 'files': inventory, 'go_env': goenv,
               'go_version': command('go', 'version', cwd=root).decode().strip(),
               'node_version': command('node', '--version', cwd=root).decode().strip(),
               'environment': {k: v for k, v in os.environ.items()
                               if k.startswith(('GO', 'CGO_', 'WAILS_'))
                               or k in ('CC', 'CXX', 'PKG_CONFIG', 'SDKROOT',
                                        'MACOSX_DEPLOYMENT_TARGET', 'SOURCE_DATE_EPOCH',
                                        'LANG', 'LC_ALL', 'TZ')},
               'cli': digest(Path(cli).read_bytes())}
    return digest(json.dumps(payload, sort_keys=True, separators=(',', ':')).encode())


def validate(cache, key):
    manifest = json.loads((cache / 'manifest.json').read_text())
    actual = files(cache / 'bindings')
    return (manifest == {'version': VERSION, 'input': key, 'files': actual}
            and MARKER in actual and bool(actual))


def save(cache, key, bindings=BINDINGS):
    inventory = files(bindings)
    if MARKER not in inventory:
        raise ValueError('missing generated service marker')
    if cache.exists():
        shutil.rmtree(cache)
    cache.mkdir(parents=True)
    shutil.copytree(bindings, cache / 'bindings')
    (cache / 'manifest.json').write_text(json.dumps({'version': VERSION, 'input': key, 'files': inventory}, sort_keys=True))


def main():
    action = sys.argv[1]
    if action == 'key':
        try:
            print(input_key())
        except (OSError, ValueError, subprocess.SubprocessError, json.JSONDecodeError) as error:
            print('disabled', file=sys.stdout)
            print(f'Bindings cache disabled: {error}', file=sys.stderr)
        return
    cache, key = Path(sys.argv[2]), sys.argv[3]
    if key == 'disabled':
        raise ValueError('disabled input')
    if action == 'restore':
        if not validate(cache, key):
            raise ValueError('cache input or output mismatch')
        if BINDINGS.exists():
            shutil.rmtree(BINDINGS)
        shutil.copytree(cache / 'bindings', BINDINGS)
    elif action == 'save':
        # Verify the inputs have not changed during generation.
        if input_key() != key:
            raise ValueError('inputs changed during generation')
        save(cache, key)
    else:
        raise ValueError('unknown action')


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, subprocess.SubprocessError, json.JSONDecodeError) as error:
        print(f'Bindings cache: {error}', file=sys.stderr)
        sys.exit(1)
