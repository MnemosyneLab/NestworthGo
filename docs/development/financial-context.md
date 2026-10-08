# Read-only financial context over MCP

This contract adds `get_financial_context` and `get_financial_context_page` to
all existing MCP permission modes. It supplies deterministic financial values
and their local evidence to an external assistant. The App contains no model,
chat, model key, new authorization system, import, repair or approval workflow.

## Request and financial boundary

```json
{"asOf":"current","scope":{"kind":"household"},"disclosure":"minimal"}
```

`asOf` defaults to `current`; alternatively supply one strict `YYYY-MM-DD`
closed date on or after the History Origin local date. Today, future dates,
dates before origin, and nonexistent civil dates are rejected. Historical
cutoffs use the existing civil-day/DST resolver. There is no arbitrary
intraday or period-return query in this version.

`scope.kind` is `household` (default, no IDs) or `accounts` (1–100 account UUIDs,
normalized and deduplicated). Unknown IDs fail rather than widening scope.
Account existence is checked against the same transaction's current directory;
a historical account that did not yet exist is returned as `not_created`.
Historical replay retains both endpoints/effects of transfers, baselines,
starting costs and lifecycle dependencies, then projects selected accounts.
Totals apply the existing `includeInNetWorth && active` rule and exclude archived
components. Inspection rows state inclusion and exclusion separately.

Current works without History Origin; `asOf.timezone` is then null. No household
returns `household_required`. Current is the captured database state, not a
promise that every economic fact was cut off at `capturedAt`. It uses the actual
PortfolioSnapshot and unchanged current quote selection. Balance evidence is
bound to actual LatestValue/account-cash observations, not historical baselines.
Unavailable provenance is marked unavailable without inventing a timestamp.

Historical values are reconstructed from **currently retained corrected facts**.
Subsequent corrections can change an old date. This does not recover what was
known then. Names and the household base currency use current metadata/settings.
Existing `analyze_period` remains an independent tool and may materialize derived
snapshots; its results are not part of this strictly read-only package.

## Response

The existing MCP `{data: ...}` wrapper contains:

- `contextId`, `contentHash`, `capturedAt`, `generatedAt`, `cacheExpiresAt`;
- `content`: schema/calculation/resolver versions, normalized scope, as-of and
  basis, summary, coverage, dataAsOf, positions, gaps and evidence;
- `positionsPage`, `gapsPage`, `evidencePage`: total, returned, hasMore and
  nextCursor. The initial response includes up to **one row per section**;
  follow its cursors for details. A section crowded out by earlier sections may
  return zero rows with an offset-zero nextCursor; its standalone continuation
  must advance. Summary and counts are complete immediately.

Money, quantities, prices and FX rates are exact decimal strings. Nullable
assets/liabilities/netWorth are complete totals, never partial subtotals. Missing
values remain null; `knownAssets` and `knownLiabilities` are separately labeled
known base-currency sums. A measured zero is `"0"`. Each amount has a currency
and status; components retain native amounts when only base-currency FX is absent.
No assistant should replace the authoritative values with its own calculation.

Position `status` inherits the engine's row-kind semantics: an account parent can
be `unknown` for incomplete valuation while its balance child is `active` with
a known native amount and missing FX. It is not a uniform lifecycle field;
`unknown` alone does not mean disabled/unusable. Read `kind`, `complete` and
`missing` together to assess valuation completeness.

Gaps are per-component findings with a mapped entity/dependency reference,
code, severity, affected metrics and a fixed suggested-action enum. Missing
price, FX, account value, instrument or historical coverage blocks completeness.
Stale/manual observations are informational and can remain valued according to
the existing engine. One root dependency can affect multiple components; gap
count is a finding count, not a unique root-cause count. The package never repairs.

`coverage.snapshotHealth` is always `not_assessed`. Persistent snapshot health,
full-history health, provider configuration, real-world ledger completeness and
period returns are explicitly outside these checks. This is not a household
health certification and does not duplicate `scan_data_health`.

