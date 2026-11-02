#!/usr/bin/env python3
"""Emit a GitHub job-container value after validating the reviewed image pin."""
import json
from pathlib import Path
import re
import sys


def container_for(image):
    if not image:
        # The entire container value is an empty string, rather than a mapping
        # with an empty image (which GitHub rejects during workflow validation).
        return ''
    if not re.fullmatch(r'ghcr.io/mnemosynelab/nestworth-ci@sha256:[0-9a-f]{64}', image):
        raise ValueError('CI image must be empty or the approved GHCR image with a SHA256 digest')
    return {'image': image, 'options': '--user 1001:1001'}


def main():
    pin = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).resolve().parents[1] / '.github/ci-native-image.txt'
    try:
        toolchain_pin = pin.with_name('ci-toolchain-image.txt')
        toolchain = toolchain_pin.read_text().strip()
        preinstalled = bool(toolchain)
        # Validate both pins even when the toolchain variant takes precedence.
        container_for(pin.read_text().strip())
        container = container_for(toolchain or pin.read_text().strip())
    except (OSError, ValueError) as error:
        raise SystemExit(str(error)) from error
    print('container=' + json.dumps(container, separators=(',', ':')))
    print('preinstalled=' + str(preinstalled).lower())


if __name__ == '__main__':
    main()
