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
escaping and SDK duplication are included in sizing. For these context tools only,
the stateless JSON HTTP transport also buffers at most 64 KiB of the final body.
An oversized SDK validation/error response is replaced in full by a fixed
`too_large` tool error with the same bounded request ID; offending input text is
not echoed or truncated. This covers errors raised before the business handler.
Each context tool requires one JSON-RPC object per HTTP request. Any JSON-RPC
batch containing any context tool, including a mixed batch, is rejected in
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

## Frozen item drill-down

`get_financial_context_item` reads one `(contextId, ref)` from the cached, already
disclosed projection. It never calls the application/database, refreshes quotes,
recalculates totals or creates another context. Both **minimal** and **named**
packages are accepted. The item inherits the captured disclosure and returns only an already
disclosed subset: minimal has no names/raw IDs, named retains its existing names
and row IDs. No new identity fields, private ID mapping, token, disclosure
upgrade or identity lookup is introduced.

Arguments: required `contextId`, `ref`; optional `section` (`positions`, `gaps`,
`evidence`), `cursor`, `limit` (default 50, maximum 100). Omitted section returns
at most one row from each related section. An explicit section without cursor
starts at zero, including for empty sections. Continuations require both their
section and matching cursor. All three page descriptors retain total/returned/
hasMore/nextCursor semantics; crowded initial sections can defer zero rows, but
a standalone nonempty continuation must advance or return `too_large`.

The response's `schemaVersion` is `financial-context-item/1`. It retains the
original `contentHash`, `capturedAt`, `cacheExpiresAt`, `asOf`, `basis`, and
`disclosure`; `generatedAt` reflects this read. The hash still covers the entire
original package, not a newly hashed subset. `ref` and `type` identify the target:

- Account: `position` is the account rollup; `positions` contains direct non-account
  children only. Never add the rollup amount to those children. Gaps are limited
  to the target/children and evidence to their referenced dependencies.
- Position: `position` is the exact component, with its own gaps and evidence;
  `positions` is empty. No parent, siblings or sibling evidence are expanded.
- Evidence: `evidenceItem` is the selected observation, `positions` contains only
  its direct users, and gaps must directly name this evidence as dependency.
  `evidence` is empty; other observations and users' siblings are not expanded.

Related evidence is deduplicated by ref and original projection order is kept.
Nullable amounts, zero, inclusion/exclusion, row-kind status, missing FX, source
time and provenance are unchanged. Missing FX gaps may name a currency pair,
not an existing evidence ref; such a dependency is not a valid item target.
Only existing account/position/evidence members resolve, classified by collection
membership and row kind rather than alias prefixes or UUID shape. Ambiguous
refs are rejected. Minimal retains its no-name/no-raw-ID guarantee. Neither mode
returns source notes/URLs or an unrelated household summary/directory.

Cursors reuse the existing HMAC and authenticate operation, target, section,
context, generation and offset. Package-page cursors, wrong targets/sections,
wrong generations, tampering and past-end continuations fail `validation`.
A bare alias cannot encode its origin: identical spellings in two packages are
independent references. Clients must retain the original contextId/ref pair and
must not attach old answers to aliases from a new capture, even with an equal
hash. Expired/evicted contexts fail `context_expired`, revoked generations fail
`context_revoked`; neither path rebuilds or extends the fixed five-minute TTL.

The same cache lock, final SDK wire sizing (64 KiB), HTTP error buffer and batch
rejection protect this tool, including absent/legacy protocol headers. Ordinary
writes leave old items frozen. Both current and closed-day packages work; absent
rows remain absent. Transactions, period comparisons, describe-target identity
and additional evidence not in the package are outside this slice.

## Two-state comparison

`compare_financial_context` and `get_financial_comparison_page` extend this
contract with `financial-comparison/1`. Both tools are read-only in every MCP
mode and share the existing context cache's process budgets and revocation.

```json
{"leftAsOf":"2026-08-01","rightAsOf":"current","scope":{"kind":"household"},"disclosure":"minimal"}
```

