# Available Funds, Term Deposits, and Locked Products

- **Owner:** Walt / Nestworth product
- **Status:** Planned; implementation specification, not implemented behavior
- **Design date:** 2026-09-20
- **Verified code baseline:** `980151c`
- **Audience:** An implementation agent working sequentially through small, verifiable tasks
- **Scope:** Desktop-only Wails feature, including persistence, accounting, forecasts, UI, export, and tests
- **Validation boundary:** Repository inspection informed this plan. No feature code, migration, prototype, or acceptance test has been implemented or run for this feature.

## 1. Objective and decisions

Answer: **“Of the assets I currently own, how much cash could I access by a chosen date, how much is reserved, and what actions or costs would be involved?”**

This is a liquidation/availability scenario based on today's assets. It is not a forecast of future salary, spending, market returns, or future FX rates.

The following decisions are binding for this implementation:

1. Keep Account as the real-world container. Support multiple deposits/products inside one Holdings Account alongside its cash balances.
2. Reuse Instrument + Holding + manual quotes for financial value. Add contract and availability metadata; do not create a second asset valuation ledger.
3. Use the existing `bank_investment_product` instrument type. Distinguish `term_deposit` and `locked_product` in the new contract record. A new account type or instrument enum is unnecessary.
4. Each managed contract owns one private Instrument and one Holding. Holding quantity is `1` while open and `0` after settlement. Its manual unit price represents the value of the entire contract.
5. A term deposit's current carrying value is principal. Future interest is a forecast until actually received. A locked product's carrying value is its latest explicitly recorded total valuation.
6. Keep contract schedules separate from actual accounting. Reaching a date never creates cash, income, a sale, or a renewal automatically.
7. Use current local valuation/FX inputs. Go owns all financial calculations; React displays authoritative DTOs.
8. All product operations are previewed and committed atomically, with persistent idempotency. A failed renewal must never leave only its redemption committed.
9. Wails frontend/backend ship together: replace contracts directly and regenerate bindings. Existing SQLite data and backup verification still require deliberate handling.
10. All phases below are required to finish the feature. An intermediate read-only page is not completion.

## 2. Verified integration points

Use these code paths as starting points. Some older architecture prose still mentions retired APIs or old schema behavior; current code and tests take precedence.

| Concern | Existing implementation | Consequence |
| --- | --- | --- |
| Account combinations | `internal/domain/account_type.go` | Bank accounts can use `holdings`; existing balance/manual-value modes are immutable |
| Financial positions | `internal/domain/portfolio.go` | Bank investment products already exist; quantity and quotes can support contract-sized positions |
| Component valuation | `internal/application/valuation.go`, `internal/domain/valuation.go` | Consume per-component native amounts and exact base conversion, never account totals plus their children |
| Activities and effects | `internal/domain/change.go` | Buy/sell principal, cash interest, fees, and reconciliation already have distinct semantics |
| Recording and corrections | `internal/application/change_service.go`, `history_changes.go`, `change_mutation.go` | Reuse pure preview/building logic; do not compose separately committed public commands |
| Atomic persistence | `internal/infrastructure/sqlite/change_repository.go` | `commitActivityTx` is a useful transaction-local building block |
| Write coordination | `internal/application/write_coordinator.go` | Lock order: write permit → ledger lock → SQLite transaction |
| Analysis association | `internal/application/analysis_classifier.go` | Ordinary cash interest belongs to cash; product-linked interest needs explicit holding/instrument association |
| Schema | `internal/infrastructure/sqlite/database.go`, `schema.sql`, `schema_verify.go` | Current schema is 11; implement an explicit next-version migration |
| Export | `internal/infrastructure/sqlite/export_repository.go`, `internal/application/json_export.go` | Dataset projection is explicit; new tables are not exported automatically |
| IPC registration | `cmd/nestworth/main.go` | Register the new service only when the live business service is available |
| Navigation | `frontend/src/app/navigation.ts`, `frontend/src/App.tsx` | Add the page through typed navigation and existing startup gates |
| Cache invalidation | `frontend/src/queries/invalidation.ts` | Add liquidity reads to all relevant financial invalidation paths |

Before implementation, inspect the named files again and record any baseline drift. Do not reintroduce the 33 APIs removed in `980151c`.

## 3. Deliverable scope

### Included

- Available Funds page and Overview entry card.
- Today, 7-day, 30-day, and custom-date cumulative availability.
- Per-component rules, explicit unknown/unavailable states, and simple fixed-money reservations.
- Opening a new term deposit or locked product from tracked account cash.
- Recording an already-owned product without pretending it was bought today.
- Manual locked-product valuation updates.
- Term-deposit interest estimates, with explicit calculation assumptions or a user-entered maturity interest amount.
- Full redemption, early full withdrawal where allowed, separately received interest, and renewal into a new contract.
- In-app maturity reminders, operation history, and safe grouped undo.
- Migration, schema verification, JSON export, backup round-trip tests, localization, and native smoke acceptance.

### Explicitly deferred

- Partial product subscription/redemption, multiple lots inside one managed contract, product transfers between accounts, and automatic reinvestment.
- Variable-rate schedules, compounding, scheduled periodic-interest forecasts, tax engines, and provider-specific contractual formulas.
- Holiday/exchange calendars, intraday cutoffs, and jurisdiction-specific settlement promises.
- Full goal budgeting, reservation transfers between assets, forecasts of future wages/expenses/debt installments, and historical “as of last year” liquidity reports.
- Live product feeds, OCR/import, OS notifications, scheduled background actions while the app is closed, and bank transactions.
- Zero-proceeds product write-off: existing trade commands require positive gross proceeds. Reject this explicitly; do not fabricate a positive sale or silently archive a loss.
- Automatic conversion of an existing Simple Account to Holdings mode.

These exclusions must be visible in the affected forms. They must not produce apparently supported controls that silently approximate a different operation.

## 4. User flows

### 4.1 Open a new product from cash

From an eligible Account detail, choose **Add product → Term deposit / Locked product → Buy/open now**.

1. Select the contract currency and principal/subscription amount.
2. Show available account cash in that currency. No automatic FX conversion.
3. Enter the contract dates, value assumptions, and access rules.
4. Preview cash decrease, new product value, fees if any, and resulting account value.
5. Confirm once. Success returns the product detail and refreshes financial reads.

Example: account cash 150,000; deposit principal 100,000; opening fee 0. After opening: cash 50,000 + deposit 100,000 = 150,000. It is not 250,000 and creates no external contribution.

Only active asset-side Holdings Accounts are eligible, excluding `cash_on_hand`. Existing legal bank/investment/brokerage/pension/other Holdings Accounts may hold products. Account-level restrictions still apply to liquidity defaults.

For a Simple Account, explain that product details require a Holdings Account. Offer navigation to create one, followed by the existing explicit transfer flow. Do not silently create an overlapping balance or move historical data.

### 4.2 Record an already-owned product

Choose **Record existing product** and enter original contract dates, principal/cost, and current value.

- Record the position at the current confirmed effective time using existing already-existed/reconciliation semantics and cost evidence.
- Do not deduct account cash or create a buy, contribution, or invented pre-origin history.
- Require acknowledgement: “The cash balance shown in this account excludes this product.” Show the increase in recorded assets in the preview.
- If the tracked cash currently includes the product amount, use Buy/open from cash or reconcile the cash explicitly first; never count both.
- Original contract start may predate History Origin. Financial evidence starts when recorded; contract dates do not backfill ownership or valuations.

### 4.3 Review available funds

Open **Available funds** from Overview or navigation. Select a date and optionally enable **Consider early withdrawal**.

