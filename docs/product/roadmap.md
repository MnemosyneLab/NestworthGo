# Product Roadmap

## Authority

The current application and tests define what exists. This roadmap records the
order in which user value and operational safety should improve; a roadmap
entry is not an implementation claim.

## 0.2.1 — Account container model

Status: `In progress`.

This line keeps the Wails v3 local-first desktop and replaces Account
categories with `account_type`, `balance_sheet_role`, and `tracking_mode` on
schema `7`. Closeout priorities are:

- legal type/role/tracking combinations on create and update;
- Overview `assetsByType` / `liabilitiesByType` at component granularity;
- whole-account `include_in_portfolio`;
- reject incompatible databases, including schema `6`, without rewriting them.

## 0.2.0 — Coherent local-first desktop

Status: `Superseded`. The unpublished `0.2.0` Wails v3 closeout is replaced by
`0.2.1`.

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
