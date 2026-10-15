# Reconciliation, cost correction and historical mistakes

Read current state or `get_activity` first and choose by intent:

| Intent | Tools | Time/effect |
| --- | --- | --- |
| Current balance, quantity or cost should equal an evidenced target | `preview_reconciliation` / `commit_reconciliation` | Current adjustment; does not recreate past trades |
| A recorded historical transaction was wrong | `preview_correction` / `commit_correction`, action `fix` | Original timestamp, replay later records |
| Reverse a valid prior record now | Same correction tools, action `undo` | Reversal now; original history remains |

## Current targets

`preview_reconciliation` accepts `targets` (1–100 expanded commands maximum).
An account target is a simple balance/manual value or a holdings account's cash
in the specified currency. Submit the **target**, not a self-calculated delta.
Historical statement dates are not accepted by this current-state tool.

<!-- example: balance-target -->
```json
{"targets":[{"accountId":"${accountId}","targetBalance":"1250","currency":"USD"}]}
```

A holding quantity increase needs the original unit cost of the added units;
a decrease preserves average cost and must omit `unitCost`. Neither implies
a purchase/sale or cash movement. Cost-only correction uses the **total cost of
the entire remaining position** in instrument currency:

<!-- example: cost-target -->
```json
{"targets":[{"holdingId":"${holdingId}","totalCost":"300","currency":"USD"}]}
```

Include `targetQuantity` with `totalCost` for a combined quantity/cost target.
Do not supply both `unitCost` and `totalCost`. The preview can expand one target
into two commands; inspect all of them, then `commit_reconciliation` atomically.
Unchanged targets, duplicate endpoints, managed/archived targets and missing or
unrepresentable cost evidence are rejected. Current cost correction changes
subsequent sale/transfer cost, not earlier realized gains.

## Historical fix and current undo

For `preview_correction` with `action:"fix"`, provide `activityId` and a full
`replacement` command without timestamps or mutation IDs. The App preserves the
original time and validates later balances/positions. Trade replacements need
an explicit existing `holdingId`; `position_import` cannot be a replacement.
`preview.resulting` describes the historical replacement state, not today's
balance. Query current state again after committing.

For `action:"undo"`, supply `activityId` and omit replacement. The inverse is
posted at preview time. A later quantity event can block undo because of cost
dependencies; do not silently switch to a historical fix with different effects.
Already corrected/reversed records and managed products can be rejected.

Cost-adjustment records have no generic fix/undo path; submit a new target cost.
For a historical mistake in a buy/sell, fix the historical trade instead.

Use only the matching plan/commit tool. Confirm that a proposed correction
matches the user's intended time/reporting effect, commit when authorized,
then verify the activity links, today's balance and affected analysis/health.
See [recovery.md](recovery.md) if posting succeeded but derived work did not.

## Account-by-account sessions (discover support first)

Session checks are separate from financial target adjustments. Require the
`reconciliation_session_read` capability for saved evidence and
`reconciliation_session_write` / ledger_write for attestations. Read
`list_reconciliation_sessions` before starting another session; offer resume
when a matching in-progress or paused session exists. A session selects a fixed
subset of active accounts. An empty account still needs its completeness check.

Never manufacture a source, statement date/time, reference value, explanation or
confirmation. Numeric equality, an MCP preview and a committed correction are
not user attestations. Ask for missing evidence, or leave the check pending.
Only explicitly confirmed scope and required points at current financial
revisions advance an account's last-success time. A partial session must be
reported with pending/skipped/unresolved counts, never as all passed.

1. `preview_reconciliation_session` action start, then its matching commit.
   Omit accountIds only when the user's intent covers every active account.
2. Read the header with `get_reconciliation_session` (`id`); page accounts and
   items with `get_reconciliation_session_page`. Each row supplies business
   identity and financialRevision. Use `get_reconciliation_item` for complete
   single-point evidence and product terms; page section attempts for its prior
   checks. Page section changes for recorded-order facts since prior success;
   section links is the explicitly linked subset, never inferred from time.
