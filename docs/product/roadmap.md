# Product Roadmap

## Authority

The current application and tests define what exists. This roadmap records the
order in which user value and operational safety should improve; a roadmap
entry is not an implementation claim.

## 0.2.0 — Coherent local-first desktop

Status: `In progress`.

This line consolidates the current Wails v3 desktop shell, the React product
surface, the local SQLite data model, and the financial correctness contracts
into one maintainable release. Its closeout priorities are:

- stable onboarding, portfolio, history, analytics, and settings flows;
- exact decimal calculations and backend-authoritative read models;
- deterministic provider failure, offline, and incomplete-data states;
- accessible keyboard paths and complete localized UI states;
- reproducible build metadata and a documented release gate.

## Next product increments

These are planned, not part of the current release:

1. Data safety: recoverable local backup/restore with explicit validation and
   no accidental overwrite.
2. Controlled exchange: import/export with preview, validation, and one
   atomic commit.
3. Maintenance: search, saved views, and clearer audit/history workflows as
   the local data set grows.
4. Integrations: optional direct connections only after the local model,
   privacy boundary, and recovery story are strong enough to support them.

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
