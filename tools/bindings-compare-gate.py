#!/usr/bin/env python3
"""Select an independent regeneration for contract/tool changes; fail closed."""
import os
import subprocess


def requires_comparison(names):
    for name in names:
        if name.endswith('.go'):
            return True
        # Only presentation and documentation changes are exempt. Frontend
        # generator/runtime inputs are not presentation changes.
        if name.startswith('docs/') or name.endswith('.md'):
            continue
        if name.startswith('frontend/src/') and name.endswith(('.ts', '.tsx', '.css', '.svg', '.png', '.webp')):
            continue
        return True
    return False


def main():
    base, head = os.getenv('BASE_SHA', ''), os.getenv('HEAD_SHA', '')
    compare = True
    if base and head and set(base) != {'0'}:
        try:
            output = subprocess.check_output(['git', 'diff', '--no-renames', '--name-only', '-z', base, head])
            compare = requires_comparison([os.fsdecode(n) for n in output.split(b'\0') if n])
        except subprocess.SubprocessError:
            pass
    print('compare=' + str(compare).lower())


if __name__ == '__main__':
    main()
