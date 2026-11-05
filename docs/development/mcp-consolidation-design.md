# MCP interface consolidation: design note (Phase 3)

Status: items 1 (icon merge), 2 (PATCH audit, see [mcp-update-conventions.md](mcp-update-conventions.md)),
3 (`commit_plan`) and 5 (compact output schemas) are implemented in this branch;
[mcp-changelog.md](mcp-changelog.md#phase-3-icon-merge-commit_plan-and-compact-output-schemas) has the
measurements. Item 4 (financial read merges) is still design only. Phases 1 and 2
(alias removal, prose slimming, byte-budgeted inlining, `delivery`) are described
in [mcp-changelog.md](mcp-changelog.md#discovery-size-before-the-phase-3-schema-cut) and
[mcp-pagination.md](mcp-pagination.md). Current ceilings are in
[mcp.md](mcp.md#discovery-budgets).

Where the implementation departs from the recommendations below (the sections
keep the original analysis):

- Item 3 recommended stopping after plan-kind verification. The decision was
  made to unify all ten commit tools. The risk analysis still holds and shaped the
  implementation: the kind is read from the stored plan, never from the caller;
  each kind keeps its own recoverable/non-recoverable receipt rule and its
  historical receipt tool name (so hashes, `get_operation` and the host-review
  binding are unchanged); pruned plans are routed from receipts.
- Item 5 recommended waiting for consumer evidence. The built-in AI adapter,
  skill examples and tests do not depend on the removed detail, so the wire schema
  was made compact and the strict schema moved into a test oracle. The cost is that
  clients validating `structuredContent` and the server's own output validation no
  longer enforce field types or the summary/page exclusion.
- Item 1 is implemented as proposed: one SQL statement writes the supplied
  name and icon.

## Principles

The goal is a better agent experience, not a smaller tool count. A merge is only
worth its cost when it removes a real decision or round trip for the agent
without hiding a different permission, version check, idempotency rule or
unknown-outcome path behind a larger argument shape. Every item below lists what
must be preserved, and the discovery contract snapshot
(`testdata/discovery-contract.json`) turns each change into a reviewed diff.

Surface before Phase 3 (ledger_write): 102 tools; now 90. Of these, 10 are `commit_*`, 12 are
`preview_*`, 3 are `set_*_icon`, 10 are `update_*`, and about 30 are `get_*`.
Output schemas are the largest discovery cost (about 45 KB per mode, mostly the
`mcp-read/1` envelope repeated per tool), ahead of descriptions (17-30 KB).

## 1. Fold `set_*_icon` into `update_*` (recommended)

Today: `update_member`, `update_institution` and `update_group` take
`{id, name}`; `set_member_icon`, `set_institution_icon` and `set_group_icon`
take `{id, iconKey}`. They are separate tools with separate operations, receipts
and permission checks (both `directory_write`).

Proposal: `update_member|institution|group` accept `{id, name?, iconKey?}` with at
least one field. Remove the three icon tools (-3 tools, about 1 KB of discovery).

Preserve: same permission (`directory_write`), same archived-entity rules, one
operation and one receipt per call. When both fields are present they apply
atomically in one transaction, or the call fails with neither applied; this
needs a new application-service method because the GUI currently exposes two
independent writes. An empty `name` must stay invalid (omission, not empty,
means keep); the schema must say so.

Risks: the GUI services are separate, so a combined atomic write is new
behavior in `internal/application`, not a pure MCP change. Existing clients and
the skill that call `set_*_icon` break; the skill version impact is a PATCH only
if the guidance change is not an incompatible workflow, otherwise MINOR (decide
with the PR).

Decision needed: accept the atomic combined write (recommended), or keep two
sequential non-atomic calls inside the MCP adapter (not recommended: partial
failure semantics).

## 2. PATCH semantics audit (recommended before any merge)

The update tools do not share one convention (the first draft of this section
contained a table that was wrong in two rows; the audited table now lives only in
[mcp-update-conventions.md](mcp-update-conventions.md)).

*Audited; see [mcp-update-conventions.md](mcp-update-conventions.md) for the corrected
table. The audit found that the `update_account` description misstated how fields
are cleared and that `terms.shortName` is an optional exception in an otherwise
full-replacement tool; both are fixed.* The mix of patch, replace and clear conventions is the likeliest source of
agent mistakes (an omitted field silently clears in one tool and is preserved in
another). Proposed audit output, no code change: a table of every writable field
with its omit / empty / null meaning and its version guard, generated from the
registered schemas and checked in a test so a new tool cannot add a fourth
convention unnoticed. Follow-up options after the audit: standardize patch tools
on "omitted = keep, `null` = clear" (a breaking schema change, so a separate
decision), and surface the convention in each tool's schema description rather
than prose elsewhere.

## 3. `commit_*` incremental plan (do not merge now)

Ten commit tools exist because plans have different owners:

| Family | Preview / commit | Distinct behavior |
| --- | --- | --- |
| single and batch ledger records | `preview_change`/`commit_change`, `preview_batch`/`commit_batch` | batch is atomic across holdings and receipts; a single-change plan must not be committed as a batch |
| reconciliation, correction | `commit_reconciliation`, `commit_correction` | correction `fix` replays history and returns resulting values at the historical point |
| managed products | `commit_product_operation`, `commit_product_terms`, `commit_product_valuation` | reservation and revision checks; original-ID replay on retry |
| reconciliation session | `commit_reconciliation_session` | session revision binding |
| liquidity policy | `commit_save_liquidity_policy`, `commit_reset_liquidity_policy` | pending or unknown receipts are never replayed |

A single `commit_plan({operationId, planId})` would be possible only if the plan
kind is derived from the stored plan, never from the caller. The risk is the
exact failure modes the separate names prevent today: committing a plan of the
wrong kind, and liquidity-policy recovery differing from managed-product
recovery.

Incremental path if it is pursued:

1. Verify the stored plan `kind` in every commit tool. *Implemented before this
   note was written: `loadPreviewPlan(planID, kind)` already rejects a plan of
   another kind. `TestCommitToolsRejectPlansOfAnotherKind` now guards it across all
   ten commit tools without writing anything.*
2. Introduce `commit_plan` for the families whose commit input is exactly
   `{operationId, planId}` and whose retry semantics are identical (single,
   batch, reconciliation, correction), keeping the other families separate.
3. Only then consider the remainder, with the unknown-outcome tests per family
   passing unchanged.

Preserve: permission (`ledger_write`), version/staleness checks under the shared
write permit, the outer `{operationId, input}` envelope, durable receipts, and
`get_operation` recovery. Not recommended: a unified tool that accepts any plan
and infers the recovery policy from a caller-supplied field.

Expected gain: about 4 tools and 3 KB of descriptions, with real regression risk
in the highest-stakes path. Recommendation: step 1 is done; do not proceed to step 2 now.

## 4. Financial reads: merge by semantics, not by count

The read tools answer different questions with different contracts; a single
"get_financial" tool would need a large discriminated union and would blur
result semantics:

| Question | Tool | Contract |
| --- | --- | --- |
| One-date consistent summary, gaps, evidence | `get_financial_context` (+ `_item`) | frozen package, minimal or named disclosure |
| Two-state difference | `compare_financial_context` | right minus left, never a return |
| Two-state change with return attribution | `compare_financial_attribution` | may write derived snapshots, compatibility link |
| Current GUI overview and account values | `get_overview`, `get_account_valuations`, `get_account_snapshot` | GUI calculations and filters |
| Historical endpoint view and trend | `get_historical_overview`, `get_historical_overview_series` | endpoint movement, not returns |
| Period returns, income and expense | `analyze_period` (+ contribution, return-day, driver tools) | closed period, may refresh snapshots |

Mergeable on semantics:

- `get_historical_overview` and `get_historical_overview_series` share filters,
  views and drill-down; a single tool with `{date, compareTo}` or `{from, to}`
  would work, at the cost of two result shapes under one name. Candidate, low risk.
- `get_account_snapshot` and `get_account_valuations` overlap on current account
  values and should be compared before deciding (open question: is the snapshot
  a single-account projection of the same capture?).
- `get_contribution_item`, `get_return_day`, `get_asset_driver_detail` are
  drill-downs of one report family. They could become `get_read_item` sections of
  the parent capture, which already exists for other families. This removes three
  tools but requires the parent captures (`list_contributions`, `analyze_period`)
  to freeze the drill-down detail, which has memory and TTL costs; measure first.
  Implemented on this branch: the three tools are removed. Return days and asset
  drivers are frozen with `analyze_period` but omitted from the marshaled capture
  so the 4 MiB cap still accepts the ranges it accepted before. See
  [mcp-wave2-c2-measure.md](mcp-wave2-c2-measure.md).

Not merged: context versus comparison versus attribution (different write
behavior and permission-adjacent side effects), and returns versus endpoint
movement (a documented, deliberate semantic boundary).

## 5. Output schema footprint (separate decision)

`readEnvelopeSchema()` is repeated for 28 tools (about 1.2 KB each). Options:

- Keep the schema but shorten it. Possible savings are limited because the JSON
  is already minimal and descriptions are not the cost.
- Drop `oneOf` branches or `items` constraints. This weakens client-side
  validation of results and is a contract change; only acceptable if no
  maintained consumer (built-in AI adapter, skill examples, tests) relies on
  it.
- Move the generic envelope out of per-tool schemas and describe it once in the
  server instructions. Clients that validate would lose validation of `data`.

Recommendation: do not change until it is known which clients actually use
`outputSchema` for validation or for tool-result typing; the measured
potential is roughly 30 KB of discovery per mode.

## Suggested order

1. PATCH audit and its guard test (no behavior change).
2. Server-side plan `kind` verification (safety, no surface change).
3. Icon merge, once the atomic combined write is accepted.
4. Historical overview merge, then drill-down folding, each only after measured
   capture-cost data.
5. Output schema decision.

Each step ships with its own contract-snapshot diff, the unknown-outcome and
permission tests for the affected tools, and a skill PATCH bump when guidance
changes. None of this is implemented until confirmed.