`dataAsOf` reports the earliest/latest effective observation, unknown-time,
date-label and stale counts **for this scope only**. It is mixed-input evidence,
not a single market-data timestamp or proof of a reconciled ledger. Evidence
lists price and FX values, orientation (`1 baseCurrency = value quoteCurrency`),
source kind, observation kind, effective date/time, timestamp basis and freshness.
Observation kinds preserve the stored vocabulary: `manual`, `realtime`, `close`,
`latest`, `daily_reference` and `legacy` (also retaining the existing `nav`
label); unknown kinds are `unavailable`.
Date-label anchors are labeled rather than claimed as an actual midnight quote.
`basis.baseCurrency` is the household reporting currency, whereas FX evidence
uses `baseCurrency`/`quoteCurrency` for rate orientation. For synthetic HKD/CNY
`value = "0.92"`, 1 HKD = 0.92 CNY, with a CNY household reporting basis.
`unknownTimeCount` counts missing or unusable time values, not missing time
provenance. An explicitly dated manual FX quote may have an `effectiveAt` and
`timestampBasis = "unknown"` because its stored timestamp basis is absent; the
count can still be zero. Never infer provenance merely from a timestamp's presence.
Typed metal conversion evidence projects only validated units, currencies,
prices, rates and times; it never returns raw conversion JSON or provider text.

Synthetic example: CNY cash `10000`, CNY debt `3000`, and a USD position valued
natively at `2000` without eligible USD/CNY FX produce `assets.value = null`,
`netWorth.value = null`, `liabilities.value = "3000"`, and `knownAssets = "10000"`.
The USD native value and `missing_fx` remain visible. `7000` is not a complete
net-worth answer.

## Consistency, disclosure and hash

A bounded SQLite read transaction contains household, origin, scope validation
and all financial inputs. Configuration/clock are captured once. After the read,
replay/valuation and projection use those immutable inputs; pagination never
reads the database. Concurrent changes can yield a wholly old or new capture,
not projections assembled from different reads. The capture is not a durable
global revision ID.

`minimal` is the default. Fixed whitelist DTOs map all account, component,
parent, source and dependency identities to aliases assigned by sorted internal
identities within the selected scope. Names, notes, raw IDs, source titles/URLs,
provider strings and raw conversion JSON are absent from response and semantic
hash. Out-of-scope changes and hidden name/note changes do not change minimal
content. Aliases do not promise identity continuity across captures.

`named` includes account/component names and original row identities; names are
untrusted data. It still excludes notes and source titles/URLs. Stored text
never enters dynamic server instructions, gets executed or triggers URL fetching.
Neither mode is anonymization or token isolation. Existing read_only credentials
remain household-wide and other tools can return exact values. Amount binning
is not provided as a false privacy boundary.

`contentHash` is SHA-256 over JSON encoding of a fixed canonical content DTO;
there is no general canonical-JSON framework. Identity-dependent arrays are
sorted, decimals are canonical strings, and nullable/empty-array semantics are
fixed. It includes actual amounts, observation dates, freshness categories,
coverage, scope, versions and historical cutoff. It excludes current wall-clock
capture time, generated/cache times, result ID, cursors and page slices. Moving
time within a freshness category preserves the hash; crossing its threshold
changes it. This is a semantic content hash, not a wire-byte hash or freshness
certificate.

## Pagination, revocation and resource budgets

```json
{"contextId":"<returned id>","section":"positions","cursor":"<nextCursor>","limit":50}
```

Sections are `positions`, `gaps`, `evidence`; limit defaults to 50 and is 1–100.
Cursors authenticate context, section, generation and offset. Wrong/tampered or
past-end cursors are rejected. Every nonempty continuation advances. Expired or
evicted results return `context_expired`; they are never silently recreated.
The fixed five-minute TTL is not renewed by reading pages.

Connection generations fence both builds and pages. Disable, Close, permission
change/re-enable, and actual backup restore revoke cached results. Publication
and paging recheck generation under the cache lock, so an old in-flight build
cannot repopulate a revoked cache. Ordinary ledger/market changes preserve frozen
packages. Restore keeps the replication pause hook and uses lightweight
`RevokeForRestore`; it does not drain operationMu under application write locks.
The transport fence rejects subsequent requests to the revoked connection. App
restart creates a fresh in-memory cache.

