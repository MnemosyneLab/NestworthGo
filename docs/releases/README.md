# Release Documents

The current release line is `0.2.1`. The release contract below is the single
maintained scope and acceptance document for this line.

## Current release

- [v0.2.1 release contract](v0.2.1.md) — runtime, product scope, Account model
  cutover, validation, packaging, and public-distribution gates.

## Historical

- [v0.2.0 release contract](v0.2.0.md) — superseded unreleased Wails v3
  closeout line.

## Release documentation rules

- A release document describes code and tests that exist in the current tree,
  or labels future work as planned/deferred.
- Build metadata, `internal/version`, frontend metadata, and artifact names
  must agree before a release is called synchronized.
- Validation records must name the command, environment, and any manual gate
  that was not exercised.
- Historical implementation detail belongs in the changelog or version
  control, not in the current release contract.
