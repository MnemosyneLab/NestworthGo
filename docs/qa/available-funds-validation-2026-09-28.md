# Available Funds validation record — 2026-09-28

## Evidence boundary

This is a sanitized summary of the dated Available Funds branch-closeout note
that was consolidated during documentation cleanup. That note did not identify
the tested source commit. Its local logs and screenshots were outside the
repository, so the results below are historical assertions from that note,
not independently verifiable v0.3.5 release evidence. No household database,
settings, credentials, screenshots, or local evidence paths are retained here.

## Reported coverage

The note reported manual review of Available Funds across current and future
dates, source currencies, filters, reservations, and data-health findings. It
also reported adding, editing, and releasing reservations; opening and undoing
a deposit; settling and undoing a deposit receipt; and checking cash/product
totals. The test copy was made from a read-only SQLite backup, with provider
credentials removed. The note reported that the original database remained
unchanged and passed SQLite integrity checks.

The note reported wails3 task check passing, followed by a final frontend run
of 504 tests, a Go race run for the application/domain/SQLite packages, and a
real-data smoke test against an explicitly supplied copy. It did not report a
native desktop-window acceptance, release packaging, or live provider requests.
The local outputs named by the note were not committed, so the detailed
command logs and screenshots are unavailable from this repository.

## Current release status

These historical checks do not close any v0.3.5 gate. Current Go/frontend CI,
the full candidate checks, native arm64 packaging, isolated launch, and
distribution signing status are tracked in the [v0.3.5 release contract](../releases/v0.3.5.md).