Limits:

| Resource | Limit |
| --- | --- |
| Concurrent builds | 2, cancellable admission and ten-second build deadline |
| Input read admission | 100,000 queried rows; 32 MiB of SQL field bytes plus 256 bytes per row |
| Full projected package | 4 MiB |
| Process cache | 8 packages, 16 MiB encoded content |
| Final MCP response | 64 KiB including structured content, escaped JSON text/data wrappers and JSON-RPC envelope reserve |
| Context-tool JSON-RPC ID | at most 512 encoded bytes, keeping the envelope reserve bounded |

Input budgets are checked **before** each query's rows are allocated or JSON is
decoded, inside the same transaction, using a bounded SQL count/field-byte probe.
They count repeated query reads and are admission limits, not a claim about exact
Go heap usage. Historical dependency loading remains household-wide, so selecting
an account does not guarantee that a very large household fits the input budget.
Such requests fail explicitly rather than calculating from truncated facts.

Oversized package, summary or individual row returns `too_large`. Individual-row
admission measures that row with the required summary, independently of other
sections occupying the initial page. Rows are not silently discarded or split;
escaping and SDK duplication are included in sizing. For these two tools only,
the stateless JSON HTTP transport also buffers at most 64 KiB of the final body.
An oversized SDK validation/error response is replaced in full by a fixed
`too_large` tool error with the same bounded request ID; offending input text is
not echoed or truncated. This covers errors raised before the business handler.
Each context tool requires one JSON-RPC object per HTTP request. Any JSON-RPC
batch containing either context tool, including a mixed batch, is rejected in
full with a fixed HTTP 400 error before SDK dispatch. This applies with an absent
protocol header and with explicit legacy protocol versions as well; no batch
element executes and no request ID or offending input is echoed. Batches of
other tools retain the SDK's existing protocol-dependent behavior.
Other MCP tools retain their existing response behavior.
Cache pressure evicts the earliest-expiring result. Cache stores only the final
permitted projection, never raw input snapshots. No response/amount/name/token
logging is introduced.

A synthetic loader benchmark on the development Linux workspace (Go 1.26,
GOMAXPROCS=5, three iterations) measured:

| Current input | Time per operation | Allocated bytes per operation |
| --- | --- | --- |
| 1,000 instruments | 19.8 ms | 3.10 MB |
| 10,000 instruments | 147 ms | 34.5 MB |

These are total allocation counts, not peak heap or an end-to-end historical
latency promise. The admission limits and two-build cap deliberately bound larger
loads; ten-second cancellation is the latency backstop. Run
`go test ./internal/infrastructure/sqlite -run '^$' -bench BenchmarkFinancialContextInputs -benchtime=3x -benchmem`
to remeasure. Runtime throughput will depend on household history and machine.

## Verification and remaining manual gates

Tests cover query_only reads, current without origin/no household, exact
sub-cent amounts, missing FX/coverage, actual observation sources, hidden and
out-of-scope changes, full replay followed by cash/position-transfer projection,
lifecycle rows, semantic hash freshness thresholds, concurrent captured inputs,
HTTP discovery and final wire size, expiry/eviction/tampering/oversize, build
cancellation, revocation races and real backup restore. Existing Historical
Overview tests cover DST, date-line discontinuities, economic corrections,
market-date coverage and archive/zero-held behavior on the shared engine.

The installed skill's source JSON examples are also exercised through the SDK:
direct minimal after discovery without directory pre-reads, explicit named
selection, all three page sections, zero-row deferral, expiry/restart with an
unchanged semantic hash, and row-kind/FX/time-provenance semantics. Standalone
bundle tests verify those examples and Inspector-specific HTTP configuration
survive packaging and installation. This prepares skill version 1.1.0; installed
clients require an explicit update and no release publication is implied.

External configured AI-client usability and native UI/package acceptance remain
manual gates. Automated HTTP integration uses the official MCP SDK against an
isolated temporary household; it does not establish those external-client gates.