The page shows usable subtotal, reserved amount applied to that subtotal, and unreserved subtotal. Each row explains its timing, assumptions, required action, and missing inputs.

### 4.4 Confirm receipt / early redemption

From product detail or a due reminder:

1. Enter actual receipt date/time and actual amounts.
2. For a deposit, enter returned principal, interest received, and fee separately.
3. For a locked product, enter gross redemption proceeds and fee; investment gains already embedded in proceeds are not a second interest entry.
4. Preview the product closing and cash increase.
5. Confirm receipt only after money actually arrived. A submitted redemption request is not cash.

Settlement is into the parent account's cash in the product currency. A subsequent transfer uses the existing Transfer workflow.

### 4.5 Renew

Choose **Confirm receipt and renew**. Review old receipt amounts, then a new contract, principal, dates, and any opening fee.

The old contract closes and the new one opens in one transaction. New principal can differ from old proceeds, subject to available cash after receipt. Uninvested interest remains cash. The new contract gets new IDs and a link to its predecessor.

### 4.6 Correct mistakes

- Names, notes, and forecast assumptions: edit with a revision check.
- Principal, owner account, currency, financial effective time, and settled receipt facts: do not edit in place.
- Undo the latest eligible grouped financial operation, then re-enter it correctly.
- History shows the group and routes Undo to the product operation. Individual children cannot be fixed or reversed independently.
- If later dependent activity makes undo unsafe, show a concrete conflict and preserve all data. Do not cascade silently.

## 5. Financial representation

### 5.1 Managed contract identity

For each contract, create a private `bank_investment_product` Instrument with Manual source and one Holding. The product owns the instrument; another account cannot buy or create a holding in it.

| Item | Term deposit | Locked product |
| --- | --- | --- |
| Holding quantity | 1 open / 0 closed | 1 open / 0 closed |
| Opening quote | Principal | Subscription amount for new purchase; confirmed current value for existing position |
| Cost basis | Principal plus actual opening fee; explicit total cost when recording an existing position | Subscription cost plus actual opening fee; explicit total cost when recording an existing position |
| Later value | Principal, until actual settlement | Latest user-entered total valuation |
| Future proceeds | Principal plus estimated unpaid maturity interest, less configured estimate of exit cost | Latest valuation less configured exit cost; expressly an estimate |
| Actual settlement | Sell returned principal; separately record actual interest | Sell actual gross proceeds |

One contract-sized position makes different maturities and withdrawal rules independently addressable. The UI shows **Principal / Current value**, not “1 share at 100,000”. Generic portfolio calculations can still use quantity × price.

Do not create synthetic daily quotes for expected interest. A forecast never changes net worth, cost basis, historical snapshots, or investment returns.

### 5.2 Existing components

Create one liquidity source per current valuation component:

| Source kind | Stable key | Meaning |
| --- | --- | --- |
| `account_value` | `account_value:<accountId>` | One Simple Account value |
| `account_cash` | `account_cash:<accountId>:<currency>` | One cash currency in a Holdings Account |
| `holding` | `holding:<holdingId>` | One holding, including a managed contract |

Use typed source references internally. Derive keys in Go, not by concatenating arbitrary frontend strings for SQL lookups.

Never add an Account's total on top of its components. Ignore liability accounts and archived accounts in availability totals. Keep exclusion reasons visible. Zero quantity holdings do not contribute.

`IncludeInNetWorth` and the old `IncludeInLiquidAssets` flag are not availability rules. Do not repurpose either. Include all active asset-side sources unless their liquidity policy explicitly excludes them; label sources excluded from net worth so the scope difference is understandable. Existing Overview totals retain their current semantics.

## 6. Persistence contract

Add typed UUID v7 identities in `internal/domain`, following current ID conventions. New tables use `ON DELETE RESTRICT`; financial evidence is not cascade-deleted.

All money/rates are canonical decimal `TEXT`; all dates are validated `YYYY-MM-DD`; timestamps are existing UTC millisecond strings. UUID shape, cross-household ownership, enums, and shape constraints are checked in Go and appropriate SQL constraints.

### 6.1 `product_contracts`

| Field | Type / rule |
| --- | --- |
| `id`, `household_id` | PK and household FK |
| `account_id`, `holding_id`, `instrument_id` | FKs; holding and instrument each UNIQUE; all same household/account |
| `kind` | `term_deposit` or `locked_product` |
| `name`, `note` | Existing name/note limits |
| `currency` | Instrument quote currency; immutable |
| `principal` | Positive Money; original principal/subscription cost, immutable |
| `start_on` | Original contract date, not inferred financial acquisition time |
| `maturity_on` | Required for term deposit; nullable for locked product |
| `interest_mode` | `none`, `manual_maturity_amount`, `simple_act_365`, `simple_act_360` |
| `annual_rate` | Nullable decimal ratio, e.g. `0.025`; 0..1, at most 8 fractional digits |
| `maturity_interest` | Nullable nonnegative Money; unpaid interest expected at maturity |
| `interest_paid_through_on` | Required for simple-interest mode; initially `start_on` |
| `renewed_from_id` | Nullable contract FK; new contract never overwrites old terms |
| `state` | `open`, `settled`, `cancelled`; checked against quantity and active operations |
| `opened_operation_id`, `closed_operation_id` | Operation references; closed ID nullable |
| `revision` | Positive integer for compare-and-swap updates |
| `created_at`, `updated_at` | UTC timestamps |

The first version supports simple interest payable at maturity. For products paying interest periodically, use manual unpaid maturity-interest mode and record actual interest receipts as they occur; do not pretend a periodic payment schedule was forecast.

Contract identity and principal are immutable after creation. Date/rate/estimate edits affect future liquidity projections only; they do not rewrite historical financial facts. Reject edits producing `maturity_on <= start_on`, paid-through outside the term, or inconsistent policy dates. Settled/cancelled contracts are read-only except notes/name.

Avoid cyclic SQL FK insertion problems: create the operation identity row first inside the transaction, insert the contract, then operation-product links. No partially written identity survives rollback.

### 6.2 `liquidity_policies`

One optional explicit policy per source. Store `id`, `household_id`, `source_kind`, `account_id`, nullable `holding_id`, `currency`, and:

| Field | Values / meaning |
| --- | --- |
| `access_kind` | `on_request`, `on_date`, `unknown`, `excluded` |
| `unlock_on` | Required for `on_date`; earliest date an action may begin |
| `settlement_days` | Integer 0..365, nullable when unknown |
| `day_basis` | `calendar` or `weekdays` |
| `receipt_on_override` | Optional exact estimated receipt date; cannot precede action eligibility |
| `accessible_amount_cap` | Nullable nonnegative Money for ordinary sources; null means the whole source, zero means none; managed products must use null |
| `normal_exit_fee` | Nullable nonnegative Money in source currency; null means unknown; an explicit 0 means no assumed exit fee |
| `early_kind` | `not_allowed`, `allowed`, `unknown` |
| `early_settlement_days`, `early_day_basis` | Required for allowed early access |
| `early_fee` | Nonnegative Money, required for allowed early access |
| `early_amount_mode` | `current_value` or `fixed_gross` |
| `early_gross_amount` | Required for fixed-gross; for deposits this is principal plus estimated early interest |
| `confirmed_at`, `note` | When assumptions were reviewed and explanation |
| `revision`, `created_at`, `updated_at` | CAS and timestamp fields |

Add unique indexes for `(account_id)` where kind is account_value, `(account_id,currency)` where account_cash, and `(holding_id)` where holding. SQL CHECKs enforce the nullable-reference shapes; application/schema verification enforces ownership and currency equality.

