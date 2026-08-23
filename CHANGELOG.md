# Changelog

All notable changes to the Go + Fyne Nestworth application are recorded here.
The project has not published a public release yet.

## [Unreleased] — v0.1.2

### Added

- Added exact decimal Money, Currency, Category, TrackingMode, Ownership, and typed UUID domain contracts.
- Added local SQLite schema bootstrap, migration versioning, integrity checks, foreign-key enforcement, and unsupported-future zero-write blocking.
- Added atomic Household onboarding with Members, Institutions, Groups, archive semantics, and retained references.
- Added Account creation, exact ownership, append-only current values, archive workflow, base-currency enforcement, and deterministic latest-value reads.
- Added backend-authoritative Overview totals and allocation by Category, Member, Institution, and Group.
- Added live Fyne onboarding, Overview, Accounts, Members, Institutions, and Groups pages with English, Simplified Chinese, and existing Traditional Chinese fallback coverage.
- Added bounded local image normalization and Household-scoped avatar/logo attachment workflows.
- Added multi-currency Accounts, Instruments, Holdings, cash observations,
  append-only quotes, FX preferences, authoritative portfolio valuation, and
  incomplete-value diagnostics.
- Added explicit Yahoo current-price refresh with safe normalization, partial
  persistence, cancellation, retry, and provider failure handling.
- Added Settings-routed FX provider selection with Yahoo Finance as the default
  and Frankfurter as an FX-only daily-rate provider.
- Added Investments views, allocation summaries, provider disclaimers, and
  English/Simplified Chinese refresh and provider-routing states.

### Verification

- `go test ./...`, targeted `go test -race`, `go vet ./...`, `go build
  ./cmd/nestworth`, and `gofmt -l cmd internal` pass on macOS arm64.

### Deferred

- Remaining Phase 10 release closeout: isolated desktop launch smoke tests,
  keyboard and VoiceOver review, signing/notarization policy, and final
  artifact retention/publication. The unsigned arm64 `.app` and UDZO DMG were
  built locally after synchronizing v0.1.2/build 1 metadata.
- Activity, History, Analytics, Backup/Restore, Import/Export, sync, and
  background refresh remain outside v0.1.2.
