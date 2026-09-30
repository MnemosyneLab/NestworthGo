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