For managed products, create the policy in the opening transaction. Contract and policy date changes go through one contract update operation, not two independent requests. A deposit's normal `unlock_on` equals maturity. A locked product may have a lock end before a later maturity; if maturity is set, ordinary contractual redemption is scheduled at maturity. Its earlier lock end describes when the optional early route is allowed. To represent normal redeem-on-request after lock end, leave maturity unset.

For ordinary components, an optional exact receipt override replaces calculated timing. For managed deposits it must be on/after maturity. Unknown fee/amount/timing must be represented as unknown, not a fabricated zero; allowed routes require the corresponding fields before they contribute.

### 6.3 `liquidity_reservations`

Fields: `id`, `household_id`, typed source reference fields as above, `label`, `amount`, `currency`, `revision`, `created_at`, `updated_at`, nullable `released_at`.

- Amount is positive Money in the source currency; multiple active reservations may reference one source.
- Reservations are intentions, not transactions or valuation deductions.
- Do not prohibit a reservation larger than the current value: retain it and show a shortfall. This also handles later price declines.
- Release rather than delete. Released reservations are exported and no longer deducted.
- A reference to a disappeared/zero/archived source remains visible as unresolved; do not silently drop or move it.
- Product settlement/renewal with active reservations requires an explicit previewed choice to release them. The first version does not transfer them automatically to cash or the successor product.

### 6.4 Product operation evidence

Add these tables:

- `product_operations`: `id` (also the client mutation ID), `household_id`, `kind`, `payload_sha256`, `request_version` (1), normalized `request_json`, `result_json`, `effective_at`, `created_at`, nullable `reverses_operation_id` (UNIQUE when present).
- `product_operation_products`: `(operation_id, product_id, role)`; roles `opened`, `settled`, `income`, `reopened`, `cancelled`; references describe operation participation.
- `product_operation_activities`: `(operation_id, activity_id, sequence, purpose, product_id)`; activity ID UNIQUE; purpose `acquisition`, `existing_position`, `redemption`, `interest`, `reversal`.
- `product_operation_reservations`: `(operation_id, reservation_id, previous_released_at, resulting_released_at, resulting_revision)` for restoring reservation state during eligible undo.

Operation kinds: `open`, `record_existing`, `receive_interest`, `settle`, `renew`, `undo`.

Also support `value_observation` in this operation evidence table for the
dedicated valuation endpoint: it links to the product with role `valued`,
has no financial Activity children, and saves its created quote ID in the
receipt. This provides persistent idempotency for valuation writes without
creating a fake ledger activity. Undo of this observation is not a product
operation in v1; correct it through the existing append/supersession quote
mechanism and preserve the original evidence.

Store immutable evidence and receipt DTOs, not arbitrary in-memory Go serialization. Hash normalized typed commands in Go; do not trust a client-supplied payload hash. The operation group is additional metadata around existing Activities; do not invent a new financial ActivityKind for each product UI action.

## 7. Availability rules and calculations

### 7.1 Time model

- Capture `asOf` once from the injected application clock for each request.
- Use History Origin's IANA timezone. If there is no origin, require it before financial product operations; read-only liquidity may use an explicit UTC fallback and expose that timezone.
- `today` is the captured local date. Windows end at `today`, `today + 7 calendar days`, and `today + 30 calendar days`, inclusive. Never obtain those dates by adding 24-hour durations across DST.
- Custom horizon must be today or later, at most 10 calendar years ahead; invalid dates are errors.
- A zero settlement lag returns the action-eligible date. For N > 0, begin counting on the following date. `weekdays` skips Saturday/Sunday only. Label it **Estimated weekdays; holidays not included**.
- Recompute on page focus, local-day rollover, and query refresh. Do not leave yesterday's Today card cached indefinitely.

### 7.2 Conservative defaults

| Source | Default normal access |
| --- | --- |
| Asset-side cash-on-hand | Today, 0-day lag |
| Bank balance or bank cash component | Today, 0-day lag; clearly marked as assumed until reviewed |
| Brokerage, exchange, digital wallet, pension, insurance, or other cash/account values | Unknown until reviewed; account restrictions can prevent withdrawal |
| Ordinary stocks/funds/bonds/other holdings | Unknown until reviewed; do not hardcode a market's settlement convention |
| Property/vehicle/collectible | Excluded from short-term access by default; can be configured |
| Managed product | Explicit contract policy is required |

For restricted account types, an unconfigured holding is unknown regardless of instrument type. An explicit source policy is authoritative and must warn that the user is confirming both asset liquidation and account withdrawal restrictions. A policy can always override an assumed bank-cash default.

Return `policyOrigin = assumed | explicit | contract`, and `assumptions[]`. Completeness means all relevant inputs are known under the displayed assumptions, not that a bank guarantees settlement.

### 7.3 Candidate routes

Build routes for each source, not additive asset copies:

1. **Normal:** access today or from `unlock_on`, then settlement lag/override.
2. **Early:** only when enabled in the query and explicitly allowed by the policy; begin no earlier than today and any product lock end. A term deposit's early route exists only while today is before maturity.

For a locked product with a mandatory lock, early access cannot bypass that lock. `early_kind=allowed` means access before normal maturity but after the mandatory lock. Products genuinely withdrawable during the nominal term should have an earlier/no lock end and separate normal maturity.

For horizon H, consider only routes whose estimated receipt date is <= H. Select the route with the highest known nonnegative net proceeds; on ties prefer normal, then earlier receipt. Never add normal and early proceeds together. Display the selected route and the alternative/cost explanation. All candidates model action starting from today's state, not sequential actions over the forecast period.

A known alternative does not make the result complete when another eligible alternative has unknown proceeds; include the known subtotal but mark that row and bucket partial.

### 7.4 Amounts

For an ordinary holding or locked product:

```text
normalGross = min(current native valuation, accessibleAmountCap if present)
normalNet = max(normalGross - normalExitFee, 0)
```

For term deposits:

```text
days = civil-calendar days in [interestPaidThroughOn, maturityOn)
interest = principal * annualRate * days / (365 or 360)
normalGross = principal + unpaidMaturityInterest
normalNet = max(normalGross - normalExitFee, 0)
```

Round simple-interest output once to four decimal places, half away from zero, using exact decimal/rational arithmetic. `manual_maturity_amount` uses the supplied unpaid interest directly; `none` is an explicit principal-only estimate. APR input “2.5%” becomes canonical ratio `0.025` in Go; accept a percent string in the form request to avoid frontend floating-point conversion.

After a recorded interest payment, simple mode requires an explicit new paid-through date <= today and <= maturity; manual mode requires the remaining unpaid maturity interest. Advance neither by guessing from the payment amount. The operation saves both the actual income and new forecast metadata atomically. Store before/after forecast metadata in its normalized evidence for undo.

Early routes use either current native value or an explicitly entered gross amount, minus the configured early fee. For deposits, the editor displays principal and estimated early interest separately and saves their gross sum. Display foregone maturity interest separately; do not subtract it a second time from the early amount.

For ordinary sources, the accessible-amount cap also limits early gross
before fees. The rest remains visible as restricted value and is never
converted into a reservation. For example, cash 10,000 with accessible cap
5,000 and reservation 1,000 yields available 5,000 and unreserved 4,000.
Managed contracts remain all-or-nothing in v1, so a cap is not accepted for
them. Null fee produces an unknown net amount, not a zero fee.

If a configured fee exceeds known gross proceeds, net is 0 and the row carries an assumption/conflict warning; this is a known zero, not a missing value. Future prices and FX remain unchanged assumptions, not predictions.

### 7.5 Due but not received

A managed deposit/product with a scheduled normal receipt date <= today and no actual settlement is **Due — receipt unconfirmed**. Do not promote it to cash or include its normal route in spendable totals. Keep its current value in net worth.

