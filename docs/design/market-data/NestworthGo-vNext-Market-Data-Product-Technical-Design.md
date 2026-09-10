# NestworthGo vNext Product + Technical Design
## Market Data Subsystem Upgrade

**Status:** Ready for implementation planning

**Revision:** 2026-09-10 — review decisions incorporated
**Project:** NestworthGo  
**Scope:** Market data, historical prices and FX, sync/backfill, valuation integration, snapshot repair, Data Health Center, unified Market Data UI  
**Primary goal:** Upgrade NestworthGo from a latest-quote-only model into a reliable local historical market-data subsystem that can support complete daily valuation and repair missing historical data without manufacturing fake market observations.

---

Planning readiness means the product policies, persistence boundaries, compatibility behavior, and acceptance cases below are decided. It does **not** mean implementation, provider compatibility tests, native Wails verification, or release acceptance have passed. Section 66 supplies the implementation gate matrix and evidence requirements.

## 1. Executive Summary

This version should be treated as a **Market Data subsystem upgrade**, not as a simple API-provider change.

The current application is primarily optimized for fetching and storing latest prices/rates. The vNext design introduces four capabilities that should work as one system:

1. A unified **Market Data** page replacing the current overlapping Investments and Market Data pages.
2. Historical instrument and FX data, including provider-backed backfill.
3. A first-class **Sync / Backfill Engine** that understands coverage, realtime data, official historical closes, manual pricing, and snapshot invalidation.
4. A new **Data Health Center** that diagnoses incomplete data locally and invokes the same Sync Engine to repair it.

The most important architectural rule is:

> **Real market observations and market-data coverage are separate concepts.**

NestworthGo must never persist fake weekend/holiday prices just to make a daily series look continuous. It should persist only real observations returned by a provider or entered manually. Separately, it should remember that a day or range has already been checked and produced no observation, so the system does not repeatedly query the same non-trading or no-data dates.

A valuation resolver may use the most recent previous value as a **carried-forward value**, but that resolved value is not a raw observation and should not be stored as one.

---

## 2. Product Goals

### 2.1 Primary goals

The version must:

- Merge the current Investments and Market Data pages into one unified **Market Data** page.
- Preserve all existing instrument-management and market-data capabilities.
- Add historical prices for instruments.
- Add historical FX rates.
- Add Tiingo as an optional provider for **US-listed stocks and ETFs only**.
- Keep Yahoo Finance for China and all other supported equity markets.
- Keep Frankfurter for FX.
- Preserve fully manual pricing for instruments that do not use a market-data provider.
- Distinguish `realtime` quotes from official historical `close` observations.
- Avoid storing synthetic weekend/holiday prices.
- Persist enough coverage information to know that a date has already been checked and returned no observation.
- Detect and repair historical gaps.
- Rebuild only snapshots affected by new or corrected market data.
- Surface missing or stale data through a dedicated **Data Health Center**.
- Show meaningful live sync/backfill progress rather than only a generic spinner.
- Keep historical synchronization idempotent.
- Respect current quote TTL/cache behavior for today's data.
- Avoid unnecessary repeated provider requests.

### 2.2 Historical coverage horizon

The required historical range starts when data first becomes relevant to the household.

For instruments:

> Required historical coverage starts on the first date the instrument actually affects household net worth.

For FX:

> Required historical coverage starts on the first date that currency conversion is required for household valuation in the configured base currency.

Baseline vNext behavior may use:

```text
required_start → today
```

for simplicity.

A future optimization may narrow this into actual exposure intervals, but this is not required for vNext.

### 2.3 Non-goals

This version is not intended to:

- Become a general-purpose financial market terminal.
- Download full provider history for every instrument regardless of user ownership.
- Store synthetic daily prices for non-trading dates.
- Implement exchange trading calendars as a hard dependency.
- Add Tiingo support for Chinese equities.
- Support arbitrary per-market provider routing beyond the explicit vNext requirements.
- Automatically perform network synchronization every time the application starts without user intent.
- Guarantee intraday charting or tick-level market data.
- Use market-data observations as an accounting ledger.

---

# 3. Product Information Architecture

## 3.1 Navigation

Replace the existing separate Investments and Market Data entries with:

```text
...
Market Data
Data Health
...
```

`Market Data` is the primary management and inspection surface.

`Data Health` is an independent diagnostics and repair surface.

When data is healthy, Data Health should remain visually quiet. It should become prominent only when the application detects incomplete valuation inputs, missing coverage, failed synchronization, invalid provider configuration, or stale snapshots.

---

# 4. Unified Market Data Page

## 4.1 Page name

Use:

> **Market Data**

This name accurately covers both instrument pricing and FX rates, while remaining broader than “Investments”.

## 4.2 Tabs

The page contains two primary tabs:

```text
[ Instruments ] [ FX Rates ]
```

The existing Investments page should be used as the primary structural basis for the Instruments tab.

Features currently located on the existing Market Data page should be merged into the relevant tab rather than retained as a second overlapping page.

---

## 4.3 Market Data page header

Example:

```text
Market Data

Manage pricing sources, historical observations and FX rates.

Data updated Sep 10, 10:32
[ Sync Data ]
```

If there are detected health issues:

```text
Data updated Sep 10, 10:32
3 issues need attention
[ Sync Data ] [ View Data Health ]
```

If synchronization is running:

```text
Syncing market data...
Historical prices  8 / 12
[ View Progress ]
```

---

# 5. Instruments Tab

## 5.1 Responsibilities

The Instruments tab owns:

- Instrument list
- Instrument creation
- Instrument editing
- Instrument archive/unarchive
- Pricing mode
- Provider binding
- Provider symbol
- Current resolved price
- Raw latest observation
- Historical price sheet/chart
- Manual price entry
- Price source status
- Per-instrument refresh/sync status

## 5.2 Example provider-priced instrument

```text
AAPL
Apple Inc.
US Equity · USD

$234.42
Realtime · Tiingo · 8 minutes ago

Historical coverage: Verified through Sep 9

[ History ] [ Edit ]
```

If today is a non-trading day:

```text
$231.78
Carried forward from Sep 9 close

Latest provider check: Sep 10, 10:30
```

## 5.3 Example manually priced instrument

```text
ABC Bank Wealth Management
Manual · CNY

¥1.041
Manual price · Sep 8
Carried forward to today

[ Set Price ] [ History ] [ Edit ]
```

A manual instrument must never be included in provider-fetch plans.

---

# 6. FX Rates Tab

The FX Rates tab owns:

- Required/preferred FX pairs
- Current FX rates
- Historical FX observations
- Manual FX rates
- FX source mode
- FX history
- Sync state
- Missing coverage state

Example:

```text
USD / SGD
Frankfurter

1.2834
Latest · 20 minutes ago

Historical coverage: Verified through Sep 9
[ History ]
```

For historical Frankfurter data, the stored semantic is not literally an exchange closing auction price. Use an FX-specific observation type such as:

```text
daily_reference
```

rather than falsely describing it as an equity-style close.

---

# 7. Market Data Source Configuration

## 7.1 Provider routing

vNext provider strategy:

| Market / Data Type | Provider |
|---|---|
| US equities | User-selectable: Tiingo or Yahoo Finance |
| China equities | Yahoo Finance |
| Other supported equity markets | Yahoo Finance |
| FX | Frankfurter |
| Manual instruments | No provider |
| Manual FX | No provider |

Routing uses the instrument's verified listing market/exchange and asset type, not its currency or issuer country. US-listed ETFs such as QQQ and SGOV are in scope; unsupported types retain their existing manual/provider behavior and must not be silently routed to Tiingo. Bindings must validate instrument identity, listing, currency, and price units before activation.

## 7.2 Settings UI

Add a Market Data section to Settings:

```text
Market Data

US stock provider
[ Tiingo ▼ ]

Tiingo API Key
[ •••••••••••••••• ]
Status: Configured

Other stock markets
Yahoo Finance

FX provider
Frankfurter

Quote cache TTL
[ 3 hours ▼ ]
```

If Yahoo is selected:

```text
US stock provider
[ Yahoo Finance ▼ ]

Tiingo API Key
Configured but unused
```

Do not delete the Tiingo key when the user temporarily switches back to Yahoo.

## 7.3 Tiingo secret storage

The Tiingo API key should not be persisted in ordinary application SQLite data or exported with normal backups.

Use a secret-storage abstraction backed by the operating system where practical:

- macOS: Keychain
- Windows: Credential Manager
- Linux: Secret Service / compatible keyring

Application settings store non-secret provider selection only. `tiingo_key_configured` is a derived backend status from the secret store, not an authoritative persisted boolean: a restored database/settings file may be opened on a machine without the key.

The backend should retrieve the real secret from the secret store only when constructing Tiingo requests.

If the OS store is unavailable or locked, expose that state and allow a session-only key held in backend memory. Do not silently fall back to plaintext persistence. Save/replace/remove return redacted status only; restore never implies that a key has been restored. Native verification of store access and failure behavior is a release gate.

---

# 8. Provider-Neutral Market Data Architecture

The backend should not implement historical data as Tiingo-specific business logic.

The application layer should expose provider-neutral contracts.

Conceptual interface:

```go
type MarketDataProvider interface {
    Capabilities() MarketDataCapabilities

    LatestInstrument(
        ctx context.Context,
        identity InstrumentMarketIdentity,
    ) (LatestInstrumentQuote, error)

    InstrumentDailyHistory(
        ctx context.Context,
        identity InstrumentMarketIdentity,
        dateRange DateRange,
    ) (HistoryBatch[InstrumentDailyObservation], error)

    LatestFX(
        ctx context.Context,
        identity FXMarketIdentity,
    ) (LatestFXQuote, error)

    FXDailyHistory(
        ctx context.Context,
        identity FXMarketIdentity,
        dateRange DateRange,
    ) (HistoryBatch[FXDailyObservation], error)
}
```

History results carry coverage evidence; an empty successful array alone is insufficient:

```go
type HistoryBatch[T any] struct {
    Observations   []T
    VerifiedRanges []DateRange // complete retrieval for eligible finalized dates
    PendingRanges  []DateRange // publication/session not ready; never negative-cache
    UncertainRanges []DateRange // truncation or uncertain completeness
    NextCheckAt    *time.Time
    Evidence       ResponseEvidence // adapter/version, source policy, request identity
}
```

All ranges use inclusive market dates internally; adapters translate provider endpoint boundaries. Verified, pending, and uncertain ranges are disjoint subsets of the requested range. Validate identity, currency/unit, numeric values, timestamps, duplicates, response limits and pagination before committing a batch. Invalid batches commit nothing. Valid observations from an explicitly partial response may be persisted, but only `VerifiedRanges` can produce no-data records. Provider publication rules, request limits and evidence for completeness are adapter contracts tested with recorded fixtures, not deductions from HTTP 200.

