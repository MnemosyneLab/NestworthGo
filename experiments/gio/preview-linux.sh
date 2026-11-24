#!/bin/sh
set -eu

if [ "$(uname -s)" != Linux ]; then
	printf 'Run this launcher on Linux.\n' >&2
	exit 1
fi
here=$(CDPATH='' cd "$(dirname "$0")" && pwd -P)
exec python3 "$here/packaging/preview.py" "$@"