The user can confirm receipt or enter a revised expected receipt date. If the revised date is future, that route contributes to future buckets. A locked product without scheduled maturity may simply become redeemable after unlock; that is not an overdue automatic receipt.

Derived display states: `locked`, `redeemable`, `due_unconfirmed`, `settled`, `cancelled`. The first three are calculated from dates and policy, not separately persisted lifecycle flags.

### 7.6 Reservations and FX

For each selected native route i:

```text
requestedReserve_i = sum(active reservations in source currency)
appliedReserve_i = min(requestedReserve_i, netProceeds_i)
unreserved_i = netProceeds_i - appliedReserve_i
reserveShortfall_i = max(requestedReserve_i - netProceeds_i, 0)
```

Apply reservations only when their source is available in that horizon. A reserve against a locked deposit does not reduce today's unrelated bank cash. Show total requested reserves and inaccessible/unresolved reserves separately so this behavior is visible.

Convert net proceeds and reserve amounts using the same eligible current FX path and source preference as current valuation. Never use a future quote, provider fetch, or implicit Manual/Provider fallback. If needed, extract a small shared conversion helper from `valuation.go`; do not copy its policy into another calculator.

Sum exact converted values, then format at the existing MoneyView precision. For each displayed bucket, round gross and applied reserve and set displayed unreserved to their displayed difference to preserve the identity; retain exact intermediate values internally. Native-currency groups remain available when base FX is missing.

### 7.7 Completeness and invariants

Return separate `status = complete | partial | unavailable` and `knownSubtotal` fields. `0` is valid only when zero is actually known; nullable full totals represent incomplete knowledge.

- Unknown access/amount/cost/timing for an otherwise in-scope asset makes the affected horizon partial. A known unlock date after H does not require its amount to prove it is unavailable before H.
- Explicitly excluded assets do not make a bucket incomplete; disclose them separately.
- Missing base FX makes the base result partial; preserve the known native amount.
- If no eligible amount can be valued and unknown candidates exist, full total and known subtotal are null. If eligibility proves no assets available and there are no unknown candidates, total is exactly zero.
- Reservation source missing/archived or no longer positive produces a visible unresolved-reservation warning. It cannot silently disappear or be deducted from an unrelated asset.
- Under a fixed query snapshot and assumptions, cumulative known available and unreserved amounts cannot decrease as the horizon moves out. Test this property.
- Outstanding debt is not automatically subtracted. Say **“Based on current assets; future spending, repayments, and FX conversion costs are not included unless reserved or configured.”** Do not label the result “safe to spend”.

## 8. Atomic financial operations

### 8.1 Shared execution algorithm

Implement a dedicated application use case and `ProductRepository` transaction boundary.

```text
Preview(command):
  parse + normalize command; read a consistent current snapshot
  validate contract, account, currency, dates, policies, reservations
  build all domain previews sequentially against an in-memory working state
  return ordered effects, metadata changes, warnings, normalized command,
         reviewedStateHash, and evaluated local date

Record(command, mutationId, reviewedStateHash):
  acquire beginLedgerWrite
  normalize + server-hash command
  replay stored operation FIRST when ID/hash match; conflict when hash differs
  reload relevant state; compare reviewedStateHash; stale => new preview required
  rebuild and validate every effect using current state
  commit identities + quotes + activities + projections + metadata + operation
       + reservation releases + history dirty markers in ONE SQLite transaction
  invalidate analysis once after successful commit; return saved receipt
```

Hash relevant persisted facts, revisions, source quote IDs, cash balances, quantity/cost evidence, and evaluated local date, plus the normalized command. Do not include a changing read timestamp. Mutation ID stays constant across retries; generate a new ID after the user changes the command. Replays return the saved receipt even if current state has subsequently changed.

`Preview` cannot write quotes, generate persisted records, or fetch providers. Generated preview IDs are not authoritative. Allocate persistent IDs during commit, and map operation-local draft references deterministically to those IDs.

Do not call public `CreateInstrument`, `RecordChange`, or `AppendManualQuote` repeatedly from inside a product transaction: they own independent transactions and may reacquire locks. Factor transaction-local SQL helpers where necessary. Every child preview consumes the working state resulting from the previous child, especially renewal cash and interest.

### 8.2 Operation recipes

| Operation | Atomic recipe |
| --- | --- |
| Open | Create manual private instrument; opening quote; zero holding; build `buy` quantity 1, gross principal, optional fee; persist contract and required policy |
| Record existing | Create instrument, quote and holding through existing already-existed quantity/cost evidence semantics; pass the user-confirmed total cost basis as unit cost for quantity 1; no cash effect; preserve reconciliation classification |
| Receive interest | `cash_in` with reason `interest`; link to product; update paid-through or remaining maturity-interest estimate |
| Settle deposit | `sell` quantity 1, gross returned principal, optional fee; then `cash_in/interest` if interest > 0; mark settled; release confirmed reservations |
| Settle locked product | `sell` quantity 1, actual gross proceeds, optional fee; mark settled; no extra fabricated interest; release confirmed reservations |
| Renew | Run appropriate old settlement recipe, then new opening recipe in the same working state/transaction; create new IDs and `renewed_from_id` |
| Undo | Reverse all financial children in reverse sequence; restore lifecycle/forecast/reservation state from evidence, with dependency checks |

For deposit settlement, principal may be lower than original principal to record a real loss, but must be positive and <= original principal. Excess receipt belongs in interest. For a locked product, positive proceeds may be below or above original cost. Actual amounts are user-confirmed and do not have to equal forecasts.

Validate actual settlement fee <= its sale gross and actual interest >= 0;
interest-only receipt requires amount > 0. These are v1 operation limits,
not assumptions inferred about a financial institution. A gross-zero loss
or a fee larger than proceeds is explicitly unsupported by this form.

Same-account, same-currency settlement is required. All financial operations use a user-confirmed timestamp between History Origin and now, normalized through existing time validation. For managed products, enforce chronological append: a new financial operation cannot precede that product's latest active financial operation. Record-existing acquisition uses now; its original contract start is descriptive only.

Persist all creation/state/preference observations and the opening quote at
the actual opening effective time, including when that time precedes the
recording time. A newly created product must be reconstructible from its
first financial event. Reject an opening effective date before its contract
start date. Existing-position recording may use a historical contract start,
but never creates financial observations before its recording time.

Allocate child Activity IDs in execution order and use the existing stable
effectiveAt/createdAt/ID ordering so replay at one shared timestamp matches
the preview's order. Test same-timestamp interest, redemption, and renewal;
do not rely on unspecified SQL row order.

Do not add restrictions on unrelated historical account activity beyond existing replay validation. Reject a product operation if replay would produce insufficient cash/quantity. Do not bypass existing future-date, history-origin, mutation, backup, or restore guards.

### 8.3 Grouped undo and dependent activity

Reuse existing reversal construction and replay checks, including the original effective-time semantics. Do not construct a cash-only refund.

- Only the latest non-reversed operation for every participating product is eligible.
- A later renewal, income receipt, contract/policy edit, valuation observation, or reservation edit affecting the operation's before/after state blocks undo until resolved. Compare revisions/evidence explicitly.
- After undoing opening, quantity is 0 and contract is cancelled. Keep immutable activities and quotes; never delete the financial history.
- After undoing settlement, quantity returns to 1, contract reopens, and eligible released reservations/forecast metadata are restored.
- Undo renewal closes/cancels the successor and reopens the predecessor in one transaction.
- Subsequent spending of the received cash can make inverse effects invalid. Surface the existing conflict; do not permit negative cash to force an undo.
- Generic `UndoChange`, `PreviewFixChange`, and `FixChange` must reject member activity IDs and direct the UI to grouped undo. Enforce this in Go, not only via hidden buttons.

