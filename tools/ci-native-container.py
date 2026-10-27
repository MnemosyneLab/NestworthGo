#!/usr/bin/env python3
"""Emit only the reviewed image pin; registry credentials stay in job YAML."""
from pathlib import Path
import re
import sys


def image_for(image):
    if image and not re.fullmatch(r'ghcr.io/mnemosynelab/nestworth-ci@sha256:[0-9a-f]{64}', image):
        raise ValueError('CI image must be empty or the approved GHCR image with a SHA256 digest')
    return image


def main():
    pin = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).resolve().parents[1] / '.github/ci-native-image.txt'
    try:
        image = image_for(pin.read_text().strip())
    except (OSError, ValueError) as error:
        raise SystemExit(str(error)) from error
    print('image=' + image)


if __name__ == '__main__':
    main()
