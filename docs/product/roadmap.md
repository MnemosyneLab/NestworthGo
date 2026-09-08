# Product Roadmap

## Authority

The current application and tests define what exists. This roadmap records the
order in which user value and operational safety should improve; a roadmap
entry is not an implementation claim.

## 0.3.1 — Insights: Return Analysis and Asset Changes

Status: `In progress` (code and automated tests for the analysis kernel and
six Insights tabs; Wails desktop smoke and named manual gates still pending).

This line carries forward the local-first Wails v3 desktop and schema `9`, and
replaces the old Analysis dashboard with Return Analysis and Asset Changes:
scoped universes, one-pass classification, signed Asset Changes identity,
path-aware Price/FX, Dietz linking, memoized Wails projections, shared
filters, History deep-links, residual tolerance, and Origin-timezone
boundaries.

## 0.3.0 — Backup, restore, and CSV portability

Status: `Superseded` by `0.3.1`.

This line keeps the Wails v3 local-first desktop and schema `9`, and adds
recoverable local backup/restore plus create-only Accounts/Holdings CSV
exchange. Closeout priorities are:

- verified `.nestworth-backup` packages and journaled restore with quit/relaunch;
- blocked-startup restore without an open business session;
- CSV preview, mapping, and all-or-nothing create-only import.

## 0.2.1 — Account container model

Status: `Superseded`. The unpublished `0.2.1` Account-container closeout is
replaced by `0.3.0`.

## 0.2.0 — Coherent local-first desktop

Status: `Superseded`. The unpublished `0.2.0` Wails v3 closeout is replaced by
`0.2.1`.

## Next product increments

These are planned, not part of the current release:

1. [Monthly review and data confidence](../design/monthly-review-and-data-confidence.md):
   one local loop to surface data issues, confirm as-of balances and holdings,
   save the review, and notice when later history invalidates it.
2. Maintenance: search, saved views, and clearer audit/history workflows as
   the local data set grows.
3. Integrations: optional direct connections only after the local model,
   privacy boundary, and recovery story are strong enough to support them.
4. Encrypted backup as a separate security design.

## Long-term direction

Synchronization, multi-device operation, background refresh, tax reporting,
budgeting, and multi-user permissions require separate product and security
contracts. They must not be introduced by implication through a provider,
backup, or import feature.

## Dependency rules

- Data safety precedes synchronization.
- Import validates and previews before it writes.
- Automation proposes facts; it does not silently post financial changes.
- Optional integrations remain replaceable infrastructure.
- Historical and financial definitions remain stable when presentation
  changes.
