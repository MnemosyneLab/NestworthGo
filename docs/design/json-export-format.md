# Nestworth JSON export, version 1

## Purpose

`*.nestworth.json` is a UTF-8 structured export for external programs. It is not
an import or recovery format. Use `.nestworth-backup` to restore or migrate the app.
Export includes all archived records and all locally stored price/FX observations;
it never fetches missing history. Secrets, settings, local paths, UI icon keys,
logs, daily valuation caches, mutation keys, and retry scheduling are not exported.
User-authored text is not redacted.

The normative field/type inventory is [JSON Schema](json-export-v1.schema.json).
`format` is `com.nestworth.export`; `formatVersion` is `1`, independent of the DB
schema. Consumers must reject unsupported versions. A breaking field, type, or
semantic change requires a new format version. The application version/build are
informational. IDs are preserved strings; resolve references by ID, never by name.

## Representation

- Amounts, quantities, prices, FX rates, split factors and dividend cash are decimal
  strings. Parse with decimal arithmetic, not binary floating point.
- Optional values are JSON `null`, including unavailable money and missing timezone.
  Empty collections are always `[]`. Flags are booleans. Sequences, revisions,
  sort orders and ownership basis points are integers (`10000` means 100%).
- Timestamps retain full stored precision and offset (RFC 3339); local dates are
  `YYYY-MM-DD`. `timezone` is the history-origin IANA timezone, or null when history
  has not started. Do not substitute the importing machine's timezone.
- Dataset arrays have deterministic ordering. Business replay order is explicitly
  `(effectiveAt, createdAt, id)`, with effect `sequence` within each activity;
  do not infer chronological order from array position or object key order.
- Every dataset and all summary calculation inputs come from one read transaction.
  `exportedAt`/`currentState.asOf` label the export calculation time, not quote time;
  quotes retain their own dates and provenance. The operation does not modify data.

## Dataset layout

`facts.directory` contains household and entity definitions, ownership, and holding
identity/metadata. Holding quantity is only in the derived current summary and
historical facts, so it cannot compete with history as a second authoritative input.

`facts.history` contains the opening state, all activities and effects, trade and
dividend details, correction/reversal links, and effective account/holding/instrument
states and valuation-source preferences. Original and reversal activities are both
preserved: consumers must apply the reversal semantics instead of summing every
trade twice. Cross-account transfers retain both endpoints and cost evidence.
`balanceBaselines`, `cashBaselines`, and `quantityBaselines` preserve observations
captured before history started, including households without an origin. When an
origin exists, replay starts from `openingPositions`; do not add the pre-origin
baselines again. Event/replay projections are deliberately omitted.

`facts.marketData` contains all manual/provider price and FX observations, including
superseded revisions; current and historical provider bindings; source preferences;
and the observation-slot selections needed to distinguish current canonical
observations from older revisions. `coverage` retains day-level availability facts,
including missing-day reasons and observation checking times. These facts describe
what was available locally at export, not a guarantee of complete market history.
Metal quotes include a normalized `conversion` object, with source price, unit,
FX rate and timestamps. Unknown fields in stored conversion evidence are excluded.

## Derived current state

`currentState.derived` is always true. `accounts` contains **every** account, including
archived accounts. `balance` is the latest balance/manual-value observation (null
for holdings-mode accounts); `cash` contains the latest observation per currency.
`valuedSubtotal` is only the sum of valued components, never an assertion that the
whole account is valued: check `complete` and `missingInputs`. Archived holdings are
excluded from account subtotals, matching the application valuation rules. Archived
account summaries are provided for inspection, not inclusion in household totals.

`holdings` contains every holding, including archived and zero-quantity definitions.
`quantity` is its current recorded quantity; `averageUnitCost` and `costBasis` are
replayed with the application's cost and transfer rules. `costStatus` is `complete`
or `unavailable`; unavailable cost is null, not zero. Costs use the instrument's
quote currency. `nativeValue`, `baseValue`, `valuationComplete`, and `missingInputs`
retain the application's missing-price and missing-FX semantics. An archived
holding's valuation is a current indicative valuation, not its historical exit value.
Zero-quantity holdings may have a valid zero valuation without a quote.

`baseCurrency` and `fxProvider` identify the current summary's currency and FX route.
No household total is emitted, avoiding accidental aggregation of archived or
excluded records. Consumers can display the current summary without implementing
history replay, or use the underlying facts for their own analysis. Exact historical
recalculation still requires the application's versioned calculation/source-selection
rules; this interchange document is not a database restore protocol.

## Example consumer

```python
import json
from decimal import Decimal

with open("Nestworth.nestworth.json", encoding="utf-8") as source:
    data = json.load(source)
assert data["format"] == "com.nestworth.export"
assert data["formatVersion"] == 1
instruments = {item["id"]: item for item in data["facts"]["directory"]["instruments"]}
for holding in data["currentState"]["holdings"]:
    if holding["archived"]:
        continue
    name = instruments[holding["instrumentId"]]["name"]
    print(name, Decimal(holding["quantity"]), holding["costStatus"])
```
