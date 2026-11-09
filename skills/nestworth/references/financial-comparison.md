# Compare two states without doing the arithmetic

For “how much did cash, investments, debt and net worth change between these
states?”, call `compare_financial_context` directly. Do not fetch two independent
context packages, pair their aliases, or calculate the answer yourself. The App
aligns real identities before assigning common aliases and calculates exact
right-minus-left differences. **Net-worth change is not investment return,
income, or attribution.** Period returns and profit questions use
[analysis.md](analysis.md) and [change attribution](financial-attribution.md).
One-date summaries use [financial context](financial-context.md).

<!-- example: financial-comparison -->
```json
{"leftAsOf":"2026-08-01","rightAsOf":"current","scope":{"kind":"household"},"disclosure":"minimal"}
```

Left requires a closed local date on or after History Origin. Right accepts a
closed date or current. No history means comparison is unavailable. Current means
the captured database state, not a date close and not a filter of every economic
fact by capture time. Historical results reflect currently retained corrections.
Both sides use current metadata and the household reporting currency. Preserve
the returned investment and category basis. Property and other non-investment
assets are separate. Named disclosure and selecting real account IDs follow the
same explicit-choice rules as a one-date package. Do not silently disclose
directory identities to work around failure.

Read and save all three initial page descriptors independently. Fetch only the
sections required by the question. For a complete detailed audit, continue each
matching cursor until complete, updating only the requested section's saved
state from the returned page and appending items to that section. Continuations
do not repeat the initial comparison header or other sections. Evidence
references include the side. Keep gaps and `dataAsOf` attached to that side.
Missing base deltas are null even when a same-currency native delta exists.
Different currencies, absent cells and unavailable amounts are not zero. Read
inclusion and lifecycle separately. Always answer total and category changes
from `change.*`. Never sum `positions[].baseChange` to explain `change.netWorth`
or a category. This remains true even when inclusion or archive status is
unchanged: a positive liability row delta means more debt and lowers net worth,
and an account rollup and its component rows represent the same money, not
separate contributions. `change.cash` and `change.netWorth` include entry and
exit from the included scope and therefore need not describe balance flows. A
row absent on one side has a null delta, while its included value still affects
`change.*`.

Synthetic USD example: cash rises from 100 to 120, investment value from 50 to
60, property from 300 to 330, and debt from 30 to 35. Report `change.cash = "20"`,
`change.investments = "10"`, `change.otherAssets = "30"`,
`change.liabilities = "5"`, and `change.netWorth = "55"`. The cash account and
balance child each show `baseChange = "20"`. Do not add them. The debt account
and its child each show `"5"`, which is debt growth, not a gain. If the unchanged
120 cash account is then excluded, authoritative cash and net-worth changes are
`"-120"` although its row balance delta is `"0"`. If a new included 7 cash
account appears only on the right, its null row delta does not remove that 7
from the summary change.

<!-- example: financial-comparison-positions-page -->
```json
{"cursor":"${cursor}","limit":25}
```

<!-- example: financial-comparison-gaps-page -->
```json
{"cursor":"${cursor}","limit":25}
```

<!-- example: financial-comparison-evidence-page -->
```json
{"cursor":"${cursor}","limit":25}
```

## When the comparison fails

Send one JSON-RPC object per request, never a batch. Zero rows plus nextCursor
means deferred, not finished. Use the same comparisonId, capture time and matching
section cursor throughout. Context IDs and cursors cannot be reused here.
Expiry, eviction or revocation requires a new comparison and restarting every
section. Discard old pages. The five-minute TTL never renews. A `too_large`
input, summary or single row may not be fixable by paging. Narrowing scope needs
actual, authorized account IDs and must not silently change the user's question.
On `context_expired` or `context_revoked`, restart with `compare_financial_context`.

Use `sections` to select which detail rows arrive initially. For a summary, send
`sections: []`. Each omitted section retains its total and a continuation when
rows are available. Default delivery fits up to 25 rows per section within a 24 KiB target.
An explicit `limit` allows up to 100 within the wire budget.
