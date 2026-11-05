# Release Documents

The latest published release is [v0.3.10 / build 11](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.10),
published 2026-10-28 from `74558eea202ddd054fffb43b73b217115a6a8883`.
Its Apple Silicon arm64 DMG/ZIP, checksums and standalone skill **2.0.0** are
available. It requires macOS 27.0 or newer and supports verified database
schemas 21, 22 and 23, with ordered additive upgrades to schema23; v0.3.8's
schema16 cannot be opened directly.

The published GitHub release records native package and archive startup/restart
checks, ad-hoc signing, no notarization, Gatekeeper rejection and the known
scheduled race-shard timeout. Remaining native GUI/IPC, reconciliation interaction,
archive onboarding, keyboard/accessibility and normal-menu Quit acceptance were
owner-waived; those gates were not passed. See the published release for the
exact evidence and limits.

The current repository skill is **3.0.1 (unpublished)**. It is separate from the
published v0.3.10 skill 2.0.0 and does not replace any release asset. See the
[skill guide](../user/nestworth-skill.md#skill-version-policy) for version policy.

## Current published release

- [v0.3.10 published release](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.10) — authoritative publication, source identity, assets, compatibility and acceptance evidence.
- [v0.3.10 prepared release notes](v0.3.10-notes.md) — retained preparation summary; the published GitHub body records final validation and waivers.

## Previous published releases

- [v0.3.9 release record](v0.3.9.md) — historical source/tag identity, scope, compatibility and acceptance; shipped skill 1.11.0.
- [v0.3.9 release notes](v0.3.9-notes.md) — historical published summary and upgrade limits.
- [v0.3.9 Mac package QA](../qa/v0.3.9-release-package-2026-10-21.md) — historical environment, checks and asset hashes.
- [v0.3.8 release record](v0.3.8.md) — historical scope and acceptance.
- [v0.3.8 release notes](v0.3.8-notes.md) — historical summary and upgrade guidance.
- [v0.3.8 Mac package QA](../qa/v0.3.8-release-package-2026-10-14.md) — historical package evidence.

## Published history

- [v0.3.10 release](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.10)
  — published 2026-10-28 at `74558eea` (build 11), with arm64 DMG/ZIP,
  checksums and skill 2.0.0. The published GitHub body records final validation
  and acceptance limits; [prepared notes](v0.3.10-notes.md) remain historical.

- [v0.3.9 release](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.9)
  — published 2026-10-21 at `57ddf33f` (build 10), with arm64 DMG/ZIP,
  checksums and skill 1.11.0. See the [release record](v0.3.9.md) and
  [dated QA](../qa/v0.3.9-release-package-2026-10-21.md).

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