Both dates are required. Left must be a closed local date; right is a closed
local date or `current`. The same origin, strict-date and civil-day/DST checks
apply. History is required even when right is current. Dates can be equal or in
reverse order: changes are always **right minus left, not return or attribution**.
Current is the captured database state, including retained subsequent corrections
and current observations; it does not truncate all economic facts to capture time.
Historical states use currently retained corrected facts. Household reporting
currency, classification and names use current metadata on both sides.

The application captures configuration and bounded read inputs once, replays
both sides with complete transfer effects, then projects the account scope.
It does not call `BuildFinancialContext` twice. Existing HistoricalOverview
alignment and decimal difference functions compute row changes. A sorted union
of real identities defines shared account/component aliases before disclosure.
Aliases are stable across these two sides only, never across independent packages.
The same evidence projection supplies each side's scoped coverage, gaps and
`dataAsOf`. Evidence references have `left:`/`right:` prefixes because freshness
or replay assertions about an observation can differ between sides.

The response has `comparisonId`, the same capture/hash/cache timestamps, `content`
and the three page descriptors. Content includes `left`, `right`, `change`,
`positions`, `gaps`, `evidence` and fixed basis declarations. Each side includes
the existing summary plus cash, investments and otherAssets. These categories
sum existing included asset components: `cash` uses the engine's cash bucket;
`investments` includes non-cash holdings and unclassified-investment balances;
remaining asset components (such as property) are `otherAssets`. No new pricing,
FX selection or financial engine is introduced. Each category is null if any of
its included components is incomplete. Liabilities remain a separate subtotal.

Positions pair left/right cells with base/native/quantity changes and `changed`.
A null side is absent from replay, not an assumed zero; explicitly selected
accounts not yet created have `not_created`. Archived, excluded, measured zero
and unknown values retain the existing row semantics. Row deltas are inspection
value changes, not inclusion-weighted contributions to the net-worth delta.
Only complete base values have base deltas. Native deltas require known amounts
and equal currencies; missing FX can leave a native delta with null base delta.
Summary deltas require both complete totals. No absent or unknown row is filled
with zero to manufacture a difference.

```json
{"comparisonId":"<returned id>","section":"positions","cursor":"<nextCursor>","limit":50}
```

Paging, semantic hashing, 5-minute TTL, 4 MiB package limit, 8-package/16 MiB shared
cache, two simultaneous builds and input/deadline limits are unchanged. A package
contains the permitted comparison projection only. The cache entry type rejects
cross-context/comparison paging; authenticated cursors also bind result identity,
section, offset and connection generation. The final 64 KiB HTTP guard and
single-object-only rule include both comparison tools, schema errors and legacy
or mixed batches. Ordinary writes preserve frozen results; connection changes
and restore revoke them. Expiry retains the existing `context_expired` code;
clients must rebuild the comparison and restart all sections.


Client paging state must retain each initial section descriptor independently.
After a context, item or comparison section request, update only the requested
section's cursor/completion and append its rows. Other descriptors in that
response restart at offset zero and must not replace accumulated progress.
Comparison `change.*` totals/categories are authoritative; position base changes
are never an additive contribution bridge, even with stable inclusion. Liability
sign, account/component duplication, absent sides and scope membership changes
prevent that interpretation. See the executable synthetic example in the user
skill's analysis reference.


The HTTP guard uses the pinned SDK's exact-case, single-JSON-value decoding for
both array detection and individual envelopes. Trailing JSON values are ignored
by the SDK; they must not make the guard skip ID, batch or final response limits.
The guard does not impose an EOF requirement or rewrite the request body. This
preserves unrelated tools' existing transport behavior, including legacy batches,
while protecting the first value the SDK will actually dispatch.

## Coherent comparison attribution

`compare_financial_attribution` accepts the same date/scope/disclosure request
as `compare_financial_context`, and returns a **new** `FinancialComparisonResponse`
with `content.attribution` (`financial-attribution/1`). It never accepts or
modifies an old comparisonId. The existing comparison tool remains strictly
no-write and leaves this optional field absent. No App AI integration or second
financial engine is introduced. No MCP permission mode or token scope changes.

