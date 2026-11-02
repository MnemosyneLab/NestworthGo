#!/usr/bin/env python3
"""Public CI tool versions, derived exclusively from the project's go.mod."""
import argparse
import json
from pathlib import Path
import re


def versions(text):
    go = re.findall(r'^go (\d+\.\d+\.\d+)\s*$', text, re.M)
    wails = re.findall(r'^\s*github\.com/wailsapp/wails/v3 (v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)\s*$', text, re.M)
    if len(go) != 1 or len(wails) != 1:
        raise ValueError('go.mod must pin one patch-level Go version and one Wails v3 version')
    if re.search(r'^toolchain\s', text, re.M) or re.search(r'^\s*(?:replace\s+)?github\.com/wailsapp/wails/v3[^\n]*=>', text, re.M):
        raise ValueError('toolchain directives and Wails replacements need explicit image support')
    return {'go': go[0], 'wails': wails[0]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--json', action='store_true')
    parser.add_argument('--go-mod', type=Path, default=Path(__file__).resolve().parents[1] / 'go.mod')
    args = parser.parse_args()
    try:
        result = versions(args.go_mod.read_text())
    except (OSError, ValueError) as error:
        raise SystemExit(str(error)) from error
    print(json.dumps(result) if args.json else result['go'] + ' ' + result['wails'])


if __name__ == '__main__':
    main()
