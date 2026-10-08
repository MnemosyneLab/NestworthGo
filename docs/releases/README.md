# Release Documents

The current unreleased candidate is v0.3.7 / build 8. The release contract
below owns its scope, schema compatibility, release evidence, and outstanding
native/distribution gates. The latest published release remains v0.3.6.

## Current release

- [v0.3.7 release contract](v0.3.7.md) — candidate scope, evidence, and open
  acceptance/distribution gates.
- [v0.3.7 release-note draft](v0.3.7-notes.md) — publication text pending
  final gates.

## Published history

- [v0.3.6 release contract](v0.3.6.md) — published 2026-10-03 with Apple
  Silicon DMG/ZIP, checksums, and standalone skill assets; preparation-time
  evidence is retained in the contract. The archived
  [release-note draft](v0.3.6-notes.md) is not the exact published release body.
- [v0.3.5 release contract](v0.3.5.md) — published household liquidity and
  MCP/agent workflows; historical preparation evidence retained.

- [v0.3.4 release contract](v0.3.4.md) — portfolio/holdings, settings, insights,
  JSON export, validation, packaging, and distribution gates.
- [v0.3.3 release contract](v0.3.3.md) — market-data providers, metals/crypto,
  schema compatibility, validation, packaging, and distribution gates.
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
