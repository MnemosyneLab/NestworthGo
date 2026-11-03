#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
export GOWORK=off
export TMPDIR=/tmp
go test -count=1 -timeout=120s -v ./...
go vet ./...
go list -m all
