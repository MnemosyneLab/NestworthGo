# Bounded read-only import comparison (historical proposal)

Status: historical proposal from the batch-history change. The implemented
`compare_market_data_import` tool is documented in the [API contract](mcp-import-comparison-api.md).
That contract is authoritative for request bounds, classification and evidence
scope; this page preserves the design rationale. The active-only batch-history
capture remains insufficient for import absence proof.

## Why a separate capture is required

Current market snapshots omit superseded/retracted Agent quotes. Chart values
also reflect source selection, date grouping and FX orientation. Neither an
empty chart nor a missing active observation proves there was never a matching
fact. A safe comparison needs raw observations, linked audit operations and
current source-policy evidence from one transaction. Using the existing active
snapshot alone would misclassify withdrawn evidence as missing. Implementing
this repository path and its independent oracle is deferred rather than exposing
an endpoint with incomplete evidence.

## Proposed request and response

The future tool should be a read effect available under the existing authenticated
read permission. Accept 1–100 items and a page limit of 1–100. Bound request
bytes with the existing MCP guard; reject oversized inputs before DB work.
Each item contains:

- A unique, nonempty `clientItemId` (up to 128 bytes), preserved exactly.
- Exactly one instrument ID or complete oriented FX pair.
- A valid civil date, quote type (`latest`, `close`, `nav`, `daily_reference`),
  currency and exact decimal price string. Latest also needs the actual instant
  and timezone; date alone cannot identify an intraday quote.
- A price basis/unit when type and currency do not uniquely specify it. NAV is
  unit NAV, never cumulative NAV, yield or raw close. Ambiguous basis is an
  invalid item, not missing.

Return a normal immutable `mcp-read/1` capture with ordered `results` and a shared
`evidence` section. Results have input ordinal, clientItemId, normalized input,
classification, reasons, evidence references and selection impact. Evidence
contains original quote identity, decimal, currency/direction, effective date,
instant/timezone basis, source kind/key, price basis, revision, audit links and
active/superseded/retracted state. Store evidence once and reference it from
duplicate results. Do not embed a second chart copy.

Do not run ordered items through `normalizeFrozenQuery`'s recursive array
sorting/deduplication. The implementation must add a narrowly scoped, tested
ordered-query freeze path (or encode ordinal-keyed items) and hash that exact
ordered representation. Do not weaken set normalization for existing tools.
Two identical inputs with different clientItemIds remain two ordered results.
Reject repeated clientItemIds; preserve duplicate business identities and label
them with stable duplicate group IDs. Different prices for the same normalized
identity/type/basis/date/direction produce an explicit input-conflict group even
when no persisted quote exists. Do not silently collapse or choose one input.

## Classification and evidence completeness

| Classification | Required proof |
| --- | --- |
| `same` | Complete raw evidence includes an active comparable observation with an exactly equal decimal in the same unit, basis, currency and original FX direction. Report any additional conflicting evidence separately. |
| `conflict` | Complete comparable active evidence exists at a different exact price, or the input group contains inconsistent prices. Include all alternatives and their sources/selection status. |
| `missing` | Known, authorized target and complete bounded search of active and historical evidence; no comparable active or historical observation exists. No market/provider request is needed to make this local-only statement. |
| `historical_only` | Comparable evidence exists only as retracted/superseded facts; expose audit links and replacement evidence. Never report these records as active or simply missing. |
| `unknown_target` | Instrument does not exist in the authorized scope. Do not call it missing price. |
| `invalid` / `unavailable` | Invalid value/type/date/basis, unsupported identity, incomplete evidence, unsupported raw conversion, budget exhaustion, or storage error prevents a comparison. Do not report missing. |

Authentication/permission failure aborts before target disclosure; never turn it
into an item-level missing result. Fail a capture on incomplete SQL evidence or
row/byte budgets, without returning partial classifications. Client pagination
does not affect computed classifications: all were computed from the complete
fixed snapshot. Until evidence pages are consumed, the client must present
evidence as unread, not absent. Return count and completeness metadata on every
section. Expiration/revocation requires restarting all sections together.

Use exact domain decimal parsers and arbitrary-precision decimal equality.
Numerically equal supported forms can normalize canonically; unsupported scale,
range, sign, exponent syntax, NaN or infinity must be rejected before rounding.
Instrument zero is allowed where the domain permits it; FX zero is invalid.
Use existing instrument and FX precision limits rather than silently changing
them. Tail differences within those limits remain conflicts. Do not compare
lexical formatting alone or use a tolerance.

Provider/Agent/manual provenance is independent of numeric equality. Compare
only like types and bases, and report same-value evidence from different sources
separately. Storage `observationKind=close` with `agent_unit_nav_v1` is NAV, not
`agent_raw_close_v1` or provider close. Converted metal values retain conversion
dependencies; unavailable original basis produces unavailable, not missing.

FX equality requires original direction. A reverse stored quote is evidence of
a related rate, not an exact equal original quote. Its rounded reciprocal may be
shown as a labeled display projection, never used for equality/missing proof.
Return `reverse_direction_only` as an unavailable reason when no original rate
can support a classification; do not fabricate a precise reciprocal.

Selection impact should separately report current policy preference, active
chart membership, selected quote identity and whether the candidate could affect
selection under the captured policy. Reuse existing selectors in a pure
simulation; do not insert a quote to test the outcome. Uncertain ties, incomplete
conversion dependencies or unrepresentable simulations yield `unknown`, with a
reason. A source can match numerically while not being selected. Current
valuation selection and chart selection remain distinct fields.

## Bounds and non-effects

Use a scoped, read-only SQLite transaction for target ownership, complete raw
observations, linked Agent operations and source preferences. Apply a captured
clock and the existing domain date-window rules, including market dates, local
intraday instants, DST and the documented missing-origin UTC fallback. Filter
audit history by the affected observation's date, not its correction/retraction
creation timestamp. Bound total examined rows to the shared 20,000-row read
budget and serialized capture to 4 MiB. Reuse five-minute TTL, 16 MiB/eight-entry
cache, two builders, ten-second build timeout, generation binding, content hash,
signed section cursors, 100-row page limit and 64 KiB final-wire budget. Do not
truncate evidence to fit. SQL failures and oversized captures are errors.

There are no provider calls, market refreshes, durable writes, operation receipts,
source-preference changes, automatic imports or overwrites. A content/capture hash
identifies historical captured bytes and is **not** a later-write CAS precondition.
Concurrent data or preference changes can occur after capture. Any later write
must independently revalidate against its authoritative mutation protocol.

## Required implementation acceptance

Use isolated synthetic SQLite and real MCP HTTP/SDK calls. Cover equal/conflict/
missing/empty/unknown/unauthorized and invalid inputs; 8-place instrument and
12-place FX tails; provider/manual/Agent and source priority; NAV versus close;
direct/reverse FX and rounded reciprocal collisions; correction and withdrawal
chains; ordered duplicates and same-key inconsistent prices; same ID rejection;
market day versus UTC date; spring/fall DST and UTC fallback; pagination and
unread evidence; exact content hash/frozen retry after writes; tampered/wrong
section/wrong capture cursors; TTL/revocation; SQL errors and row/byte/4 MiB
failure; no writes under SQLite query_only and a provider fake that fails if
called. Test normalization with at least two repeated identical business items
and a deliberately reversed client order.

For the 35-target/233-observation workload, measure capture plus every result,
metadata and evidence continuation call and actual/reserved response bytes.
Compare against legacy traversal using identical synthetic data and explicit
limits. Report default and explicit-limit measurements separately, without
promising one page or global latency/p95 improvements.