Capabilities remain explicit:

```text
LatestInstrument
InstrumentSearch
InstrumentDailyHistory
LatestFX
FXDailyHistory
```

A provider should never be called for a capability it does not declare. Implement the conceptual contract with separate optional capability interfaces where appropriate; FX-only providers need not implement instrument methods. Preserve instrument search and existing latest capabilities.

---

# 9. Provider-Specific Instrument Bindings

## 9.1 Problem

A single instrument may require different symbols on different providers.

For example, changing a US instrument from Yahoo to Tiingo must not overwrite or destroy the Yahoo symbol binding.

The current concept of one `providerKey/providerSymbol` attached directly to an instrument is insufficient once provider switching becomes a first-class feature.

## 9.2 Proposed model

Introduce provider-specific bindings:

```text
Instrument
    id
    canonical metadata
    pricing_mode

InstrumentProviderBinding
    instrument_id
    provider_key
    provider_symbol
    market
    currency
    enabled
    binding_revision
    effective_from
```

Unique key:

```text
(instrument_id, provider_key)
```

Example:

```text
AAPL
  yahoo_finance → AAPL
  tiingo        → AAPL
```

A provider switch changes routing through an effective-dated routing revision, not the canonical instrument record. The current binding can retain the unique key above, but prior binding revisions remain immutable and addressable. Changing a symbol, currency/unit, listing, or source policy creates a revision; observations and coverage reference that revision. Old-symbol coverage must not suppress a query for the corrected binding.

If the chosen provider does not have a valid binding for a required instrument, Data Health should report:

```text
Provider binding missing
AAPL cannot be synchronized with Tiingo.
```

---

# 10. Observation Model

## 10.1 Core principle

Only persist **real observations**.

Never persist forward-filled values as raw observations.

## 10.2 Instrument observation kinds

Instrument observations should distinguish at least:

```text
manual
realtime
close
legacy
```

### `manual`

Entered by the user.

### `realtime`

Returned by a latest/intraday provider request.

### `close`

Returned by a historical daily/EOD provider endpoint and considered the canonical historical price for that trading date.

### `legacy`

Used only for migrated observations whose historical meaning cannot be proven.

A legacy provider observation must not automatically count as an official daily close.

## 10.3 FX observation kinds

FX should use:

```text
manual
latest
daily_reference
legacy
```

`daily_reference` is the historical daily rate returned by Frankfurter or an equivalent daily FX source.

## 10.4 Suggested fields

Conceptually:

```text
MarketObservation
    id
    target_type
    target_id
    provider_key
    provider_symbol
    observation_kind
    effective_date
    observed_at
    value
    currency/base/quote metadata
    provider_timestamp
    fetched_at
    source_metadata
    binding_revision
    source_policy_version
    price_basis
    revision
    supersedes_observation_id
```

The application may keep separate instrument-quote and FX-rate tables if that better matches the existing domain model. The semantic rules are more important than forcing both into one physical table.

## 10.5 Time and date contract

Keep four separate concepts:

- `effective_date` / market date: the provider's market-session or reference-date label.
- `value_effective_at`: the actual economic timestamp represented by the value; for a finalized equity close, the session close instant.
- `valuation_cutoff`: household-history-timezone end of day for snapshots, or backend `now` for live valuation.
- `fetched_at`: ingestion time; this does not decide historical eligibility.

Persist market timezone/session metadata needed to derive the effective instant. An EOD date serialized at UTC midnight is a date label, not proof that its closing price was available at midnight. Never derive household snapshot dates by copying a market-date label.

A historical correction fetched later may restate history only when its economic effective instant is at or before the cutoff. A close formed after the cutoff is never eligible. Household history timezone remains authoritative; OS/display timezone changes do not reinterpret stored history.

The resolver's closed-day policy is **latest eligible finalized close/reference as of the household cutoff**, including carry-forward when eligible. It does not use an in-progress session's eventual close. This deliberately replaces legacy intraday-based snapshot valuation and requires a resolver-policy version and migration invalidation. A session whose close lies after the cutoff does not require a finalized close for that snapshot.

Example: while Singapore is on Sep 10, a Sep 9 US-session realtime value can be the newest valid live price. For the Singapore Sep 9 closed-day snapshot, the later Sep 9 US close cannot be used. Use the preceding eligible close under this policy, with provenance. Market session state and provider publication state must be determined separately from the household date.

No full exchange-calendar dependency is required, but an adapter must provide trustworthy session/effective-time and finalization rules. Unknown session timing or early-close handling remains uncertain; never invent a timestamp or mark coverage verified. Accept conservative delay/retry rather than false finality.

## 10.6 Price basis and numeric contract

Canonical valuation prices must match the historical ledger quantity basis. Use prices without dividend adjustment and without a quantity-basis mismatch. Tiingo raw `close` is the intended input; `adjClose` is not interchangeable. Yahoo mapping must be validated with split and dividend fixtures, including any normalization needed to match historical ledger quantities; a field named `close` is not sufficient evidence of its adjustment basis.

Retain provider-returned raw values and adjustment evidence. Any normalized valuation price is derived with explicit provenance, not relabeled as a raw observation. Record `price_basis`, provider field mapping/adapter version, currency and quote units. Unsupported or unverifiable basis is a blocking adapter/target issue, never a silently accepted price. Provider corporate-action metadata may support normalization/diagnostics but must not automatically create ledger dividends, splits or trades.

Parse numeric JSON directly into decimal representations, not through binary floating point. Preserve exact decimal values through FX conversion and aggregation; round only at existing domain/DTO boundaries. Validate supported domain range and scale, positive provider prices/rates and unit conversion explicitly; malformed, null or unsupported values are not zero observations.

Acceptance includes splits, reverse splits, cash dividends and switching providers, checking both net worth and Analytics Price/Dividend attribution against independent expected values.

---

# 11. Historical Canonical Value Rules

## 11.1 Closed household dates

Resolve at the household cutoff using the pricing-mode and routing revisions effective then:

1. Select the latest eligible canonical close (FX: daily reference) whose `value_effective_at <= cutoff`.
2. A prior value may be carried forward; retain its effective date, observation revision and source.
3. Missing/unverified intervening eligible sessions or provider-no-data uncertainty produce a quality issue. They must not be relabeled as verified non-trading days.
4. No usable value produces `missing` with a null amount, never a decimal zero.
5. Realtime and legacy observations do not satisfy finalized historical coverage. Legacy compatibility display is separately governed by section 32.7.

A finalized session whose close is after the cutoff is not eligible even if its market-date label equals the household date. A session that had ended by the cutoff but whose data is still pending publication leaves historical quality pending until verified.

## 11.2 Live valuation

Use the latest valid observation from the effective pricing source as of backend `now`, comparing actual economic timestamps. Eligible realtime and canonical closes are both candidates; a finalized close wins a tie for that session. Do not discard the latest US-session realtime because its market date is yesterday in the household timezone. If only a prior close is usable, display carried-forward provenance.

TTL determines whether to request data, not whether a valid cached value becomes numerically unavailable. On refresh failure retain usable values with stale/quality metadata. Never manufacture a today observation or take a future observation.

## 11.3 Future dates

Future valuation cutoffs are invalid. Historical requests stop at adapter-eligible finalized market dates; pending sessions are separate from confirmed gaps.

## 11.4 Manual instruments and FX

Resolve the latest manual observation whose actual effective timestamp is at or before the cutoff. It remains effective until superseded; it is missing before the first usable manual value. Carry-forward alone is not an error for manual mode.

Modes are effective-dated. A target currently in manual mode receives no automatic provider requests, including Repair All. Earlier provider-mode gaps remain diagnosable with an explicit explanation and a historical manual-entry/routing action; they are never declared repaired by skipping the target. Switching back to provider permits planning only the historical provider-mode intervals. Historical manual intervals remain excluded.

Automatic cross-provider fallback is disabled in vNext. Legacy compatibility display is explicit and does not create selected-provider coverage.

---

# 12. Data Coverage Model

## 12.1 Why coverage must be separate from observations

The application needs to distinguish:

1. Data is missing because it has never been queried.
2. The date was queried successfully, but the provider returned no observation.
3. The provider request failed and should be retried.

Storing fake prices is not an acceptable solution.

## 12.2 Day-status record

Introduce a separate coverage/day-status concept.

Example:

```text
MarketDataDayStatus
    target_type
    target_id
    provider_key
    effective_date
    status
    reason
    checked_at
```

Coverage outcomes:

```text
no_observation     # successful finalized-range omission; negative-cache with expiry
pending           # session/publication not ready; retry later
uncertain         # completeness or data quality cannot be established
```

Persist `next_check_at`, `expires_at`, binding revision and source-policy version with checked results. `no_observation` is a retrieval outcome, not proof of a closed market or trustworthy zero return. Expired results become eligible for normal recheck.

Suggested initial reason:

```text
provider_no_data
```

Do not call this `market_closed` unless the application has independent evidence from a trading-calendar system.

A provider omission may mean:

- weekend
- holiday
- suspended instrument
- pre-listing date
- no trade
- provider data gap

`provider_no_data` is deliberately conservative.

## 12.3 Provider-specific coverage

Coverage must include `provider_key`.

Example:

```text
AAPL / Tiingo / Sep 6 → no_observation
```

must not prevent a future Yahoo query from checking Sep 6.

## 12.4 Successful range query behavior

When a historical range request succeeds:

```text
requested: Sep 1–Sep 7
provider returned:
    Sep 1
    Sep 2
    Sep 3
    Sep 4
    Sep 7
```

Then:

```text
persist real observations for returned dates
persist no_observation status for Sep 5 and Sep 6
```

only within the adapter-returned `VerifiedRanges` for finalized dates. Pending or uncertain dates never become no-observation. A supplied observation supersedes that date's negative-cache record in the same transaction.

## 12.5 Failed requests

The following must **not** create no-observation markers:

- timeout
- connection failure
- authentication failure
- HTTP 429
- server error
- malformed response
- provider-level partial failure where completeness cannot be trusted

These remain unresolved gaps and should be retried.

## 12.6 Force recheck

vNext includes a scoped **Force Recheck** action for an instrument/FX pair and date range. It bypasses both positive and negative history caches after a request-plan preview, subject to quota/backoff. Do not delete existing usable data before a successful replacement. It can discover corrected closes as well as replace no-data markers.