3. If different, ask whether the user wants an audited direct correction or a
   missing real transaction. Keep the existing preview/commit tool pair.
   Attach `reconciliationLink` from the selected current session/account/item
   and revisions. Direct adjustments never fabricate trades. Use dedicated
   managed-product operation/terms/value tools for managed products.
4. Reread after a correction. Outcomes remain pending until the user explicitly
   checks the refreshed evidence. Check with action check, expectedRevision,
   financialRevision and confirmed/skipped/unresolved/pending. Confirmation
   requires source.reference and source.asOfDate. A date-only source must omit
   asOfTime; HH:mm uses the ledger timezone. Actual checked/completion times
   are recorded separately by the App. Skipped/unresolved requires a note.
5. Preview and commit finish. If anything remains outstanding, require explicit
   allowPartial plus the user's closing explanation. Reread immutable results
   and account outcomes. Subsequent edits show recheckRequired without changing
   the original evidence or deleting its historical successful time.

<!-- example: session-start -->
```json
{"action":"start","accountIds":["${accountId}"]}
```

Commit the returned plan with `commit_reconciliation_session`:

<!-- example: session-commit -->
```json
{"operationId":"${newOperationUUID}","input":{"planId":"${sessionPlanId}"}}
```

<!-- example: session-items -->
```json
{"sessionId":"${sessionId}","section":"items","accountId":"${accountId}","limit":25}
```

Alternative A: `preview_reconciliation` (then its existing commit):

<!-- example: session-linked-target -->
```json
{"reconciliationLink":{"sessionId":"${sessionId}","accountId":"${accountId}","itemId":"${itemId}","expectedRevision":3,"financialRevision":12},"targets":[{"accountId":"${accountId}","targetBalance":"1250","currency":"USD","note":"${userProvidedReason}"}]}
```

Alternative B: `preview_change` (then its existing commit), only when the user
has supplied the missing real transaction and its actual time:

<!-- example: session-linked-income -->
```json
{"kind":"money_added","accountId":"${accountId}","amount":"25","currency":"USD","reason":"income","effectiveAt":"${actualRFC3339Time}","reconciliationLink":{"sessionId":"${sessionId}","accountId":"${accountId}","itemId":"${itemId}","expectedRevision":3,"financialRevision":12}}
```

All example revision numbers are illustrative. Copy fresh revisions, never these
numbers. Reread the session and item after committing either route.

<!-- example: session-explicit-check -->
```json
{"action":"check","sessionId":"${sessionId}","expectedRevision":4,"accountId":"${accountId}","itemId":"${itemId}","financialRevision":15,"outcome":"confirmed","source":{"reference":"${userProvidedSource}","asOfDate":"${actualSourceCivilDate}"},"referenceValue":"${userProvidedValue}","note":"${userProvidedNote}"}
```

<!-- example: session-partial-finish -->
```json
{"action":"finish","sessionId":"${sessionId}","expectedRevision":5,"allowPartial":true,"note":"${userProvidedClosingExplanation}"}
```

Use action pause/resume with sessionId and expectedRevision to preserve or
continue work. Financial conflicts require reread and renewed user review;
never recycle an old confirmation against refreshed values. Quote/NAV/FX
freshness is informative and does not invalidate cash/quantity attestations.
For lost responses, retry the exact original operationId/input/plan first;
receipt recovery precedes expiry and session-state checks. A new UUID is not
an outcome-recovery strategy. See [recovery.md](recovery.md).

For atomic missing-record groups, preview_batch accepts a single batch-level
reconciliationLink. Every command must affect that selected account. Do not put
links inside individual commands or mix session scopes. Historical replacement
commands do not accept links; use the current target or missing-record route for
session corrections. Unlinked pre-existing target plans retain their original
receipt hash and recovery semantics.
