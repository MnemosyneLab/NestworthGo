# Changelog

All notable changes to the Go + Fyne Nestworth application are recorded here.
The project has not published a public release yet.

## [Unreleased] — v0.1.1

### Added

- Added exact decimal Money, Currency, Category, TrackingMode, Ownership, and typed UUID domain contracts.
- Added local SQLite schema bootstrap, migration versioning, integrity checks, foreign-key enforcement, and unsupported-future zero-write blocking.
- Added atomic Household onboarding with Members, Institutions, Groups, archive semantics, and retained references.
- Added Account creation, exact ownership, append-only current values, archive workflow, base-currency enforcement, and deterministic latest-value reads.
- Added backend-authoritative Overview totals and allocation by Category, Member, Institution, and Group.
- Added live Fyne onboarding, Overview, Accounts, Members, Institutions, and Groups pages with English, Simplified Chinese, and existing Traditional Chinese fallback coverage.
- Added bounded local image normalization and Household-scoped avatar/logo attachment workflows.

### Verification

- `go test ./...`, `go vet ./...`, and `gofmt -l cmd internal` pass on macOS arm64.

### Deferred

- Portfolio, multi-currency, Activity, History, Analytics, Backup/Restore, Import/Export, sync, and provider integrations remain outside v0.1.1.