Negative-cache policy: recent omissions (last 7 finalized market dates) expire after 24 hours; older omissions after 30 days. Pending publication follows adapter `NextCheckAt`; ordinary sync also reconciles the last 3 finalized dates once per 24 hours to catch recent corrections. Force Recheck supports older corrections. These policies run only within user-initiated sync/repair; they do not start background network work on a timer or application launch. They are request policies, not guarantees that a provider omission means market closure.

---

# 13. Required Coverage Planner

Introduce an application service responsible for answering:

> What historical data does NestworthGo actually need?

Conceptual service:

```go
type RequiredCoveragePlanner interface {
    RequiredInstrumentCoverage(...) []CoverageRequirement
    RequiredFXCoverage(...) []CoverageRequirement
}
```

## 13.1 Instrument requirement

For each provider-priced instrument:

```text
required_start =
    first date the instrument affects household net worth

required_end =
    today
```

Current manual instruments are excluded from automatic fetch execution; historical mode intervals remain part of diagnostics.

## 13.2 FX requirement

For each required conversion pair:

```text
required_start =
    first date the currency must be converted
    to the household base currency

required_end =
    today
```

Current manual FX pairs are excluded from automatic provider sync; historical mode intervals remain part of diagnostics.

## 13.3 Base-currency change

Changing household base currency may create new historical FX requirements.

After a base-currency change:

```text
recompute coverage requirements
→ Data Health identifies new missing FX ranges
→ user may repair
```

Do not assume prior FX coverage remains sufficient. Base currency changes are versioned valuation-policy changes; invalidate affected snapshots and Analytics caches even if all required FX is already local. Preserve the application’s historical currency contract rather than merely altering display symbols.

## 13.4 Requirement horizon versus fetch dependencies

Compute required exposure from Starting point, immutable activities, historical account eligibility and mode facts, including archived instruments with past exposure. Do not use only today's holdings/list filters. Include account cash, liabilities and Analytics native/base conversion and period-opening inputs; missing snapshots must not be the only source of requirements.

Separate the required valuation interval from the fetch interval. A Sunday Starting point may require a preceding Friday price/FX. An opening anchor can predate the assigned routing interval, but must come from the source selected for that interval and retain its true timestamp. Search backward for an eligible anchor with widening 7, 30, then 365 calendar-day windows, bounded by provider availability. Stop when an anchor is found; after the bound report `initial_anchor_missing` with manual historical-entry guidance rather than an endless Repair loop. Never use a later price to seed an earlier date.

The baseline may overfetch through today for an eligible provider target, but historical manual intervals are excluded and current manual mode suppresses all automatic fetching. Actual exposure-interval optimization remains optional.

---

# 14. Gap Detection

A finalized historical market date is retrieval-covered for the effective binding/source-policy revision when one of these is true:

```text
canonical close exists for the effective binding/source policy
OR
unexpired provider/binding-specific no_observation status exists
```

A `realtime` or `legacy` observation does not satisfy historical close coverage.

For FX:

```text
canonical daily_reference exists for the effective source policy
OR
unexpired provider/binding-specific no_observation status exists
```

Retrieval coverage and valuation quality are separate. A no-data response suppresses requests only until expiry; it does not prove a non-trading day. The health scan reports unverified provider gaps even when a prior amount is displayable. Only independently supported non-trading carry-forward (or manual policy) can be marked verified complete without an exact observation. Without such evidence, conservative quality warnings are expected; no exchange-calendar service is required to fabricate certainty.

Historical gap scanning is local and should not require network access.

---

# 15. Range Coalescing

The Sync Engine should not generate one API request per missing day.

Example missing dates:

```text
Sep 1
Sep 2
Sep 3
Sep 7
Sep 8
```

Coalesce into:

```text
Sep 1–Sep 3
Sep 7–Sep 8
```

Provider adapters may further widen or batch ranges when efficient.

The planner should minimize requests while preserving provider limits and correctness.

---

# 16. Sync / Backfill Engine

## 16.1 Replace “Refresh” with a broader concept

The existing refresh behavior should evolve into a first-class synchronization workflow.

Conceptual pipeline:

```text
Scan
  ↓
Plan
  ↓
Historical Backfill + Atomic Invalidation
  ↓
Today's Quotes + Atomic Invalidation
  ↓
Collect Dirty Ranges
  ↓
Snapshot Rebuild
  ↓
Verify
```

## 16.2 Local scan

The Scan stage:

- computes required coverage
- inspects existing observations
- inspects no-observation markers
- identifies provider configuration issues
- identifies historical gaps
- identifies today's stale/missing latest data
- identifies dirty/incomplete/missing snapshots

No network requests occur during this stage.

## 16.3 Plan

The plan should be inspectable before repair.

Example:

```text
Repair Plan

Historical prices
8 ranges across 12 instruments

Historical FX
2 ranges across 3 pairs

Today's quotes
5 stale instruments

Snapshots
4 dates require rebuild
```

## 16.4 Historical backfill

Historical backfill:

- uses provider date-range endpoints
- stores official closes / daily FX references
- records no-observation day status for omitted dates from successful complete responses
- does not create synthetic prices
- skips unexpired covered dates outside the recent-correction window unless Force Recheck is requested
- skips manual targets
- may replace historical reliance on old realtime/legacy observations
- invalidates only affected snapshots

## 16.5 Today's quotes

Today's step:

- applies existing TTL/cache semantics
- fetches only stale or missing latest data
- stores realtime/latest observations only when provider timestamps make sense
- may receive prior-day data on weekends/holidays without treating it as today's observation
- resolves current display using prior close if required

## 16.6 Snapshot step

Any new or corrected historical market data may change historical valuation.

Affected snapshots must be marked dirty and rebuilt.

Do not rebuild unrelated dates.

## 16.7 Verify

After the job finishes:

```text
run local health scan again
```

The result should state whether repair fully succeeded or remaining issues exist.

---

# 17. Sync Idempotency Rules

Running the same sync repeatedly must be safe.

Rules:

```text
official close already exists
    → skip unless recent-correction reconciliation or Force Recheck is due

unexpired no_observation marker exists
    → skip unless recent-correction reconciliation or Force Recheck is due

past date contains realtime only
    → still requires historical close lookup

manual target
    → never provider-fetch

failed previous request
    → retry allowed

latest capability has a successful check inside request TTL
    → cached / skip

historical provider returns same close again
    → no semantic change

historical provider returns corrected close
    → append a revision, advance canonical selection and atomically invalidate affected snapshots
```

Use immutable observations with deterministic deduplication and an updatable canonical-selection index for official daily observations. Do not mutate a raw observation already referenced by a snapshot.

Conceptually:

```text
canonical slot unique:
(target, provider, binding_revision, source_policy_version, effective_date, observation_kind)
```

---

# 18. Realtime vs Close Transition

Example:

On Sep 1, while market is open:

```text
Sep 1 realtime = $101
```

On Sep 5, a historical sync runs:

```text
Sep 1 close = $103
```

Expected result:

```text
Raw observations:
Sep 1 realtime = $101
Sep 1 close    = $103

Historical valuation for Sep 1:
$103 close
```

The realtime observation may remain as historical raw data, but it is no longer the canonical daily historical value.

---

# 19. Non-Trading-Day Behavior

## 19.1 Historical dates

Do not persist:

```text
Saturday price = Friday price
Sunday price   = Friday price
```

Instead:

```text
Friday:
close observation = 100

Saturday:
no_observation status

Sunday:
no_observation status
```

Resolver output:

```text
Friday   100 close
Saturday 100 carried_forward
Sunday   100 carried_forward
```

## 19.2 Live values and publication finalization

Current requests use section 29 request-check TTL, not the observation's market date. The latest valid market-session realtime may belong to the preceding household date. Show its true date and freshness.

A latest endpoint returning no new value does not establish historical no-data. A daily endpoint also cannot establish finality solely because the household date advanced: use adapter session and publication rules. Finalized-range omissions use the expiring negative-cache policy in section 12; pending data stays retryable.

---

# 20. Resolved Value API

Raw data access and valuation resolution should not be the same API.

Introduce an application/domain resolver:

```go
type ResolvedMarketValue struct {
    Value            *Decimal // nil only when unavailable; zero is a real amount
    EffectiveDate    LocalDate
    SourceKind       SourceKind
    ProviderKey      string
    Resolution       ResolutionKind
    ValueEffectiveAt time.Time
    ValuationCutoff  time.Time
    ObservationID    string
    ObservationRevision string
    PolicyVersion    string
    Quality          string // verified / unverified / pending / legacy / missing
    Reasons          []string // stable localized codes, not provider prose
}

type ResolutionKind string

const (
    ResolutionDirect         ResolutionKind = "direct"
    ResolutionCarriedForward ResolutionKind = "carried_forward"
    ResolutionMissing        ResolutionKind = "missing"
)
```

For instrument history:

```go
ResolveInstrumentValue(instrumentID, valuationCutoff, policyRevision)
```

For FX:

```go
ResolveFXRate(pair, valuationCutoff, policyRevision)
```

This resolver is the only layer that should implement carry-forward semantics.

UI history charts that intend to show raw provider observations should continue showing only real observations.

Valuation/trend/snapshot logic should use resolved values. Availability, freshness and verification are independent: a usable carried value may have uncertain quality. Propagate that quality to snapshots and Analytics coverage/AmountStatus; do not present an unverified unchanged price as confirmed zero return. Only verified eligible inputs count toward rated coverage. Retain known partial amounts using the existing partial/unavailable contract; an unverified fallback cannot upgrade a day to fully rated. Frontend consumes authoritative DTOs without recomputing amounts. Include calendar day/month/year, Contribution, Drivers, Categories and Trend in contract tests.

---

# 21. Historical Chart Semantics

The UI should distinguish between two concepts.

## 21.1 Observation history

Displays real stored observations only.

Example:

```text
Sep 5   102 close
Sep 8   105 close
```

There is no fake Sep 6 or Sep 7 point.

## 21.2 Valuation history

When a net-worth or valuation chart is computed, the resolver may output:

```text
Sep 5   102 direct
Sep 6   102 carried
Sep 7   102 carried
Sep 8   105 direct
```

These are derived values and should not be inserted into the quote table.

---

# 22. Snapshot Lifecycle

## 22.1 Existing problem

Once historical data can arrive later, a snapshot may exist but no longer be correct.

Snapshot state is therefore not merely:

```text
exists / missing
```

Track independent dimensions:

```text
existence: missing / present
completeness: complete / incomplete
freshness: current / dirty-outdated
quality: verified / unverified / pending / legacy
```

