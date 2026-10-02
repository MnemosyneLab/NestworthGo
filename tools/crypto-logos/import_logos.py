"""Import only four reviewed static brand SVGs from the pinned upstream checkout."""
import hashlib
import json
import math
from pathlib import Path
import re
import subprocess
import sys
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
REVISION = '1a63530be6e374711a8554f31b17e4cb92c25fa5'
SYMBOLS = ('btc', 'eth', 'sol', 'usdc')
SVG = 'http://www.w3.org/2000/svg'


def validate(raw):
    if re.search(r'<!|<\?', raw):
        raise ValueError('declarations and entities forbidden')
    root = ET.fromstring(raw)
    if root.tag != f'{{{SVG}}}svg' or root.get('viewBox') != '0 0 32 32':
        raise ValueError('unexpected root/viewBox')
    for node in root.iter():
        tag = node.tag.removeprefix(f'{{{SVG}}}')
        attrs = {'svg': {'viewBox', 'width', 'height'}, 'g': {'fill', 'fill-rule'},
                 'circle': {'cx', 'cy', 'r', 'fill'},
                 'path': {'d', 'fill', 'fill-rule', 'fill-opacity'}}
        if tag not in attrs or node.tag != f'{{{SVG}}}{tag}' or (tag == 'svg' and node is not root):
            raise ValueError('unsupported element')
        if set(node.attrib) - attrs[tag] or (node.text or '').strip() or (node.tail or '').strip():
            raise ValueError('unsupported content')
        for key, value in node.attrib.items():
            if key == 'fill' and not re.fullmatch(r'none|#[0-9a-fA-F]{3}(?:[0-9a-fA-F]{3})?', value):
                raise ValueError('external or unsupported paint')
            if key == 'fill-rule' and value not in ('nonzero', 'evenodd'):
                raise ValueError('unsupported fill rule')
            if key in ('cx', 'cy', 'r', 'width', 'height', 'fill-opacity'):
                number = float(value)
                if not math.isfinite(number) or number < 0 or (key == 'fill-opacity' and number > 1):
                    raise ValueError('invalid number')
            if key == 'd' and not re.fullmatch(r'[MmZzLlHhVvCcSsQqTtAa\d\s.,+eE-]+', value):
                raise ValueError('invalid path')
    return raw


def generate(source):
    if subprocess.check_output(['git', '-C', str(source), 'rev-parse', 'HEAD'], text=True).strip() != REVISION:
        raise ValueError('wrong upstream revision')
    if subprocess.check_output(['git', '-C', str(source), 'status', '--porcelain'], text=True).strip():
        raise ValueError('upstream checkout must be clean')
    output = ROOT / 'frontend/public/crypto-logos'
    assets = []
    for symbol in SYMBOLS:
        relative = f'svg/color/{symbol}.svg'
        raw = (source / relative).read_bytes()
        validate(raw.decode())
        (output / f'{symbol}.svg').write_bytes(raw)
        assets.append({'key': f'crypto-logo:{symbol}', 'source': relative, 'file': f'{symbol}.svg', 'sha256': hashlib.sha256(raw).hexdigest()})
    (output / 'LICENSE.md').write_bytes((source / 'LICENSE.md').read_bytes())
    manifest = {'repository': 'https://github.com/spothq/cryptocurrency-icons', 'revision': REVISION, 'license': 'CC0-1.0', 'assets': assets}
    (ROOT / 'tools/crypto-logos/manifest.json').write_text(json.dumps(manifest, indent=2)+'\n')


if __name__ == '__main__':
    generate(Path(sys.argv[1]))