### 8.4 Prevent bypasses

Add a common managed-position guard to general commands that could break quantity 0/1 or private ownership: create/update holding, generic trade, position transfer, quantity adjustment, archive, instrument currency/source/provider changes, and inappropriate account archive.

Reject archiving an account with open managed contracts. For ordinary
sources, account currency edits that would invalidate explicit policies or
reservations must conflict and explain which references need updating; do
not silently reinterpret saved amounts in another currency.

Generic History entry points reject managed holdings and return a stable conflict. Product use cases call pure domain builders through a typed internal path, not a user-supplied “bypass” flag.

Locked-product valuation updates remain supported through a dedicated product action that appends a manual quote using shared quote logic. Term-deposit quotes cannot be freely edited to include projected interest. Block generic manual-quote writes for managed instruments and route the UI to product detail. No provider binding can be added to a managed instrument.

Ordinary un-managed `bank_investment_product` holdings remain ordinary holdings. Do not automatically adopt or lock them during migration.

## 9. Analysis, history, and data integrity

### 9.1 Product interest attribution

Add an optional typed `ProductActivityContext` to the internal hydrated Activity read model: operation ID, product ID, purpose, holding ID, instrument ID, product kind. Populate it by joining/batch-loading the operation-activity links in every Activity reader, including historical batch/replay/export inputs and reversals. Avoid N+1 reads.

In `analysis_classifier.go`, check product-linked `cash_in/interest` before the ordinary cash-interest branch:

- Treat it as Dividend/Interest return associated with the product holding/instrument, analogous to linked dividend income.
- Household Asset Changes includes it as interest income, with zero external contribution.
- Product/instrument Return Analysis sees the interest even when the cash component is excluded.
- Do not also attribute that same interest to an unrelated cash return component.
- Reversal context must retain association and invert the signed amount.

Keep deposit redemption gross equal to returned principal, so interest is not also counted as a realized capital gain. Locked-product redemption naturally realizes actual proceeds minus cost/fee through the existing gain calculation.

The existing cash-dividend UI and dividend-specific reports must not relabel product interest as a stock dividend. Use the linked context for product labels and unified Dividend/Interest analysis.

### 9.2 Snapshot and quote behavior

- Managed positions enter current valuation, account composition, portfolio holdings, historical replay, and daily snapshots through existing Instrument/Holding/quote paths.
- Reuse standard history dirty-range marking for financial changes and manual product quote observations.
- Liquidity-policy, reservation, and forecast-only term edits invalidate liquidity reads but do not dirty historical valuations.
- A maturity-date rollover invalidates/recomputes liquidity only; it never appends financial facts.
- Product quote history carries real manual observations. Do not manufacture carry-forward observations.
- Scope changes, missing FX, corrections, and principal-only transfer neutrality must satisfy the existing Asset Changes reconciliation invariant.

### 9.3 Snapshot consistency

`ReadLiquiditySnapshot` must read portfolio facts, policies, contracts, reservations, and operation state in one database read transaction. Feed that snapshot into valuation without reopening a second repository read. Add an optional application-side `asOf` parameter or pure evaluation helper if necessary.

No current availability response should mix a pre-redemption holding with post-redemption cash. Add an integration test that forces this race.

## 10. Application and Wails contract

Create `internal/wailsapi/liquidity` exposing a thin Service over application methods. Add `LiquidityRepository` and `ProductRepository` ports; the SQLite repository implements them. Wire through the current application composition; application code must not import SQLite.

### 10.1 Read methods

```text
Overview({customHorizonOn?, includeEarlyWithdrawal}) -> LiquidityOverviewDTO
Product({productId}) -> ProductDetailDTO
ListProducts({accountId?, includeClosed}) -> ProductDTO[]
ListOperations({productId, cursor?, limit}) -> ProductOperationPageDTO
```

`Overview` always returns Today, +7, +30 buckets and optionally custom. Include all source rows so filters are local presentation; do not expose a partially paginated list as a complete financial total.

Required response fields:

```text
LiquidityOverviewDTO:
  asOf, localDate, timezone, baseCurrency, assumptions[]
  buckets[]: horizonOn, status,
    fullAvailable?, knownAvailableSubtotal?,
    appliedReserveSubtotal?, fullUnreserved?, knownUnreservedSubtotal?,
    unknownSourceCount, excludedSourceCount, estimatedSourceCount,
    nativeCurrencyGroups[], warnings[]
  sources[]: sourceRef, sourceKey, accountId, productId?, displayName,
    nativeCurrency, currentNativeValue?, valueAsOf?, priceEvidence?, fxEvidence?,
    policyOrigin, policy?, contractState?, reservationRequested,
    normalRoute?, earlyRoute?, bucketResults[], reasons[]
  unresolvedReservations[]

RouteDTO:
  kind, eligibleOn?, receiptOn?, grossNative?, feeNative?, netNative?,
  status, amountBasis, actionRequired, assumptions[], missingReasons[]

BucketResultDTO:
  horizonOn, selectedRoute?, netNative?, netBase?, appliedReserveNative?,
  unreservedNative?, reserveShortfallNative?, status, reasons[]
```

Money fields are MoneyView-like `{amount: string, currency: string}` or null, never JavaScript numbers. Nonfinancial counts/lags/revisions remain numbers. Return empty arrays rather than null arrays. Nullable totals must be explicit in generated bindings.

### 10.2 Metadata methods

```text
SavePolicy({sourceRef, expectedRevision, policy}) -> PolicyDTO
ResetPolicy({sourceRef, expectedRevision}) -> ResolvedPolicyDTO
SaveReservation({id?, sourceRef, expectedRevision, label, amount}) -> ReservationDTO
ReleaseReservation({id, expectedRevision}) -> ReservationDTO
UpdateProductTerms({productId, expectedRevision, terms, policy}) -> ProductDetailDTO
AppendProductValuation({productId, amount, observedAt, mutationId}) -> ProductDetailDTO
```

Revision 0 means create-if-absent; updates require the current positive revision. Reset is supported only for ordinary sources; managed-product policy is mandatory. Metadata writes use write coordination and revision CAS. Valuation append uses the `value_observation` operation evidence described in section 6.4; quote + operation receipt commit together. A double click must not append two observations.

### 10.3 Financial methods

```text
PreviewProductOperation(ProductCommandRequest) -> ProductOperationPreviewDTO
RecordProductOperation({command, mutationId, reviewedStateHash}) -> ProductOperationReceiptDTO
```

Use a validated tagged union in the Wails request, following the existing history command style. Exactly one payload must match `kind`:

| kind | Required payload |
| --- | --- |
| open | accountId, currency, principal, openingFee, product terms/policy, effectiveAt |
| record_existing | accountId, currency, principal, totalCostBasis, currentValue, product terms/policy, cashExcludesProduct acknowledgement |
| receive_interest | productId, amount, effectiveAt, new paid-through or remaining unpaid interest |
| settle | productId, returnedPrincipal + interest OR grossProceeds, fee, effectiveAt, releaseReservationIds |
| renew | settlement payload plus new principal, openingFee, new terms/policy |
| undo | operationId |

Preview includes before/after cash and product values, ordered activity descriptions, net-worth effect, required reservation releases, missing fields, assumptions, normalized request, and reviewed state hash. No incomplete preview can be recorded. For zero fees/interest, omit zero-valued financial effects instead of violating existing effect validation.

Here, incomplete means missing **required native accounting inputs or
confirmation**, not an unavailable optional base-currency display. Missing
FX may make the advisory base net-worth effect unavailable, but must not
block a fully specified same-currency receipt. Show that limitation without
substituting zero. Required source cash/quantity/cost evidence must still be
valid before recording.