## 22.2 Example

Initial state:

```text
Sep 3 snapshot:
AAPL uses carried-forward $100
```

Later sync:

```text
Sep 3 close arrives:
AAPL = $105
```

The existing Sep 3 snapshot is now outdated.

Expected:

```text
mark Sep 3 snapshot dirty
rebuild Sep 3
```

## 22.3 Invalidation scope

When a new close arrives for date D, it may affect:

- D itself
- subsequent no-observation days that previously carried an older value
- dates until the next direct observation

Example:

```text
Sep 3 newly inserted close
Sep 4 no observation
Sep 5 no observation
Sep 6 already has direct close
```

Affected dates:

```text
Sep 3–Sep 5
```

Compute the affected household-cutoff range from the resolver's selected source revision, rather than copying provider dates. Stop at the next eligible canonical observation under the same effective policy; an unrelated source's observation does not terminate dependency. Missing-to-available and quality/provenance changes require invalidation even when the decimal value is unchanged.

Extend invalidation to mode/routing/binding changes, manual corrections, base-currency changes and resolver-policy upgrades. Broad invalidation is allowed for an actual policy-wide dependency change; routine inserts must remain targeted. Snapshot D also affects Analytics using D as an opening value, including the following day's returns and enclosing aggregates. Evict backend memo and refresh frontend queries when the mutation is committed.

Equivalent logic applies to FX.

## 22.4 Rebuild strategy

Snapshot rebuild must be:

- deterministic
- idempotent
- limited to affected dates
- safely repeatable after application interruption

If the app exits during repair, the next health scan should detect remaining dirty/missing snapshots.

Persistent job replay is not required; durable dirty ranges and revision-safe completion are required. A rebuild captures an input generation, and may clear dirty state only if that generation is still current. A concurrent write keeps the affected range dirty. Completeness and freshness/dirty state are separate dimensions: a snapshot can be both incomplete and dirty.

---

# 23. Data Health Center

## 23.1 Purpose

Data Health is not a generic provider-status page.

It is:

> A local integrity and repair center for the data required to calculate reliable household valuation history.

## 23.2 Local-first scan

Opening Data Health should first run a local scan.

No provider requests should happen merely because the page was opened.

## 23.3 Healthy state

```text
Data Health

✓ All data is healthy

Market data is complete through Sep 10.
All required valuations are available.
All snapshots are current.
```

## 23.4 Unhealthy state

Example:

```text
Data Health

Data incomplete since Sep 5

12 market-data gaps
3 snapshots need rebuilding

[ Repair All ]
```

## 23.5 Issue categories

At minimum:

### Missing historical instrument data

```text
AAPL
Historical prices missing Sep 2–Sep 4
Provider: Tiingo
```

### Missing historical FX

```text
USD / SGD
Historical FX missing Sep 7
Provider: Frankfurter
```

### Missing initial manual price

```text
ABC Bank Wealth Management
No manual price exists before first valuation date.
```

### Provider configuration

```text
Tiingo API key missing
US stock provider is configured as Tiingo.
```

### Provider binding missing

```text
AAPL
No Tiingo symbol is configured.
```

### Incomplete valuation

```text
Sep 8
2 required inputs are missing.
```

### Snapshot missing

```text
Sep 7–Sep 9
Daily snapshots do not exist.
```

### Snapshot outdated

```text
Sep 3–Sep 5
Historical market data changed after snapshot generation.
```

### Sync failure

```text
Tiingo
Request failed: authentication / rate limit / network
```

## 23.6 Severity

Suggested severity model:

```text
blocking
warning
info
```

Examples:

- missing historical price required by snapshot → blocking
- Tiingo configured but temporary network error with usable cached data → warning
- manual instrument carried forward for 30 days → info or no issue, depending on policy

Do not treat legitimate carry-forward as an error by itself.

---

# 24. Repair All Flow

Clicking `Repair All` should not immediately perform hidden network work.

First show the plan:

```text
Repair Market Data

Historical prices
8 ranges across 12 instruments

Historical FX
2 ranges across 3 pairs

Today's quotes
5 stale targets

Snapshots
4 dates will be rebuilt

[ Cancel ] [ Repair ]
```

The preview separates executable repairs from prerequisites. Missing key opens provider settings; missing/invalid binding opens the instrument editor; missing manual price/FX or opening anchor opens historical entry with the required cutoff. Repair All executes eligible work and reports unresolved prerequisites instead of pretending those issues are fixed. Collapse related snapshot errors beneath their underlying missing input to avoid inflated issue counts.

After confirmation, invoke the shared Sync Engine.

The Data Health Center must not implement separate repair logic.

---

# 25. Sync Progress UX

The existing final refresh result is insufficient for a potentially long historical backfill.

Implement real progress reporting.

Example:

```text
Syncing Market Data

Checking coverage                         ✓
Historical prices        8 / 12          ███████░░
Historical FX            3 / 3           ✓
Today's quotes           5 / 12          ████░░░░░
Snapshots                                 waiting
```

Detailed items:

```text
AAPL          4 historical days fetched
MSFT          up to date
QQQ           fetching Sep 2–Sep 4
SGOV          rate limited
ABC Wealth    manual · skipped
```

## 25.1 Progress model

Suggested phases:

```text
scan
plan
historical_instruments
historical_fx
latest_instruments
latest_fx
invalidate_snapshots
rebuild_snapshots
verify
complete
failed
cancelled
```

## 25.2 Backend → frontend progress

For Wails, use backend events or an equivalent progress channel.

Conceptually:

```text
marketdata.sync.started
marketdata.sync.progress
marketdata.sync.item
marketdata.sync.completed
```

The frontend should render backend-reported truth rather than fake time-based progress.

## 25.3 Concurrent job policy

Allow only one market-data sync job per household/workspace at a time.

A second equivalent request attaches to the current job. A different scope is shown as not yet scheduled; never imply that an existing single-instrument job covers Repair All. The user can rerun the broader plan after completion. vNext does not silently merge new scopes into an executing plan.

## 25.4 Job identity, cancellation and reattachment

Every job and event includes `job_id`, workspace/database identity, household ID, plan/config revision, monotonic event sequence, phase and counters. Provide `GetCurrentSyncJob` / `GetSyncJob(jobID)` snapshots as well as events so late subscribers or reopened sheets recover current truth. Events are hints to reread backend state, not the sole record of completion.

A plan preview includes as-of/config revision, requested scope, estimated request count, currently known snapshot work and unresolved prerequisites. Revalidate before execution. Snapshot counts can grow after actual responses; label preview counts as estimates. Progress counts distinguish targets, ranges and requests, and never claim all work complete merely because all fetch tasks ended.

Cancellation aborts in-flight requests when possible, prevents further scheduling and retains committed batches plus dirty ranges. Terminal outcomes distinguish succeeded, partial, failed and cancelled; successful fetch with incomplete repair is partial. Rate limits honor Retry-After or bounded backoff with visible next eligibility; no unbounded retries or automatic quota consumption after restart.

Leaving a page does not cancel the backend job. Switching/closing/restoring the database cancels and drains its job before releasing that database; late responses cannot write to the replacement workspace. A settings/mode/binding edit invalidates affected plan revisions before persistence. Provider failure does not block independent local rebuilds or unrelated providers when safe. Default request execution is sequential per provider, with at most three transient-error attempts per task and cancellation-aware waits; 401/403 stop that provider, and 429 ends its work for this job with next eligibility recorded. Cap each planned range to adapter limits, show estimated calls before execution, and never expand the approved request scope silently.

---

# 26. Refresh Result Semantics

Existing concepts such as:

```text
fetched
cached
skipped
failed
rate_limited
```

remain useful.

Extend them where required:

```text
backfilled
no_observation
unchanged
repaired
```

Do not overload one status field if the UI needs both:

```text
phase
outcome
```

Example:

```text
phase: historical_instruments
outcome: no_observation
```

---

# 27. Historical Provider Behavior

## 27.1 Tiingo

Scope:

```text
US-listed stocks and ETFs only
```

Responsibilities:

- Latest/intraday quote for US equities
- Daily EOD range history
- Authentication via configured API key
- Provider-specific symbol binding

Do not use Tiingo for Chinese equities in vNext.

## 27.2 Yahoo Finance

Responsibilities:

- US equities when Yahoo is selected
- China equities
- Other currently supported equity markets
- Latest quotes
- Historical daily prices

Yahoo remains the compatibility/default provider on upgrade.

## 27.3 Frankfurter

Responsibilities:

- Latest FX
- Historical daily FX reference rates

Frankfurter remains the only API FX provider in vNext.

### FX adapter policy

Pin vNext history and latest to the Frankfurter v2 daily API contract with default blended-source policy `frankfurter-v2-blended-v1`. Preserve provider attribution where returned and mark peg/derived-source results as such; a provider-returned reference is not necessarily a single central bank observation. Do not describe all results as ECB rates. Changes to API version, provider filters, blending/normalization policy require a new source-policy version and explicit invalidation; do not merge coverage across policies.

Store the returned date and base/quote direction. Normalize unordered pair identity for preferences and coverage, while preserving observation direction in evidence. The resolver may invert an eligible rate using decimal arithmetic (division to 24 fractional digits, half-even, before final domain rounding); a reciprocal is a derived value, not an extra raw observation. Same-currency conversion is identity 1 and needs neither fetch nor stored quote. vNext does not add application-level triangulation. Unsupported currencies/ranges are actionable health issues with manual FX entry, never successful empty coverage.

For daily references without an actual publication instant, use a documented conservative adapter eligibility boundary no earlier than the end of the source reference day, and label the timestamp basis as policy-derived. Do not present that boundary as an observed publication time. Uncertain source-day timezone/publication mapping must remain pending/unverified until the adapter's evidence contract is satisfied. Latest references preserve their actual reference date; fetch time never makes them a today observation.

---

# 28. Provider Selection Rules

Upgrade defaults to Yahoo and preserves existing per-instrument bindings. US routing changes are prospective from a recorded backend effective timestamp. Historical resolution keeps the route that was effective at its cutoff; retain old bindings and observations. Manual/provider mode history is likewise preserved.

If an instrument lacks a valid binding for the new route, report `binding_missing`; do not guess a provider symbol or silently fall back. Tiingo binding creation may propose validated metadata but activation requires a matching listing/currency/unit.

vNext does not automatically restate all history when a user changes the US provider. Historical source reassignment is out of scope; correcting an erroneous binding is an explicit effective-range repair with a preview and invalidation. Force Recheck refreshes the source already assigned to that interval.

