# Product Roadmap

## Authority

The application source and tests define current behavior. The latest published
release is [v0.3.10](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.10); current main identifies itself in
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

## Uncommitted exploration

Earlier brainstorms are consolidated here as questions, not a prioritized backlog:

- Scenario sandbox: hypothetical expenses, income, transfers and life changes,
  isolated from the real ledger with visible assumptions.
- Purpose allocations: goals and unallocated funds without double counting;
  emergency-fund months would need a separate definition. Existing reservations
  and liquidity estimates are covered by [Available Funds](../design/available-funds.md).
- Statement/screenshot drafts: extract and match facts, show a reviewable diff,
  and leave unexplained differences unresolved. Existing [reconciliation](../user/account-reconciliation.md)
  does not imply automatic document ingestion.
- Life-event labels and an investment decision journal: dated rationale,
  costs, outstanding reimbursements and user-selected review dates.
- Allocation limits, hypothetical rebalancing, comparison periods and benchmarks:
  define scope and denominators before adding percentages. No trade execution.
- Private calendar and monthly summary: distinguish expected events from posted
  activity; existing [upcoming actions](../user/upcoming-actions.md) and the
  [monthly-review proposal](../design/monthly-review-and-data-confidence.md)
  own their narrower scopes.
- Search/saved analysis views, command navigation, report exports, corporate-action
  clues, bond-specific models and linked property/debt/insurance need explicit
  requirements. Member-specific returns and tax lots need separate financial
  definitions. None is a commitment implied by the retired numbered idea list.

Current feature behavior belongs in the design/user guides, not in a duplicate
idea-status table. Select future work from an observed user need and a scoped
financial/privacy contract.
