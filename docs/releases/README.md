# Release Documents

The latest published release is v0.3.7 / build 8. The release contract below
records its scope, schema compatibility, release evidence, and remaining native
GUI/distribution boundaries.

## Latest release

- [v0.3.7 release contract](v0.3.7.md) — published scope, acceptance evidence,
  and remaining GUI/signing boundaries.
- [v0.3.7 release notes](v0.3.7-notes.md) — release summary.

## Published history

- [v0.3.7 release](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.7)
  — published 2026-10-09 at `4d72b3d` (build 8), with arm64 DMG/ZIP, checksums,
  and skill 1.6.0 assets. Preparation and package acceptance evidence is in
  the [release contract](v0.3.7.md).
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