Coverage is checked against the route/binding/source policy effective for each interval, not against whichever provider was configured most recently. Historical tasks can therefore use an older provider where that provider remains assigned and the target is currently provider-priced. Missing credentials remain an explicit repair prerequisite.

No automatic cross-provider fallback is enabled. Actual selected provenance is always displayed. Existing legacy display compatibility follows section 32.7 and never counts as coverage.

---

# 29. Current-Date Cache / TTL

Use two clocks:

- Observation freshness: economic `quoted_at` / `value_effective_at`; used for age and stale display.
- Request freshness: `last_successful_check_at` / `next_retry_at`; used to schedule provider calls.

Request state is keyed by workspace/household, target, binding revision, source-policy version and capability. A successful latest check that returns the same Friday quote on Sunday starts the request TTL, without changing Friday's observation timestamp or claiming fresh market data. Errors do not advance the successful-check timestamp; apply bounded backoff and Retry-After when provided. Normal repeat clicks honor that backoff.

Example: TTL 3 hours, successful check 45 minutes ago means no new normal latest request, even if the price is two days old. A failed refresh retains the usable old value and displays stale/error quality.

Historical requests are independent of latest TTL. Canonical observations are normally cached; section 12.6 defines recent-correction reconciliation, expiring negative caches and Force Recheck. One capability's success must not suppress another capability's needed fetch.

---

# 30. Provider Corrections

Although normal history is not repeatedly fetched, forced repair or future provider reconciliation may return a corrected historical value.

If:

```text
existing Sep 3 close = 100
new verified Sep 3 close = 101
```

then:

```text
append immutable observation revision
advance canonical pointer + mark affected snapshots dirty in the same transaction
rebuild affected snapshots against the committed generation
```

Identical normalized payloads for a canonical slot deduplicate without a new semantic revision. A changed value/basis/evidence creates a new observation ID/revision with `supersedes_observation_id`; retain prior revisions. A later reversion to an older value is still a new revision, not an overwrite. Snapshot quote references identify the exact immutable revision used. Canonical selection remains deterministic under retry and serialized batch commits.

---

# 31. Database Design

Exact table naming should follow existing repository conventions, but the following conceptual schema is recommended.

## 31.1 Instrument provider bindings

```sql
instrument_provider_bindings (
    instrument_id       TEXT NOT NULL,
    provider_key        TEXT NOT NULL,
    provider_symbol     TEXT NOT NULL,
    market              TEXT,
    currency            TEXT,
    enabled             INTEGER NOT NULL DEFAULT 1,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,

    binding_revision   TEXT NOT NULL,
    effective_from     TEXT NOT NULL,
    PRIMARY KEY (instrument_id, provider_key)
)
```

## 31.2 Observation metadata

If existing quote tables are extended:

```text
observation_kind
effective_date
provider_timestamp
fetched_at
```

Recommended kinds:

Instrument:

```text
manual
realtime
close
legacy
```

FX:

```text
manual
latest
daily_reference
legacy
```

Maintain immutable observation revisions and a canonical-slot index keyed by target, provider, binding revision, source-policy version, market date and kind. The slot points to an observation revision. Preserve existing quote IDs during migration. Bindings need a revision-history table in addition to the current-binding projection; observation evidence cannot depend on mutable current metadata.

## 31.3 No-observation coverage

Conceptual table:

```sql
market_data_day_status (
    target_type         TEXT NOT NULL,
    target_id           TEXT NOT NULL,
    provider_key        TEXT NOT NULL,
    effective_date      TEXT NOT NULL,
    status              TEXT NOT NULL,
    reason              TEXT NOT NULL,
    checked_at          TEXT NOT NULL,
    household_id        TEXT NOT NULL,
    binding_revision    TEXT NOT NULL,
    source_policy_version TEXT NOT NULL,
    next_check_at       TEXT,
    expires_at          TEXT,

    PRIMARY KEY (
        target_type,
        target_id,
        provider_key,
        household_id,
        binding_revision,
        source_policy_version,
        effective_date
    )
)
```

Possible `target_type`:

```text
instrument
fx
```

For FX, `target_id` may be a canonical pair key such as:

```text
USD/SGD
```

or an existing normalized FX identity.

## 31.4 Snapshot dirty metadata

Keep existing snapshot completeness independently from invalidation state. Extend the existing durable history-dirty mechanism with household/timezone-scoped affected ranges and an input generation; do not introduce a competing second authority.

```text
snapshot_invalidations / existing history state extension
    household_id
    from_local_date
    to_local_date
    input_generation
    reason
    created_at
```

Snapshot generation records the input generation and resolver-policy version. Saving a rebuilt snapshot and conditionally completing its dirty range is atomic. If inputs advanced during computation, discard/retry that result or keep it explicitly dirty; do not publish it as current. Concurrent UI edits and sync writes obey the same rule.

Observe the existing write gate and backup/restore boundaries. Provider HTTP and secret retrieval occur outside the database write transaction/gate; validated batches recheck workspace identity and configuration revision before committing. Schema migrations and backup verification must include revision, coverage, routing and invalidation data; OS secrets remain excluded.

---

# 32. Migration Strategy

## 32.1 Migration goals

The upgrade must preserve existing user data and avoid falsely upgrading uncertain legacy quotes into official closes.

## 32.2 Existing provider binding migration

For every existing provider-priced instrument:

```text
existing providerKey/providerSymbol
→ create InstrumentProviderBinding
```

Do not remove old columns until all code paths have migrated and tests prove the new bindings are authoritative.

A staged migration is acceptable.

## 32.3 Existing manual observations

Existing manual price observations can safely migrate as:

```text
manual
```

## 32.4 Existing provider observations

If the application cannot prove whether an old provider observation came from a daily close endpoint or latest/realtime endpoint:

```text
observation_kind = legacy
```

Do not guess.

Legacy observations may remain temporarily usable as fallback display data, but:

```text
legacy does not satisfy historical close coverage
```

Historical sync will progressively add verified closes/daily references to required historical intervals; it does not delete legacy observations.

## 32.5 Existing FX observations

Apply the same conservative rule:

```text
verified manual → manual
unverifiable provider observation → legacy
```

## 32.6 First post-upgrade health scan

After migration:

```text
compute required coverage horizon
compare verified historical data
surface gaps in Data Health
```

Do not automatically download all missing history without user confirmation.

---

## 32.7 Offline compatibility after upgrade

Choosing Later performs no network work. Preserve existing data and displayable amounts: current valuation may use migrated legacy data with `legacy/unverified` provenance until a verified source is available. Existing snapshot generations remain readable as **outdated legacy results**, not current verified results. Exclude them from claims of verified Analytics coverage; display the existing amount with an outdated/unverified indicator, or the established unavailable state where a trustworthy projection cannot be produced. Never replace a missing amount with zero or silently mix old/new policy generations in one trusted aggregate.

Migration records the new resolver-policy requirement and invalidates affected historical generations locally. Do not eagerly overwrite every legacy snapshot with missing results at startup. After user-initiated repair, publish newly built generations with their actual completeness/quality; keep missing prerequisites visible. Once a scope has migrated to the new policy, do not silently fall back to legacy snapshots when a refresh fails.

Migration fixtures must cover manual-only data, provider-only legacy data, mixed history, archived exposure, no network, Later, interrupted repair and restored backups without OS secrets. Schema version/backup compatibility changes follow repository conventions; no assumption that the old schema version remains sufficient.

---

# 33. Initial Upgrade Experience

First launch after upgrade:

```text
Market data upgrade complete.

Historical data is available for synchronization.
Nestworth found 18 historical ranges needed to make valuation history complete.

[ Review ] [ Later ]
```

`Review` opens the repair plan / Data Health Center.

The user remains in control of network synchronization.

---

# 34. Data Health Scan Algorithm

Conceptually:

```text
for each required instrument:
    inspect historical manual intervals and their initial values
    retain earlier provider-interval diagnostics even if currently manual
    suppress fetch execution for a currently manual target

    resolve effective historical routing intervals (current manual mode suppresses fetches)
    verify provider configuration
    verify provider binding

    for each eligible finalized market date and required opening anchor:
        if canonical close exists for this binding/source-policy revision:
            retrieval-covered; evaluate valuation quality separately
        else if unexpired binding-specific no_observation exists:
            retrieval-covered; evaluate valuation quality separately
        else:
            gap

    inspect today's quote TTL/state

for each required FX pair:
    inspect historical manual intervals and their initial rates
    retain earlier provider-interval diagnostics even if currently manual
    suppress fetch execution for a currently manual pair

    for each eligible finalized market date and required opening anchor:
        if canonical daily_reference exists for the effective source policy:
            retrieval-covered; evaluate valuation quality separately
        else if unexpired binding-specific no_observation exists:
            retrieval-covered; evaluate valuation quality separately
        else:
            gap

inspect valuations
inspect snapshots
inspect dirty state
```

Implementation should avoid literally iterating every date with excessive DB round trips. Load observations/status ranges efficiently and compute gaps in memory or SQL ranges.

---

# 35. Historical Sync Planner

Input:

```text
required coverage
existing observations
existing day-status coverage
provider configuration
provider capabilities
today
TTL
```

Output:

```text
SyncPlan
```

Conceptual structure:

```go
type SyncPlan struct {
    HistoricalInstrumentRanges []HistoricalInstrumentTask
    HistoricalFXRanges         []HistoricalFXTask
    LatestInstrumentTasks      []LatestInstrumentTask
    LatestFXTasks              []LatestFXTask
    SnapshotTasks              []SnapshotTask
}
```

A plan should be serializable enough for UI preview.

---

# 36. Failure Handling

Provider failure should not corrupt coverage state.

## Authentication failure

Example:

```text
Tiingo key invalid
```

Behavior:

- stop Tiingo-dependent tasks
- do not mark dates no-observation
- report blocking provider issue
- continue unrelated providers when safe

## Rate limit

Behavior:

- record task outcome `rate_limited`
- do not mark unresolved dates complete
- leave health issues outstanding
- continue other providers when safe

## Network failure

Same principle:

```text
no false coverage
```

## Partial range response

The adapter must distinguish:

```text
successful complete response with omitted dates
```

from:

```text
response completeness uncertain
```

Only the former may create `provider_no_data` status for omitted dates.

---

# 37. Transaction Boundaries

Commit per validated provider batch, not per whole job:

```text
fetch + validate outside write gate
→ recheck workspace, target mode, binding/routing revision
→ one transaction:
    append immutable observation revisions / update canonical pointers
    update coverage and request-check state
    record all affected dirty ranges and advance input generation
→ commit
→ invalidate in-memory caches / emit progress
→ rebuild from a consistent input generation
→ atomically save + conditionally clear matching dirty generation
```

