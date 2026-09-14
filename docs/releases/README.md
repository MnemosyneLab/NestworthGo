# Release Documents

The current development line is `v0.3.3` / build `4`. The release contract
below is the single maintained scope and acceptance document for this line,
including the evidence boundary for gates that were not run locally.

## Current release

- [v0.3.3 release contract](v0.3.3.md) — runtime, product scope, Market Data
  entry flow, validation, packaging, and distribution gates.

## Published history

- [v0.3.2 release contract](v0.3.2.md) — first public release, including
  runtime, product scope, backup/restore, CSV portability, validation,
  packaging, and public-distribution evidence.

## Historical context

Historical release facts and superseded milestones belong in
[`CHANGELOG.md`](../../CHANGELOG.md). Unpublished or superseded implementation
contracts are not maintained as release documents.

## Release documentation rules

- A release document describes code and tests that exist in the current tree,
  or labels future work as planned/deferred.
- Build metadata, `internal/version`, frontend metadata, and artifact names
  must agree before a release is called synchronized.
- Validation records must name the command, environment, and any manual gate
  that was not exercised.
- Historical implementation detail belongs in the changelog or version
  control, not in the current release contract.