`ProductDTO` includes IDs, kind/name, currency, principal, currentValue with
evidence/completeness, current cost basis, dates, interest assumptions,
policy/revision, stored and derived lifecycle states, predecessor/successor
IDs, and next action. `ProductDetailDTO` adds active reservations and permitted
actions with explicit disabled reasons. `ProductOperationReceiptDTO` includes
operation ID, participating product IDs, ordered Activity IDs, resulting
cash/product state, and a replayed flag. `ListOperations` sorts newest first
by `(createdAt,id)`, uses an opaque cursor, defaults to 25 and caps at 100;
invalid cursors are validation errors. Include reverse links and grouped
descriptions so the UI does not infer operation membership from timestamps.

The record-existing request must always supply totalCostBasis, including an
explicit zero when known. Missing basis is not assumed to equal current
value. Reject unsupported unknown-basis recording in this v1 form rather
than fabricate gain history.

Add stable error codes/reasons for stale preview, revision conflict, managed-position conflict, unsupported partial operation, unresolved reservation release, receipt pending, and unsafe undo. Reuse existing validation/not-found/insufficient-balance wrappers where appropriate. Do not expose SQL text or raw internal errors.

## 11. UI implementation

### 11.1 Available Funds page

Navigation label: **Available funds / 可用资金**. Place after Overview or Accounts, not under Market Data.

Layout:

```text
Available funds                         [Configure availability]
As of … · Household timezone …          [Consider early withdrawal]

[ Today ]       [ Within 7 days ]       [ Within 30 days ]
Each: Available · Reserved · Unreserved · completeness indicator

Custom date [date picker]               Currency display [Base / Native]

Upcoming availability / Due, receipt unconfirmed

Assets [All | Available by date | Locked | Needs information | Excluded]
Account | Asset | Current value | Expected access | Cost | Reserved | Unreserved

Unresolved reservations / Assumptions and omitted costs
```

Cards are cumulative; explicitly say they must not be added together. Selecting a card changes the row horizon. Distinguish **cash already accessible** from **estimated proceeds requiring a sale/redemption** in each card's breakdown.

Use semantic tables, explicit incomplete badges and reason text, keyboard-accessible dialogs, focus restoration, and all existing locales (`en`, `zh-CN`, `zh-TW`). Do not rely on color or tooltips alone. Native currency mode groups currencies and never adds unlike currencies.

Loading, load failure with retry, no assets, all locked, all excluded, partial FX, no access policy, and due-unconfirmed are separate states. No empty state may encourage creating a duplicate account.

### 11.2 Account and product details

- Account detail adds **Deposits and products**, with current value, principal, maturity/unlock date, and status.
- Product detail owns terms, normal/early availability, reservations, valuation history, and grouped financial operations.
- Product forms use principal/value language; hide internal quantity=1 and private-instrument mechanics.
- Existing Portfolio/Market Data rows remain correctly valued but route managed-product edits to product detail. Show the contract subtype label rather than offering generic symbol/provider editing.
- Existing holdings tables can label the quantity cell “1 contract” for managed positions; normal securities retain current quantity behavior.
- Do not list a managed product twice as both a new asset and its backing holding.

### 11.3 Overview and reminders

Add a compact card with today's unreserved amount, 30-day estimate, and completeness state. It uses the same `Overview` query/cache as the full page.

Reminders are derived in-app: unlock/maturity within 7 days, and receipt overdue/unconfirmed. Clicking opens the product. No background cash posting, automatic renewal, provider fetch, or OS permission request.

### 11.4 Query ownership

Add `frontend/src/queries/liquidity.ts` and `queryKeys.liquidity.all`, with children for overview parameters, products, product detail, and operations. Use serializable keys with canonical parameters.

- Financial product operation: invalidate liquidity, history, holdings, instruments, account/portfolio/overview valuation, analysis, snapshot state, and quote reads affected by creation.
- Product quote: use existing quote invalidation plus liquidity.
- Ordinary activity, quote/FX change, source preference, account lifecycle, restore, or relevant base-currency change: invalidate liquidity too.
- Policy/reservation/forecast terms: liquidity queries and Overview liquidity card only.
- Never optimistically add financial totals or mark a receipt paid before the returned commit succeeds.
- Clear all caches after restore using existing restore lifecycle behavior.

## 12. Schema migration, export, and recovery

### Migration

At this baseline use schema **12** and `migrate_v11.go` implementing `migrateV11ToV12`. If another feature has consumed version 12 when implementation begins, allocate the next version and update every reference/test in this plan's implementation notes.

1. Verify the exact supported v11 shape before mutation.
2. In one migration transaction, create new tables/indexes and bump user_version only after successful checks.
3. Preserve every existing account, quote, activity, and snapshot byte-for-byte where no transformation is required. New metadata tables start empty.
4. Update fresh schema, table-shape checks, FK/integrity checks, export allowlists, and readonly verification together.
5. Test rollback on injected DDL/validation failure; never auto-repair malformed user data by deleting it.

**Important existing-code trap:** `database.go` currently sets `found = CurrentSchemaVersion` immediately after `migrateV10ToV11`. When introducing v12, change that assignment to `found = 11`, then explicitly execute 11 → 12. Otherwise a v10 database can be mislabeled as fully migrated while missing the new tables. Test complete supported upgrade chains, not just 11 → 12.

### JSON export

Increment `ExportFormatVersion` from 1 to 2. Add a documented `liquidity` facts group containing contracts, policies, reservations, product operations and their links, including released/cancelled/reversed records and operation evidence. Export current derived liquidity only if clearly marked derived with asOf/assumptions; it is optional, while exporting all persisted facts is mandatory.

Update `docs/design/json-export-format.md`, explicit SQL projections, stable ordering, and export fixtures. Secrets/provider credentials remain excluded. Quote and Activity facts stay in their existing groups; do not duplicate them inside each product as separate financial facts.

### Backups and restore

Backups already contain the database; no parallel product file or settings JSON store is needed. Test a schema-12 backup containing an open product, settled product, reservation, and renewal/undo evidence, then restore and compare facts and available totals.

Preserve the existing rule that restore verification is read-only and accepts only the supported backup database schema. Do not broaden old-schema backup restore as a side effect of this feature. Document that an existing v11 live database upgrades on open; an old archive may require the project's existing supported upgrade workflow before becoming a current backup.

New metadata/financial writes must respect the same backup/restore exclusive gate and shutdown behavior as existing commands.

## 13. File-level work map

Names marked **new** are intended additions; other entries are existing integration points.

| Layer | Files / changes |
| --- | --- |
| Domain | **new** `internal/domain/liquidity.go`, `product_contract.go`, `product_operation.go`; typed rules, IDs, validation, pure route selection and batch working-state helpers |
| Application | **new** `internal/application/liquidity.go`, `product_operations.go`, `product_guards.go`; extend `repository.go`/`usecases.go`/service wiring with narrow ports |
| Valuation | Extract/reuse exact FX conversion from `valuation.go`; no separate financial calculation in React |
| Financial commands | `change_service.go`, `history_changes.go`, `portfolio.go`: transaction-local reuse and managed-position guards |
| Analysis/read models | `analysis_classifier.go`, activity hydration and historical loaders: product income/reversal association |
| SQLite | **new** `liquidity_repository.go`, `product_repository.go`, `migrate_v11.go`; extend `schema.sql`, `database.go`, `schema_verify.go`, `schema_integrity.go` |
| Persistence helpers | Refactor `change_repository.go`, `portfolio_repository.go`, quote/observation helpers only as needed for shared transactions |
| IPC | **new** `internal/wailsapi/liquidity/liquidity.go`, request/DTO conversion tests; register in `cmd/nestworth/main.go`; add optional product context to relevant existing wire DTOs |
| Export | `internal/domain/export.go`, `internal/application/json_export.go`, `internal/infrastructure/sqlite/export_repository.go` |
| Frontend | **new** `frontend/src/features/liquidity/` with page, policy/reservation sheets, product forms/detail/operation preview |
| Navigation | `frontend/src/app/navigation.ts`, `NavigationContext.tsx`, `App.tsx`, account detail and Overview integration |
| Reads | `frontend/src/queries/liquidity.ts`, `keys.ts`, `invalidation.ts`, related mutation hooks |
| Presentation | Product-aware account/portfolio/market-data actions, localization files, existing money/date/empty-state components |
| Documentation | This plan's implementation checklist, domain/IPC docs, JSON export docs, release scope only when a release is chosen |