A crash before commit changes nothing. A crash after commit but before events/rebuild leaves durable dirty ranges discoverable. Coverage-only quality changes also invalidate. An error must not leave a new canonical observation committed without its invalidation.

A stale plan cannot commit a response under a different database, binding, source policy or pricing mode. Cancel/replan affected tasks and retain already valid committed batches. Tests must inject failures at every boundary, including concurrent manual edits and input changes during rebuild.

---

# 38. Concurrency and Rate Limiting

Provider requests should respect per-provider limits.

Recommended architecture:

```text
provider queue
    Tiingo
    Yahoo
    Frankfurter
```

Each provider may have its own concurrency and pacing policy.

Do not launch unbounded concurrent historical requests.

The existing sequential behavior can remain as the first implementation if it simplifies correctness, as long as progress is reported. Provider-local concurrency can be optimized later.

Correctness and predictable quotas are more important than maximum speed.

---

# 39. UI Status Vocabulary

Use consistent user-facing states.

Raw observation:

```text
Realtime
Close
Manual
Daily reference
```

Resolved value:

```text
Direct
Carried forward
Missing
```

Sync:

```text
Up to date
Cached
Fetching
Backfilling
No observation
Rate limited
Failed
Skipped
```

Coverage:

```text
Complete through Sep 9
Missing Sep 2–Sep 4
```

Avoid presenting `provider_no_data` to users as “market closed” unless the reason is actually known.

---

# 40. Overview / Portfolio Integration

When valuation is incomplete:

- continue existing partial-valuation behavior
- never silently treat missing values as zero
- show a visible completeness indicator
- provide a link to Data Health

Example:

```text
Net Worth
S$1,284,320

Partial valuation
2 market-data inputs are missing
[ Fix in Data Health ]
```

When values are carried forward legitimately, the valuation may still be complete.

`carried_forward` is not equivalent to `missing`.

---

# 41. Analysis / Trend Integration

Analysis and trends should use resolved historical values and complete snapshots.

Expected semantics:

```text
direct close → valid
legitimate carried-forward → valid
missing required market data → incomplete
dirty snapshot → not trusted until rebuilt
```

Do not create artificial raw quote points to make analysis charts continuous.

---

# 42. Data Health Indicator Outside the Page

Add a small health affordance to at least:

- Market Data
- Overview

Examples:

Healthy:

```text
Data Health ✓
```

Issue:

```text
Data Health · 3 issues
```

Do not turn this into a permanently alarming red badge when the only state is a normal carried-forward weekend value.

---

# 43. Manual Pricing Rules

Manual pricing is a pricing **mode**, not merely an emergency override.

For manual instruments:

```text
pricing_mode = manual
```

Consequences:

- no provider binding required
- no API calls
- no missing-provider warning
- latest manual price is carried forward
- historical valuation before first manual price may be missing
- Data Health should identify only genuinely missing required manual history

Same principle applies to manual FX pairs.

---

# 44. Provider-Priced Instrument Rules

For provider mode:

```text
pricing_mode = provider
```

Required:

- resolvable provider from market/settings
- valid provider binding
- required historical coverage
- today's quote according to TTL

Changing from manual to provider creates requirements from its effective time and exposes outstanding earlier provider intervals; it does not reclassify historical manual intervals.

Changing from provider to manual stops future API queries but does not delete prior provider observations.

---

# 45. Switching US Provider

Example:

```text
US provider:
Yahoo → Tiingo
```

Expected:

1. Existing Yahoo observations remain.
2. Existing Yahoo symbol bindings remain.
3. Tiingo binding is resolved/created.
4. Tiingo coverage from the routing effective time is scanned.
5. Data Health may show gaps for that Tiingo interval; prior Yahoo intervals remain Yahoo.
6. User can repair.
7. New US sync operations use Tiingo.

Do not destructively rewrite historical Yahoo rows as Tiingo rows.

---

# 46. Sync Entry Points

All network synchronization should converge on one application service.

Entry points:

```text
Market Data → Sync Data
Data Health → Repair All
Per-instrument → Sync
Per-FX pair → Sync
```

Each entry point supplies a scope:

```text
all
instrument
fx_pair
health_plan
```

but uses the same planner/executor.

---

# 47. Suggested Application Services

The exact package structure can follow repository conventions, but conceptually the subsystem should contain:

```text
MarketDataRegistry
MarketDataProvider
ProviderBindingService
RequiredCoveragePlanner
CoverageRepository
MarketDataGapScanner
MarketDataSyncPlanner
MarketDataSyncExecutor
InstrumentValueResolver
FXValueResolver
SnapshotInvalidationService
DataHealthService
SecretStore
```

Avoid putting all of this into one expanded `refresh.go`.

---

# 48. Suggested Repository Refactor Direction

Current refresh behavior should be decomposed rather than continuously expanded.

Potential direction:

```text
internal/application/marketdata/
    provider.go
    registry.go
    bindings.go
    coverage.go
    resolver.go
    health.go
    sync_plan.go
    sync_execute.go
    sync_progress.go
    snapshot_invalidation.go
```

This is illustrative, not mandatory.

The important boundary is:

```text
provider adapters
≠
sync planning
≠
valuation resolution
≠
health diagnosis
```

---

# 49. Frontend Structure

Potential feature structure:

```text
frontend/src/features/marketdata/
    MarketDataPage.tsx
    InstrumentsTab.tsx
    FXRatesTab.tsx
    InstrumentRow.tsx
    FXRateRow.tsx
    QuoteHistorySheet.tsx
    SyncButton.tsx
    SyncProgressSheet.tsx
    RepairPlanSheet.tsx

frontend/src/features/data-health/
    DataHealthPage.tsx
    DataHealthSummary.tsx
    HealthIssueGroup.tsx
    HealthIssueRow.tsx
    RepairPlanSheet.tsx
```

Existing Investments components that remain useful should be moved/reused rather than duplicated.

---

# 50. Wireframe — Market Data

```text
┌──────────────────────────────────────────────────────────────┐
│ Market Data                                                  │
│ Manage instrument prices, historical data and FX rates.      │
│                                                              │
│ Data updated Sep 10, 10:32         Data Health ✓  [Sync Data]│
├──────────────────────────────────────────────────────────────┤
│ [ Instruments ] [ FX Rates ]                                 │
├──────────────────────────────────────────────────────────────┤
│ Search instruments...                         [+ Instrument]  │
│                                                              │
│ AAPL                                      US Equity · USD     │
│ Apple Inc.                                  Tiingo            │
│ $234.42                                                     │
│ Realtime · 8 min ago                                         │
│ Coverage complete through Sep 9        [History] [Edit]       │
│ ──────────────────────────────────────────────────────────── │
│ SGOV                                      US ETF · USD        │
│ $100.51                                                     │
│ Sep 9 close · carried forward                                │
│ Coverage complete through Sep 9        [History] [Edit]       │
│ ──────────────────────────────────────────────────────────── │
│ ABC Bank Wealth                            Manual · CNY        │
│ ¥1.041                                                      │
│ Manual Sep 8 · carried forward                                │
│                                      [Set Price] [History]    │
└──────────────────────────────────────────────────────────────┘
```

---

# 51. Wireframe — FX Rates

```text
┌──────────────────────────────────────────────────────────────┐
│ Market Data                                                  │
├──────────────────────────────────────────────────────────────┤
│ [ Instruments ] [ FX Rates ]                                 │
├──────────────────────────────────────────────────────────────┤
│ USD / SGD                                      Frankfurter   │
│ 1.2834                                                       │
│ Latest · 20 min ago                                           │
│ Coverage complete through Sep 9                     [History] │
│ ──────────────────────────────────────────────────────────── │
│ CNY / SGD                                      Frankfurter   │
│ 0.1802                                                       │
│ Sep 9 daily reference · carried forward                       │
│ Coverage complete through Sep 9                     [History] │
│ ──────────────────────────────────────────────────────────── │
│ USD / CNY                                      Manual        │
│ 7.1220                                                       │
│ Manual Sep 8 · carried forward                      [History] │
└──────────────────────────────────────────────────────────────┘
```

---

# 52. Wireframe — Sync Progress

```text
┌──────────────────────────────────────────────────────────────┐
│ Syncing Market Data                                          │
│                                                              │
│ ✓ Checking coverage                                          │
│                                                              │
│ Historical prices                            8 / 12           │
│ ████████████████████░░░░░░░░                                 │
│                                                              │
│ AAPL       Sep 2–Sep 4      3 closes fetched                 │
│ MSFT       Sep 2–Sep 4      complete                         │
│ QQQ        Sep 2–Sep 4      fetching...                      │
│ SGOV       Sep 2–Sep 4      waiting                          │
│                                                              │
│ Historical FX                                3 / 3 ✓          │
│ Today's quotes                               5 / 12           │
│ Snapshots                                    waiting          │
│                                                              │
│                                          [Run in background*] │
└──────────────────────────────────────────────────────────────┘
```

`Run in background` here means within the currently running desktop application process, not an asynchronous cloud task. It is optional and can be omitted in vNext.

---

# 53. Wireframe — Data Health

```text
┌──────────────────────────────────────────────────────────────┐
│ Data Health                                                  │
│                                                              │
│ Market data is incomplete since Sep 5.                       │
│                                                              │
│ 12 market-data gaps · 3 snapshots need repair                │
│                                               [Repair All]    │
├──────────────────────────────────────────────────────────────┤
│ Historical Prices                                      8     │
│                                                              │
│ AAPL          Missing Sep 6–Sep 8         Tiingo             │
│ QQQ           Missing Sep 7               Tiingo             │
│ ...                                                          │
├──────────────────────────────────────────────────────────────┤
│ FX Rates                                                2     │
│ USD/SGD       Missing Sep 7–Sep 8         Frankfurter        │
├──────────────────────────────────────────────────────────────┤
│ Snapshots                                               3     │
│ Sep 6         Incomplete                                  │
│ Sep 7–Sep 8   Outdated after historical backfill          │
└──────────────────────────────────────────────────────────────┘
```

---

# 54. Wireframe — Repair Plan

```text
┌──────────────────────────────────────────────────────────────┐
│ Repair Market Data                                           │
│                                                              │
│ Nestworth will request:                                      │
│                                                              │
│ Historical prices                                            │
│ 8 ranges across 12 instruments                               │
│                                                              │
│ Historical FX                                                │
│ 2 ranges across 3 pairs                                      │
│                                                              │
│ Today's quotes                                               │
│ 5 stale targets                                              │
│                                                              │
│ Snapshots                                                    │
│ 4 dates will be rebuilt                                      │
│                                                              │
│                               [Cancel] [Repair]               │
└──────────────────────────────────────────────────────────────┘
```

