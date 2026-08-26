# Release Documents

The current release line is `0.2.0`. The release contract below is the single
maintained scope and acceptance document for this line.

## Current release

- [v0.2.0 release contract](v0.2.0.md) — runtime, product scope, validation,
  packaging, and public-distribution gates.
- [Linux v0.2.0 manual test](../qa/linux-v0.2.0/report.md) — Ubuntu 24.04
  production build and first-run GUI evidence (members, institutions,
  accounts, investments, history).

## Release documentation rules

- A release document describes code and tests that exist in the current tree,
  or labels future work as planned/deferred.
- Build metadata, `internal/version`, frontend metadata, and artifact names
  must agree before a release is called synchronized.
- Validation records must name the command, environment, and any manual gate
  that was not exercised.
- Historical implementation detail belongs in the changelog or version
  control, not in the current release contract.
