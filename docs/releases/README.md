# Release Documents

The latest published release is [v0.3.8 / build 9](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.8),
published 2026-10-14. Its Apple Silicon arm64 DMG/ZIP, checksums, and
standalone skill 1.9.0 bundle are available on GitHub Releases. Package
metadata, signatures, payload contents, and checksums were verified without
launching the app. Native GUI/functional, archive-launch/household, schema
recovery, test-ledger, and external-client acceptance remain owner-deferred and
are not claimed as passed. The app is ad-hoc signed and not notarized.

## Current published release

- [v0.3.8 release record](v0.3.8.md) — published scope, exact source/tag
  identity, package verification, and deferred acceptance boundaries.
- [v0.3.8 release notes](v0.3.8-notes.md) — published product summary and
  upgrade guidance.
- [v0.3.8 Mac package QA](../qa/v0.3.8-release-package-2026-10-14.md) — exact
  toolchain, non-interactive verification, five asset hashes, signature status,
  and owner-deferred acceptance.

## Previous published release

- [v0.3.7 release contract](v0.3.7.md) — published scope, acceptance evidence,
  and remaining GUI/signing boundaries.
- [v0.3.7 release notes](v0.3.7-notes.md) — release summary.

## Published history

- [v0.3.8 release](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.8)
  — published 2026-10-14 at source `fc1b3a9` (build 9), with arm64 DMG/ZIP,
  checksums, and skill 1.9.0 assets. Package and acceptance boundaries are
  recorded in the [release record](v0.3.8.md) and
  [dated QA report](../qa/v0.3.8-release-package-2026-10-14.md).

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