---

# 55. Domain Invariants

The implementation should encode the following as explicit tests/invariants.

1. A carried-forward value is never persisted as a raw provider observation.
2. A past realtime quote does not satisfy official historical close coverage.
3. Only verified finalized response subranges may create expiring no-observation markers; HTTP success alone is insufficient.
4. A failed provider request may never create no-observation markers.
5. Manual pricing mode never calls a provider.
6. Provider-specific no-observation status does not suppress another provider.
7. Latest request checks obey TTL independently from quote age; history uses its own reconciliation/expiry policy.
8. An observation whose provider effective date is yesterday is not stored as today's realtime observation.
9. Observation or coverage changes atomically invalidate snapshots whose resolved amount, quality or evidence changes.
10. Missing market data is never treated as zero.
11. Legitimate carry-forward can still produce a complete valuation.
12. Data Health scan itself performs no network requests.
13. Repair and normal Sync use the same planner/executor.
14. Re-running a completed sync is idempotent.
15. Existing unverifiable provider data is not silently relabeled as official close during migration.

---

# 56. Test Matrix

## 56.1 Historical instrument tests

- Range returns every trading date.
- Range omits weekend dates.
- Range omits a holiday.
- Range request fails.
- Range request is rate limited.
- Existing close skips fetch outside the due recent-correction window and Force Recheck.
- Unexpired no-observation skips normal fetch outside the due reconciliation window.
- Existing realtime-only historical date still backfills.
- Provider corrects an existing close.
- Provider switch splits routing intervals and scans the new provider only for its assigned interval plus opening dependencies.
- Missing provider binding is reported.

## 56.2 Today tests

- Successful latest check inside TTL, even if returned market data is old.
- Latest check outside TTL and failed-check backoff, independent of observation freshness.
- Market not open yet.
- Weekend.
- Holiday.
- Latest endpoint returns prior trading-day timestamp.
- Latest endpoint fails but prior close exists.
- No prior close exists.

## 56.3 Manual tests

- Manual instrument with current value.
- Manual instrument carried forward.
- Manual instrument missing first value.
- Manual FX carried forward.
- Switching provider → manual.
- Switching manual → provider.

## 56.4 FX tests

- Frankfurter historical range.
- Weekend omissions create no-observation statuses.
- Missing daily rate resolves using prior reference rate.
- Base currency change creates new required range.
- Manual FX suppresses provider sync.

## 56.5 Snapshot tests

- New historical close changes the first household snapshot whose cutoff makes it eligible.
- New close changes following carried-forward days.
- Next eligible canonical close under the effective policy stops the dependent snapshot range.
- New FX history changes affected snapshots.
- Repair interruption leaves remaining dirty snapshots discoverable.
- Rebuild is idempotent.

## 56.6 Migration tests

- Existing Yahoo binding migrates.
- Existing manual quote migrates as manual.
- Existing unknown provider quote migrates as legacy.
- Legacy does not satisfy historical close coverage.
- Upgrade defaults US provider to Yahoo.
- Tiingo key absence does not break users who remain on Yahoo.

---

# 57. Telemetry / Logging

Even for a local-first app, structured logs are valuable.

Each sync job should log:

```text
job_id
scope
provider
target
range
request outcome
insert count
no-observation count
changed observation count
invalidated snapshot range
duration
error category
```

Never log API secrets.

Provider HTTP diagnostics should redact authentication headers/query parameters.

---

# 58. Performance Considerations

Avoid:

- one query per day
- one DB round trip per required calendar date
- rebuilding every historical snapshot after every insert
- repeatedly requesting no-observation weekends
- downloading history before the instrument becomes relevant to the household

Prefer:

- range queries
- batched persistence
- range-based gap calculation
- affected-range snapshot invalidation
- provider-local request throttling
- local health scans

---

# 59. Security Considerations

- Treat Tiingo API key as a secret.
- Never include it in logs.
- Never include it in exported household data by default.
- Never surface the full key in UI after saving.
- Provide replace/remove actions.
- Provider errors shown in UI must redact credentials.
- If diagnostics are copied/exported, secret values must not be included.

---

# 60. Product Decisions Confirmed for vNext

The following decisions are considered locked unless implementation uncovers a blocker:

1. Version scope is **Market Data subsystem upgrade**.
2. Unified page name is **Market Data**.
3. Instruments and FX are tabs on the same page.
4. Historical raw data stores only real provider/manual observations.
5. Non-trading/no-data dates are represented through separate coverage/day-status records.
6. `realtime` and historical `close` are distinct instrument semantics.
7. Tiingo is used only for US equities.
8. US equity provider is user-selectable between Tiingo and Yahoo.
9. China and other stock markets use Yahoo in vNext.
10. FX uses Frankfurter.
11. Manual instruments remain fully manual and are never API-queried.
12. Manual values carry forward within their effective manual-mode interval until superseded.
13. Weekend/holiday values are not persisted as synthetic prices.
14. Live valuation may use a prior close and display `carried forward`, retaining actual market time and quality.
15. Today's lack of realtime data does not immediately create a permanent no-observation record.
16. Data Health is an independent left-navigation page; automatic repair and user prerequisites are distinguished.
17. Data Health and Market Data share the same Sync Engine.
18. Initial historical coverage starts when the instrument/FX first becomes relevant to household valuation.
19. Historical requests should use date ranges rather than per-day calls.
20. Historical repair must be idempotent.
21. New historical data may invalidate existing snapshots.
22. Snapshot repair should be limited to affected dates.
23. A local health scan should not itself perform provider network requests.
24. Historical pricing modes and provider routes are effective-dated; switching providers does not automatically restate history.
25. Raw observations are immutable; canonical corrections create revisions.
26. Every observation/coverage mutation commits with its invalidation and input generation.
27. Request TTL is separate from observation freshness.
28. Force Recheck, expiring no-data and bounded recent-correction reconciliation are in scope.
29. Snapshot eligibility uses economic timestamps and household cutoff, not market-date equality.
30. Coverage evidence, amount availability and valuation quality are independent.
31. US routing includes listed stocks and ETFs; no currency-based market guessing.
32. Upgrade Later preserves explicitly unverified legacy display without background downloads.

---

# 61. Recommended Implementation Phases

## Phase 0 — Contract fixtures and adapter qualification

Before production schema/provider integration, encode section 66 fixed-clock cases with an independent expected-value oracle. Qualify session timestamps, publication completeness, price basis, currency units and response limits for Tiingo, Yahoo and Frankfurter. Use sanitized recorded fixtures in default tests; controlled live checks are separately reported with request counts and never run in the default suite.

### Exit criteria

The adapter mapping and dependency contract is executable for the selected first vertical slice. Any unsupported market/timestamp/basis returns an explicit unsupported/uncertain result rather than a guessed price or verified coverage. Provider qualification failures block that provider's release scope; they do not silently alter the financial rules.

## Phase 1 — Domain and persistence foundation

Implement:

- observation kinds
- effective date
- legacy migration behavior
- provider-specific instrument bindings
- no-observation coverage/day-status storage
- provider-neutral history interfaces
- Tiingo secret-store abstraction
- required-coverage model

No major UI redesign yet.

### Exit criteria

- Schema migration passes.
- Existing app still opens existing data.
- Existing Yahoo/latest behavior still works.
- Manual pricing still works.
- Historical domain types are testable.

---

## Phase 2 — One end-to-end historical repair slice

Implement the shared gap/anchor planner, batch coverage contract, immutable revision persistence, resolver, atomic dirty marking, generation-safe rebuild and Analytics invalidation with Tiingo US history and a manual FX fixture. Include a minimal backend command/probe for plan, execute and verify; a redesigned UI is not required yet.

### Exit criteria

One Starting-point-to-Analytics fixture proves:

```text
provider response → immutable observation/coverage commit
→ dirty range → cutoff-safe snapshot rebuild → refreshed Analytics DTO
```

It includes a weekend starting anchor, a delayed close, a correction with unchanged rounded amount, a crash after commit, and repeated sync. Independent expected amounts and source revisions agree; repeated execution causes no duplicate semantic writes or unnecessary requests.

## Phase 3 — Provider breadth and policy integration

Extend the proven vertical slice with Yahoo historical/latest, Frankfurter v2, Tiingo latest, historical routing/mode intervals, scoped Force Recheck, current-check TTL, recent-correction reconciliation and negative-cache expiry. Preserve all existing supported instrument management/search/latest behavior.

### Exit criteria

All required providers pass deterministic contract fixtures and separately recorded live capability checks. Splits/dividends, historical manual/provider transitions, cross-timezone cutoffs, FX inversion, archived exposure and legacy/offline migration satisfy section 66. New dirty state is visible immediately to Analytics before rebuild; no stale memo is presented as trusted current data.

---

## Phase 4 — Unified Market Data UI

Implement:

- remove duplicate page structure
- Instruments tab
- FX Rates tab
- merged instrument-management capabilities
- source configuration display
- current resolved value
- raw history view
- Sync Data action
- live sync progress

### Exit criteria

The old separate Investments/Market Data UX is no longer required.

---

## Phase 5 — Data Health Center

Implement:

- local health scan
- issue model
- issue grouping
- repair plan preview
- Repair All
- health indicator from Overview and Market Data
- post-repair verification

### Exit criteria

A user can identify provider/coverage/snapshot issues and either execute repair or follow a specific prerequisite/manual action without understanding internal database state. Pending publication and unsupported provider coverage are accurately reported, not promised to be automatically repairable.

---

## Phase 6 — Cleanup and hardening

Implement:

- remove obsolete refresh/page code
- remove old provider-symbol fields if fully migrated
- performance tuning
- provider error normalization
- full regression tests
- documentation
- migration fixtures
- diagnostics/log redaction

---

# 62. Suggested Delivery Slices for Coding Agents

A practical dependency-ordered breakdown (independent adapter work can proceed after shared contracts pass):

```text
1. contract-fixtures-and-adapter-qualification
2. schema-observation-revisions-and-canonical-index
3. schema-effective-routing-bindings-and-coverage
4. schema-generation-safe-history-invalidation
5. secret-store-tiingo-and-restore-status
6. provider-capability-batch-contract
7. required-coverage-and-opening-anchor-planner
8. cutoff-resolver-and-quality-contract
9. tiingo-history-to-snapshot-to-analytics-vertical-slice
10. yahoo-history-and-price-basis-validation
11. frankfurter-v2-history-and-fx-resolution
12. latest-check-ttl-force-recheck-and-reconciliation
13. sync-job-query-events-cancel-and-workspace-lifecycle
14. unified-market-data-ui
15. data-health-prerequisites-and-repair-ui
16. migration-offline-backup-regression
17. native-acceptance-performance-and-cleanup
```