The application coordinator spans validation, existing derived snapshot
maintenance, post-maintenance immutable facts/configuration capture, evidence
validation, analysis and MCP cache publication. Nested application operations
reuse that permit. Restore's exclusive gate cannot interleave; the cache's
connection generation is checked again at publication, so disable, permission
rotation, close and restore cannot republish a revoked in-flight build. Analysis
reads may append daily snapshot revisions/complete their range and conservatively
invalidate ledger previews. They are not PR36's strictly no-write path. MCP
annotations for this tool, analyze_period and all four attribution drilldown
tools use readOnlyHint=false, destructiveHint=false, openWorldHint=false.

Two closed local dates A < B normalize to inclusive A+1 through B, using civil
label arithmetic, not 24-hour durations. A's close is exclusive, B's close is
inclusive. Existing civil-close resolution supplies comparison boundaries.
Normal DST yields 23/25-hour intervals. The current analysis/materialization
engine still requires unambiguous local midnights: unsupported historical
transitions produce historical_boundary_unsupported without materialization.
Same-day/reversed ranges are validation errors. Current B gives unavailable
with right_endpoint_not_closed and no period/returns. Dates before History
Origin or unclosed labels fail existing validation.

The first slice supports household or a single actual account UUID (comparison
scope kind accounts). Multiple distinct accounts yield unavailable with
unsupported_account_set; no per-account rate aggregation. Minimal scopes remain
alias based; named keeps the existing explicit disclosure boundary. No Bootstrap
or directory fallback is required. Excluded positions remain visible in the
comparison; attribution covers only the resolved eligible net-worth universe.

After maintenance, a bounded repository capture supplies retained corrected
facts and current metadata/base currency. Every stored day from A through B is
checked against fresh replay/valuation of that captured batch: cutoff, currency,
resolver policy, snapshot completeness (used by the engine to prove absent-zero
components), selected component keys, native/exact base values, completeness,
classification and state/price/FX/preference observation IDs must match. The
source generation is checked after snapshot loading to fence repository writes
outside the coordinator. Historical eligibility must agree with the current
analysis universe on every day. Private observation IDs only contribute to a
scoped deterministic basisHash; raw IDs/names/notes do not enter the minimal link.
Full input admission and maintenance can still be household-wide, as existing
historical replay and derived snapshots require complete transfer dependencies.

The same verified immutable inputs feed ComputeAnalysis, foldAssetChange and
projectReturnTrend, without a pre-maintenance memo. The link includes scope,
normalized period, basis, compatible/incompatible/unavailable status,
mismatchReasons, asset availability/status/missingReason/residualIssueCount,
beginning/ending values, explainedDelta excluding residual, residual, bounded
existing waterfall drivers and investment return summary/sources. Return cash
inclusion is false; the engine's linked rate is preserved, never summed from
account rates. Nullable return amounts/rates and ratedDays/totalDays survive.
Known amounts use complete/partial status; nil amounts are incomplete.

Compatibility means proof of a shared calculation basis, not complete causal
classification or profit. A residual can make assetStatus partial while the
basis remains compatible. Once evidence is proved, endpoint equality and
explainedDelta + residual == change.netWorth are additional guards. Missing
intermediate valuation/snapshot boundaries, absent single-account endpoints or
an empty eligible analysis universe return unavailable, retaining measured partial
projections while explainedDelta/residual remain null. Income/contributions,
internal transfers, debt principal and adjustments must not be called investment
profit. Price/FX/dividend/fee sources come from existing attribution calculations.

Incompatible evidence/policy/cutoffs/inclusion/source revisions suppress joined
drivers and returns and explain the mismatch. Endpoint or reconciliation failure
also suppresses joined drivers/returns. A client must stop, not substitute a live
analysis. Existing contribution/driver detail tools still compute a fresh report,
can disclose identities, and are explicitly **not** frozen drilldown for this
comparison. Broader frozen attribution detail is outside this slice.

Attribution is part of the comparison content hash and required page summary.
Existing bounded cache, fixed five-minute TTL, revocation, section-independent
HMAC cursors, 4 MiB package and 64 KiB complete wire budget apply. Pages never
recompute or extend TTL. Expiry/eviction/revocation requires a fresh attribution
capture and restart of every section, with all old pages discarded. The new
name participates in the exact-case, single-Decode envelope guard, including
legacy mixed-batch rejection and trailing JSON behavior.

SDK schema validation diagnostics for the new tool are replaced by a fixed
validation error so rejected property names/values are not echoed; bounded
application WireError codes are preserved.
