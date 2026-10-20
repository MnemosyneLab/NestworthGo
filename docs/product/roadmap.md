# Product Roadmap

## Authority

The application source and tests define current behavior. The latest published
release is [v0.3.8](../releases/v0.3.8.md); current main identifies itself in
[local workflow](../development/local-workflow.md). Release records own shipped
scope and acceptance evidence. This page describes future directions only; it
is not a delivery commitment.

## Planned design

- [Monthly review and data confidence](../design/monthly-review-and-data-confidence.md): a local month-end review of data issues, as-of balances and holdings, with later-history invalidation. Design only; not implemented.
- Improve maintenance workflows as local data grows, including search, saved views, and clearer audit/history navigation. No scoped feature or schedule is approved.
- Consider optional integrations only after the local model, privacy boundary, and recovery story are defined.
- Treat encrypted backup as a separate security design.

## Deferred direction

Synchronization, multi-device operation, background refresh, tax reporting,
budgeting, and multi-user permissions need separate product and security
contracts. Their mention here does not imply implementation or commitment.

## Product constraints

- Data safety precedes synchronization.
- Imports validate and preview before writing.
- Automation proposes facts; it does not silently post financial changes.
- Optional integrations remain replaceable infrastructure.
- Presentation changes preserve established historical and financial definitions.