Each slice includes relevant section 66 cases and avoids temporary semantics contradicting the final invariants. Slices 2–8 establish the shared contract before slice 9; provider expansion follows the end-to-end proof. UI work consumes stable Wails DTOs. Do not defer invalidation, price basis or migration semantics until cleanup.

---

# 63. Acceptance Criteria

The version is complete when all of the following are true.

### Product

- Only one Market Data management page remains.
- Instruments and FX are both available there.
- Data Health exists as a separate navigation page.
- User can select Tiingo or Yahoo for US equities.
- Tiingo key can be configured safely.
- Sync shows real progress.

### Historical data

- Required historical instrument closes can be backfilled.
- Required historical FX daily rates can be backfilled.
- Historical range requests are batched.
- Weekends/holidays do not generate fake quotes.
- Checked no-data dates are remembered.
- Failed requests remain retryable.
- Old realtime quotes are superseded by verified historical closes for past-date valuation.

### Manual data

- Manual instruments are never queried from a provider.
- Manual prices carry forward.
- Manual FX carries forward.
- Missing initial manual values are diagnosable.

### Valuation

- Closed-day snapshots use the latest eligible finalized close/reference at household cutoff.
- Live valuation uses the latest eligible realtime/close instant, independent of household date equality.
- Prior-close fallback carries explicit source time and verification quality.
- Missing data is not zero.
- Carried-forward values can still produce a complete valuation.

### Snapshots

- Historical corrections invalidate affected snapshots.
- Only affected ranges are rebuilt.
- Dirty/missing/incomplete snapshots appear in Data Health.
- Repeated repair is safe.

### Migration

- Existing data is preserved.
- Existing Yahoo users remain functional without Tiingo.
- Existing unverifiable provider observations are not misclassified as closes.
- First upgrade does not automatically trigger a large background history download.

---

# 64. Final Architecture Summary

The final subsystem should conceptually look like this:

```text
                    ┌─────────────────────┐
                    │   Market Data UI    │
                    └──────────┬──────────┘
                               │
                    ┌──────────▼──────────┐
                    │    Sync / Repair    │
                    │      Planner        │
                    └──────────┬──────────┘
                               │
              ┌────────────────┼────────────────┐
              │                │                │
      ┌───────▼───────┐ ┌──────▼──────┐ ┌─────▼──────────┐
      │ Required      │ │ Gap/Coverage │ │ Today's TTL   │
      │ Coverage      │ │ Scanner      │ │ Scanner       │
      └───────────────┘ └─────────────┘ └───────────────┘
                               │
                    ┌──────────▼──────────┐
                    │   Sync Executor     │
                    └──────────┬──────────┘
                               │
          ┌────────────────────┼────────────────────┐
          │                    │                    │
   ┌──────▼─────┐       ┌──────▼─────┐      ┌──────▼──────┐
   │ Tiingo US  │       │ Yahoo      │      │ Frankfurter │
   └──────┬─────┘       └──────┬─────┘      └──────┬──────┘
          │                    │                    │
          └────────────────────┼────────────────────┘
                               │
                 ┌─────────────▼─────────────┐
                 │ Observations + Coverage   │
                 └─────────────┬─────────────┘
                               │
                  ┌────────────▼────────────┐
                  │ Resolved Value Layer   │
                  │ direct / carried / miss│
                  └────────────┬────────────┘
                               │
          ┌────────────────────┼────────────────────┐
          │                    │                    │
   ┌──────▼──────┐      ┌──────▼──────┐     ┌──────▼──────┐
   │ Valuation   │      │ Snapshots   │     │ Analysis    │
   └─────────────┘      └──────┬──────┘     └─────────────┘
                               │
                    ┌──────────▼──────────┐
                    │    Data Health      │
                    │ diagnose + repair   │
                    └─────────────────────┘
```

The essential separation is:

```text
Provider observations
        ≠
Coverage state
        ≠
Resolved valuation values
        ≠
Snapshots
```

Keeping those four concepts distinct is what makes the subsystem reliable.

---

# 65. Implementation Principle

When implementation choices are ambiguous, prefer the option that preserves these properties:

> **Local-first, explicit provenance, no synthetic source data, idempotent repair, provider-neutral application logic, and reproducible historical valuation.**

That is the core contract of the vNext Market Data subsystem.


---

# 66. Implementation Planning Gates and Acceptance Evidence

## 66.1 Required regression matrix

These are normative cases in addition to section 56. Use a fixed backend clock, household history timezone, fixture manifest and independent expected amounts. An adapter fixture proves mapping behavior, not current provider availability.

| ID | Scenario | Required result |
|---|---|---|
| MD-01 | Singapore midnight while US session is still active; New York DST and early-close fixtures | Live value uses latest eligible market instant. Closed snapshot never consumes a later close; uncertain timing cannot become verified coverage. |
| MD-02 | Successful empty history before EOD publication; complete, truncated and malformed responses | Pending/uncertain remain retryable; only verified finalized subranges get expiring no-data. Invalid batches leave no writes. |
| MD-03 | Recent close correction, expired no-data, old-range Force Recheck | Bounded recheck discovers changes; unchanged content deduplicates; failed recheck preserves old usable data. |
| MD-04 | Split/reverse split and cash dividend on Tiingo/Yahoo | Ledger quantities and price basis agree; no duplicate split/dividend effect in net worth or Analytics attribution. |
| MD-05 | Sunday Starting point with holding/foreign cash, archived historical asset | Planner fetches preceding anchor and historical exposure; no future-value seeding; exhausted lookback reports actionable missing anchor. |
| MD-06 | Provider→manual→provider; Yahoo→Tiingo; corrected symbol | Historical modes/routes remain effective-dated; current manual suppresses all fetches; old binding coverage cannot satisfy new identity. |
| MD-07 | Weekend successful latest check and repeated Sync; latest/history independent | No repeated latest request inside check TTL; quote timestamp stays unchanged; history still fetches when needed. |
| MD-08 | Crash before/after batch commit and before rebuild/event delivery | Either no mutation or durable observations plus dirty ranges; restart local scan rediscovers unfinished repair. |
| MD-09 | Manual edit/coverage-only update during rebuild; identical rounded totals | New generation remains dirty until rebuilt; quality and exact-value changes propagate despite equal rounded totals. |
| MD-10 | Correction to snapshot D and its carried dependents | Stop at next eligible canonical dependency; invalidate next-day return and enclosing Analytics caches; unrelated snapshots stay unchanged. |
| MD-11 | Revision 100→101→100; repeated ingestion | Old snapshot evidence still resolves to original immutable values; semantic reversions get a new revision; retries do not duplicate revisions. |
| MD-12 | FX reciprocal, same currency, source-policy change, unsupported pair | Exact decimal inversion, identity 1 without query, version-separated coverage, actionable manual fallback; no synthesized raw reciprocal. |
| MD-13 | Upgrade offline/Later, interrupted repair, restored backup without secret | Existing data survives with explicit legacy/dirty quality, no startup network, no false verified Analytics, missing key does not destroy cache. |
| MD-14 | Attach after events, reopen sheet, duplicate/different scope, cancel, database restore | Job query reconstructs truth; scope is explicit; cancelled/old-workspace responses cannot mutate replacement data. |
| MD-15 | Missing key/binding/manual input; partial provider failure and 429 | Correct prerequisite action and localized code; eligible work can complete partially; request budget/backoff respected. |
| MD-16 | Missing, verified zero, unverified carried zero/nonzero across Wails and all Analytics views | Null is not zero; quality/AmountStatus survives DTO/bindings/UI; frontend does not recompute financial values. |
| MD-17 | Clean migration and backup/restore of revision/routing/dirty state | Original IDs/evidence preserved; schema validation and supported-version checks pass; secrets excluded. |

## 66.2 Release verification and scope

- Run domain/application/SQLite tests, Wails mapping and generated-binding consistency checks, frontend tests/typecheck/lint/build and migration/backup verification appropriate to each slice. Run the full regression suite before release.
- Database quantitative probes must exercise the real persistence and rebuild flow, with fixed source responses and independent expected values. Include existing Analytics residual and exact-decimal regression gates; never hide genuine mismatches by tolerance changes.
- Native Wails smoke covers key storage (including unavailable/locked store), event reattachment/cancel, database switch, source settings, history, repair and keyboard flows. Localized missing price/FX/quality states require en, zh-CN and zh-TW evidence, including calendar year/month/day surfaces.
- Performance remains measured on Apple M3 Pro normal mode. Preserve the existing Analytics cold-3y <3s target; Linux/forced single-thread results remain non-blocking references. Any new threshold exception must be explicit. Record local scan/query count, request count, rebuild count, wall time and allocation data on 3-year fixtures; do not use live-provider network latency as a deterministic unit-test gate.
- Sync idempotency acceptance means no duplicate semantic writes, no normal repeat network requests inside the defined cache/recheck windows, and no rebuild of unaffected ranges. Forced or scheduled correction checks are intentional requests and must be separately counted.
- Bind each result to commit, schema/resolver-policy versions, fixture identity, clock/timezone, commands and artifact paths. Mark unrun native/live/database cases NOT RUN, never infer them from unit tests or screenshots of another path. No secrets appear in fixtures, logs or exported diagnostics.

## 66.3 Planning handoff

Implementation tasks must reference the owning section, affected persistence/API/UI boundaries and MD case IDs. No unresolved product choice is delegated to an adapter implementer: unsupported provider semantics block that adapter, while the policy above remains stable. Physical table naming, package placement and equivalent generation-safe SQL mechanisms may follow repository conventions without changing these contracts.

Evidence checked during the preceding design review and incorporated in this revision (2026-09-10):

- [Tiingo EOD documentation](https://www.tiingo.com/documentation/end-of-day): raw versus adjusted fields, publication and correction behavior. Verify adapter fixtures and actual entitlement during implementation; documentation is not a live capability PASS.
- [Frankfurter v2 documentation](https://frankfurter.dev/): daily history, default blended sources and provider attribution. Pin and test the chosen source policy.
- Existing repository contracts: household-cutoff snapshots, effective-dated instrument/FX preferences, immutable quote references, transactional history dirty marking and authoritative Analytics DTOs. Reuse and extend these boundaries rather than creating parallel authorities.