Check actual navigation-context filename before editing; use the existing file if its extension differs. Do not create a parallel router or universal repository framework.

## 14. Deterministic acceptance fixtures

Use injected clocks and temporary databases. These examples are product-specific tests in addition to the full existing suite.

### Fixture A: cumulative availability and reservations

Clock: `2026-09-20T04:00:00Z`, timezone `Asia/Singapore`, base USD. Explicit policies unless stated otherwise.

| Source | Current value | Normal proceeds/date | Early proceeds/date | Reservation |
| --- | --- | --- | --- | --- |
| Bank cash | 10,000 USD | 10,000, today | None | 2,000 |
| Deposit D1 | 20,000 USD principal | 20,200 on Sep 25, manual unpaid maturity interest 200 | 19,950 today: gross 20,000 less fee 50 | 5,000 |
| Locked product L1 | 5,000 USD | 4,990 on Oct 5: value 5,000 less fee 10 | Not allowed | 0 |
| Stock H1 | 3,000 USD | 3,000 on Sep 22, explicit two-calendar-day lag | None | 0 |
| Property | 100,000 USD | Explicitly excluded | None | 0 |

Expected without early access:

| Horizon | Available | Applied reserve | Unreserved |
| --- | --- | --- | --- |
| Today | 10,000 | 2,000 | 8,000 |
| Sep 27 (+7) | 33,200 | 7,000 | 26,200 |
| Oct 20 (+30) | 38,190 | 7,000 | 31,190 |

Expected with early access: Today = 29,950 / 7,000 / 22,950. +7 and +30 remain as above because normal D1 proceeds are higher by those horizons. Never count D1 twice.

Variant: add 100 EUR with known today access and no EUR/USD FX. USD full totals become null/partial; known USD subtotals stay as above; native EUR 100 remains visible. Configuring the missing FX must complete the base totals without changing ledger facts.

### Fixture B: opening and deposit receipt

- Start cash 150,000 USD.
- Open deposit principal 100,000, no fee: cash 50,000, holding value 100,000, total 150,000.
- Confirm returned principal 100,000, interest 1,000, fee 10: cash 150,990, holding quantity 0, total 150,990.
- Gain/attribution: principal transfer is neutral; interest 1,000; fee 10; net wealth change 990. Product-scoped return includes interest; interest is not another capital gain or external contribution.
- Same mutation retry returns the original receipt and leaves counts/amounts unchanged. Same ID with a changed amount conflicts.
- Undo settlement before any dependent writes: cash 50,000, quantity 1, total 150,000; signed attribution cancels correctly.

### Fixture C: renewal

Start from B just before receipt. Confirm old principal 100,000 + interest 1,000, fee 0, then renew principal 100,500, fee 0.

Expected: old quantity 0; new quantity 1 valued at 100,500; cash 50,500; total 151,000; two distinct contracts/instruments/holdings; old interest counted once. Inject a failure after old settlement SQL and before new purchase: all financial and metadata state must remain pre-operation.

### Fixture D: calendar and interest

- Principal 36,500; APR 10%; ACT/365; interval Jan 1 → Jan 11 (10 civil days): interest exactly 100.
- Principal 36,000; APR 10%; ACT/360; same dates: interest exactly 100.
- Receive 50 and explicitly move paid-through to Jan 6: remaining ACT/365 interest on the first deposit is 50, not 100.
- Friday `2026-09-18` + 2 weekdays = Tuesday `2026-09-22`; +0 = Friday; weekend holidays are not inferred.
- DST transition in `America/New_York`: +7 calendar days remains the seventh local date, independent of 167/169 elapsed hours.

### Fixture E: locked-product gain

Open with 10,000. Later record value 9,800. Availability uses 9,800, not principal. Redeem actual gross 9,700 with fee 5: net cash receipt 9,695; native realized result = -305 including fee. No fabricated interest and no duplicate value after closure.

## 15. Required test matrix

| ID | Level | Required assertion |
| --- | --- | --- |
| L01 | Domain | Fixture A exact totals, early alternative deduplication, cumulative monotonicity |
| L02 | Domain | Today / +7 / +30 inclusivity; before/at unlock; custom-range validation |
| L03 | Domain | Weekday convention, month/year boundary, leap day, DST |
| L04 | Domain | Fixture D interest; manual overrides; rate/scale/overflow rejection |
| L05 | Domain | Known zero vs missing amount; fee greater than gross; missing eligible alternative |
| L06 | Domain | Multiple reservations, over-reservation, source unavailable before horizon, ordinary-source accessible cap vs reservation |
| L07 | Application | No account-plus-components double count; multi-currency cash source identity |
| L08 | Application | Unknown/assumed/excluded policy; restricted-account cash not silently available |
| L09 | Application | Missing price/FX and stale evidence; native totals preserved; no provider calls |
| L10 | Application | Due-unconfirmed remains product value, not spendable cash; revised date works |
| L11 | Application | Opening/existing-position distinction; no fabricated acquisition or contribution |
| L12 | Application/SQL | Fixture B recording, cost, interest association, fees, replay, grouped undo |
| L13 | Application/SQL | Fixture C atomic renewal, retry, stale preview, insufficient cash |
| L14 | Application/SQL | Fixture E value loss and realized result; quotation update invalidates reads |
| L15 | Application | Generic managed-position edits/trades/transfers/individual undo rejected |
| L16 | Application | Chronological product operations, pre-origin/future times, cross-household/currency rejection |
| L17 | SQL | Fault injection at each multi-write boundary leaves no partial records |
| L18 | SQL | Consistent read snapshot during settlement; CAS conflicts and concurrent double clicks |
| L19 | SQL | Fresh v12; 11→12; supported 9→10→11→12 chain; failed migration rollback; malformed/future rejection |
| L20 | SQL | Private holding 0/1, sole ownership, policy source shape, dangling/FK/currency integrity checks |
| L21 | History | Replay across open/receipt/renew/undo, closed-day dirty marking, zero duplicate product exposure |
| L22 | Analysis | Product-only vs account vs household scope; includeCash on/off; reversed interest and fees reconcile |
| L23 | Application | Reservations explicitly released on settlement; undo restores only when dependency-safe |
| L24 | Portability | All new facts in version-2 JSON; backup/restore equality; no credentials exported |
| L25 | Application | Backup/restore exclusive gate blocks writes; no nested-lock deadlock |
| L26 | Wails | Request union validation, nullable DTOs, errors, service startup registration and generated bindings |
| L27 | Frontend | Loading/empty/error/partial/all-locked states; no null-as-zero; cumulative card labels |
| L28 | Frontend | Open→preview→confirm; reservation release; actual receipt vs forecast; renewal; double-submit prevention |
| L29 | Frontend | Cache invalidation after cash/quote/FX/policy/restore changes; day rollover |
| L30 | Frontend | Keyboard-only full flow, focus restore, accessible tables/forms, all three locales |
| L31 | Native | Current build + isolated seeded DB: flows below, app restart persistence |

Do not assert only that a DTO exists. Assert independent amounts, ledger effects, relevant row counts, and missing/completeness semantics. Tests must not calculate their expected values by calling the production evaluator.

## 16. Implementation sequence for the agent

Work sequentially. Keep each phase buildable; use local tests before proceeding. Do not declare the feature complete with unchecked phases. Do not launch parallel agents unless the user explicitly asks.

### Phase 0 — Baseline and implementation log

- [x] Record HEAD, status, schema/export versions, and the exact existing command/valuation paths.
- [x] Preserve unrelated changes, including the brainstorming document.
- [x] Create a short implementation progress section at the end of this document, recording decisions that differ from this plan and why.
- [x] Run the current baseline checks once; record existing failures distinctly.

### Phase 1 — Pure domain and golden cases

- [x] Implement source references, policies, reservations, product contract validation, calendar arithmetic, and route evaluation.
- [x] Implement exact interest/cost calculations and nullable completeness rules.
- [x] Pass L01–L06 with deterministic fixtures before adding UI.

### Phase 2 — Persistence and migration

- [x] Add ports, tables, indexes, schema verification, migration, and consistent reads.
- [x] Fix the migration-chain version trap described above.
- [x] Add export v2 facts and backup verification coverage now, not as cleanup after UI.
- [x] Pass schema/migration/round-trip tests; database creation and existing test fixtures still work.

### Phase 3 — Atomic product lifecycle

- [x] Implement Preview/Record and transaction-local acquisition, existing-position, income, settlement, and renewal recipes.
- [x] Implement idempotency, stale-preview rejection, reservation release, and guarded grouped undo.
- [x] Guard all general mutation paths and managed-instrument quote/source paths.
- [x] Pass B/C/E accounting and transaction failure tests before exposing user actions.

### Phase 4 — Valuation, analysis, and history

- [x] Hydrate product context consistently across all Activity readers and reversals.
- [x] Attribute product interest to its holding/instrument exactly once.
- [x] Verify historical snapshots, current values, gain, scope filtering, and FX behavior.
- [x] Build liquidity snapshots through current valuation/FX helpers and pass A/D/missing-input cases.

### Phase 5 — Wails and query layer

- [x] Expose only the APIs specified above, with typed requests and output DTOs.
- [x] Register the service correctly; regenerate bindings through maintained commands.
- [x] Add query keys, hooks, invalidation, day rollover, and IPC tests.
- [x] Do not manually patch generated TypeScript bindings or revive retired APIs.

### Phase 6 — UI flows

- [x] Available Funds page, Overview card, source-rule and reservation editors.
- [x] Account products section, product details, opening/existing-position forms.
- [x] Valuation, interest receipt, full settlement, renewal, and grouped undo flows.
- [x] Due reminders, incomplete states, localization, and keyboard coverage.
- [x] Remove conflicting generic product actions while preserving ordinary investment-product behavior.

### Phase 7 — Acceptance and documentation

- [x] Run the complete automated gate once all changes are integrated.
- [ ] Run isolated native acceptance and reopen the app to check persistence.
- [x] Update affected domain/IPC/export docs to implemented behavior; keep exclusions explicit.
- [x] Report tests actually run, native evidence, remaining limitations, and working-tree/commit state accurately.
- [x] No release, push, live-bank connection, or changes to real household data are implied by this plan.

## 17. Validation commands and native walkthrough

Use the repository's maintained package manager and task definitions. At this baseline:

```sh
GOCACHE=/tmp/nestworth-review-gocache go test ./internal/domain ./internal/application ./internal/infrastructure/sqlite ./internal/wailsapi/...

SDKROOT="$(xcrun --sdk macosx --show-sdk-path)" \
GOCACHE=/tmp/nestworth-review-gocache \
wails3 task check

git diff --check
```

Run frontend commands from `frontend/` using pnpm. The full gate generates/checks bindings, builds the frontend, runs Go tests/vet/build and frontend lint/typecheck/tests. Do not treat a successful build as native UI acceptance.

Native walkthrough uses a freshly created temporary directory, `NESTWORTH_DATABASE_PATH`, and `NESTWORTH_SETTINGS_PATH` pointing into it. Build the current code. Never use an ambiguous installed instance with real household data.

1. Create/seed a USD bank Holdings Account with 150,000 cash.
2. Open the 100,000 deposit in Fixture B; verify cash + product = 150,000 on Account, Overview, and Portfolio surfaces.
3. Create reservations and compare Today/7/30, with early access off/on.
4. Open a locked product; change its current valuation; inspect liquidity and portfolio updates.
5. Confirm deposit receipt with interest/fee; verify History, cash, zero remaining holding, and interest attribution.
6. In a separate fixture, renew and undo renewal; inspect both contracts and amounts.
7. Check an unknown FX/policy state and due-unconfirmed state; no zero substitution or phantom cash.
8. Restart; verify persistence. Create and restore a backup into another isolated location; compare facts and totals.
9. Exercise the primary form flow with keyboard only and inspect all three languages for untranslated keys.

Completion requires automated PASS plus this native evidence, or an explicit, concrete report of any unavailable native gate. Do not check native items merely because React component tests passed.

## 18. Handoff prompt

The user can give the implementing agent this prompt:

> Implement `docs/design/available-funds-implementation-plan.md` against the current repository. Follow its binding decisions and phases in order. Read current code before changing it; the plan's baseline is `980151c`. Deliver the entire included scope, including atomic product lifecycle, analysis/history integration, migrations/export/backup, UI, and acceptance tests. Preserve unrelated work. Do not silently replace missing inputs with zero, duplicate assets, book future cash, or split a renewal into separate transactions. Keep progress checkboxes and evidence accurate. When a proposed detail conflicts with current code, record the specific conflict and resolve it consistently with the financial invariants rather than inventing a second accounting system. Do not push or publish. Report actual automated/native results and any remaining limitations.

## 19. Implementation progress

Started from `origin/main` at `5486c22` (plan document). Verified code baseline referenced by the plan: `980151c`. Branch: `cursor/available-funds-8542`. Schema 11 → 12; JSON export 1 → 2.

### Conflicts with current code (resolved using financial invariants)

1. **SQLite must not import `application`.** Snapshot/bundle/evidence types live in `internal/domain` (`LiquiditySnapshot`, `ProductBundle`, `ProductOperationEvidence`) so persistence can return them without an import cycle with application tests.
2. **Assumed bank-cash / cash-on-hand missing fee.** Default assumed `on_request` policy now attaches a known-zero exit fee (`origin=assumed`) when native currency is known. Explicit policies still treat a nil fee as unknown (L05). This is disclosed assumption, not silent substitution of a missing user input.
3. **Do not chain public `CreateInstrument` / `RecordChange` / `AppendManualQuote` inside product transactions.** Product recipes use transaction-local acquisition.
4. **Retired Wails APIs around `980151c` were not restored.** Liquidity is a new `internal/wailsapi/liquidity` service; bindings are generated, not hand-patched.
5. **`ListOperations` cursor** is `createdAtRFC3339Nano|id`. Invalid cursors are validation errors.
6. **Known totals must not become zero** when an input is missing. UI and evaluators keep null MoneyView fields and `partial`/`unavailable` status.
7. **Renewal is one operation**, not a settle plus a separate open.

### Native gate

Linux cloud worker cannot run the macOS native walkthrough in section 17 (seeded DB, restart persistence, keyboard-only native, three-locale native inspect, in-app backup restore). Automated Go/frontend gates are the acceptance evidence on this worker. Native items remain unchecked.
