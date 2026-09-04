# Nestworth Analytics Redesign — Development Plan

**Status:** Phase 1a–2a complete; Phases 2b–6 not started
**Product:** Nestworth  
**Companions:** [Product design](analytics-product-design.md), [Wireframes](analytics-wireframes.md), [Technical architecture](analytics-redesign-architecture.md)

This document splits the redesign into phases that can be finished and verified on their own. Calculation rules live in the [architecture](analytics-redesign-architecture.md). This file only covers **order, boundaries, and exit criteria**.

Do not start UI work until the engine golden cases behind it pass. Reconciling beginning to ending is not enough if Price, FX, or return % are wrong — and a base-valuation run that reconciles because FX absorbed the error is the specific failure this gate exists to catch.

---

## 1. How to read this plan

Eight phases: the engine and the API layer are each split along the Asset Changes / Return seam, because the two tracks have no dependency on each other. Each phase:

- produces a usable increment
- has an explicit **done when**
- can stay on a branch or land independently
- does not require the next phase to be valuable

Granularity is phase-level, with a few tasks inside each phase. Do not split further unless a phase is blocked.

The old Analysis page stays until Phase 6. Investments continues to use `HoldingGain` / `AccountGains` throughout.

---

## 2. Phase map

```text
Phase 1a  Universe, classifier, Asset Changes identity, Price/FX     Go only
    │
    ├───────────────────────────────┐
    ▼                               ▼
Phase 1b  Return, Dietz, linking    Phase 2a  Asset Changes Wails APIs
    │                               │  memo + DTO plumbing
    │                               ├─────────┐
    ▼                               │         ▼
Phase 2b  Return Wails APIs ◀───────┘         Phase 4  Asset Change Drivers
    │                                         │
    ▼                                         │
Phase 3  Insights shell                       │
         Return Calendar                      │
    │                                         │
    └────────────────────┬────────────────────┘
                         ▼
Phase 5  Remaining tabs
         Return Trend / Contribution / Asset Trend / Categories
                         │
                         ▼
Phase 6  History deep-link, remove old Analysis, docs
```

Phase 2b takes the memo and DTO plumbing from 2a rather than building it twice, so 2a is on both branches.

**Why 1a and 1b are separate.** Asset Change Drivers consumes only `AssetBucket`. It needs nothing from `ReturnComponent`, `DietzCapitalFlow`, or geometric linking. Gating it behind the entire engine — universe resolution, classifier, path-aware Price/FX, Dietz, linking, and cases 1–44 including 17b in one unit — serialises work that has no dependency and produces a single very large gate. Splitting gives two smaller gates and lets the Asset Changes track start while the return maths is still in flight.

The split also surfaces the conditional schema v10 quantity decision early, in 1a, instead of deep inside a monolith.

Phases 3 and 4 may proceed in parallel. Phase 5 needs the Insights shell from Phase 3 (shared filters, tabs, sheets). Cross-page links wait until both 3 and 4 exist.

If the team is one person working serially, `1a → 1b → 2a → 2b → 3 → 4 → 5 → 6` is the same work in the same order; the split still buys two smaller review units and two earlier green gates.

---

## 3. Dependencies

| Phase | Depends on | Must not start before |
|---|---|---|
| 1a Universe / classifier / Price+FX | Current snapshots, effects, `GainService` | — |
| 1b Return / Dietz / linking | Phase 1a | 1a golden cases green |
| 2a Asset Changes APIs | Phase 1a | 1a golden cases green |
| 2b Return APIs | Phase 1b, Phase 2a (shares the memo + DTO plumbing) | 1b golden cases green |
| 3 Insights shell + Return Calendar | Phase 2b | Return DTOs callable from the frontend |
| 4 Change Drivers | Phase 2a | Asset Changes DTOs callable from the frontend |
| 5 Remaining tabs | Phases 3 and 4 (shell + at least one page pattern) | Shared filter bar and sheet pattern exist |
| 6 Cutover | Phase 5 | Both pages have their three tabs |

Conditional inside Phase 1a: if `qty = nativeAmount / unitPrice` fails golden cases, add snapshot-item `quantity` (schema v10) in the same phase. Do not defer that to UI work. The path-aware Price/FX bridge and the corporate-action rules ([architecture §8.1.1](analytics-redesign-architecture.md)) are the cases most likely to force this, and they are all in 1a.

Phase 4 can therefore start against 2a while 1b is still in progress. Its only cross-page dependency — the Calendar → Drivers deep-link — is explicitly deferred to Phase 6.

**Golden case allocation.** Architecture §16 cases 1–44 (including 17b) are each owned by exactly one phase, and a phase does not close until its own cases are green:

| Phase | Cases |
|---|---|
| 1a | 1–3, 8–10, 13–17b, 23–29, 31–36, 38, 40–41 |
| 1b | 4–7, 11–12, 18–22, 30, 37, 39, 42 |
| 2a | 44 (memo staleness) |
| 2b | 43 (per-projection forced valuation) |

Cases 43 and 44 are API-layer behaviour, not engine maths, which is why they sit in 2a/2b rather than 1a/1b. This table is the authority; if a case number appears in two phases' exit criteria, this table wins.

---

## 4. Phase 1a — Universe, classifier, Asset Changes identity

**Status:** Complete (Go kernel and Phase 1a golden-case coverage verified)

**Goal:** every value movement is classified once, and signed physical amounts reconcile beginning to ending.

**Includes:**

- Domain types: `AnalysisQuery` (typed `ScopeKind` / `Valuation` / `ReturnBasis`), `AnalysisUniverse`, `InvestmentUniverse`, `AttributionBucket`, `ResolvedAnalysisContext`
- `ScopeEffectClassifier` for every value-moving leg, not only `internal_transfer`
- The `interest` reason and its precedence row ([architecture §5.2, §7.2](analytics-redesign-architecture.md))
- Signed `AssetBucket` contributions that **sum** to ending − beginning (never `- Spending` / `- Fees`)
- Path-aware holding Price / FX by **formula**, with Residual computed afterwards and able to be non-zero in base valuation as well as native
- Cash FX by formula, subtracting **all** explicit cash movements, not only principal
- Corporate actions and unpriced quantity changes (§8.1.1): split restatement, in-kind transfer, and refusing to synthesise a price
- Currency-aware residual tolerance (§4.1)
- History Origin timezone day assignment, including non-24h DST days
- `ComponentDay` / `PeriodAnalysisResult` (§11.1) with the `AssetBuckets` half populated

**Does not include:** `ReturnComponent`, `DietzCapitalFlow`, rates, linking, Wails methods, React pages.

**Done when:**

- Golden cases 1–3, 8–10, 13–17b, 23–29, 31–36, 38, 40–41 pass in `internal/application/analysis_*_test.go`
- Household transfer is not wealth creation; account-scope rewrite of the same transfer is an inflow
- Beginning + `AssetBucket` drivers + residual = ending, for every scope in the fixture household
- Liabilities contribute with negative sign; loan drawdown and principal repayment are net worth 0
- Cash loan interest is Spending −500; capitalised interest is LiabilityImpact
- Mid-day buy then mark-to-market is Price, not FX; the Price×FX cross-term is FX
- USD cash noon salary: Income at event FX; the remainder is CashFXImpact, not the whole Δbase
- **Case 33 specifically:** an injected quantity error lands in Residual and marks `partial` in base valuation, not only in native
- A 4:1 split produces Price 0 and no phantom acquisition
- Missing quote is never a 0 / ¥0 value
- Snapshot-item `quantity` (schema v10) added here if inferred quantity cannot carry these cases

**Verify:** `go test` on the analysis packages. No desktop GUI.

---

## 5. Phase 1b — Return, Dietz, linking

**Status:** Complete (Go kernel and Phase 1b golden-case coverage verified)

**Goal:** rates and return amounts whose financial meaning is correct, on top of the 1a kernel.

**Includes:**

- `ReturnComponent` and `DietzCapitalFlow` on `AttributedEffect`; the `ReturnComponents` / `DietzFlow` / `BeginningValue` half of `ComponentDay`
- Economic association (`RelatedInstrumentID` / `RelatedHoldingID`) for instrument-scope dividend and investment fee **without** putting those cash legs on the QQQ waterfall
- Orthogonal `DietzCapitalFlow` (not equal to `ExternalToScopeFlows`); salary into included cash is Income **and** Dietz capital; interest is return and **not** Dietz capital
- Daily Modified Dietz, geometric linking, and rate coverage (`ratedDays` / `totalDays`, §5.1)
- Group-level folding for Contribution (§9.1) — one component pass, not one engine pass per group

**Does not include:** Wails methods, React pages.

**Done when:**

- The remaining golden cases pass: 4–7, 11–12, 18–22, 30, 37, 39, 42
- Same-day noon contribution **and** noon salary-into-included-cash: Dietz % uses weighted `DietzCapitalFlow`, not `amount / beginning` and not the ExternalFlow bucket alone
- Salary is Income once; with `IncludeCash = false` it is not Dietz capital
- **Case 37 specifically:** interest raises the household return rate; the same amount as `reason=income` lowers it, and the two produce measurably different rates
- QQQ ex-dividend: asset identity is PriceChange only; dividend is `ReturnComponent` with `AssetBucket` nil
- QQQ instrument scope: dividend/commission belong to QQQ return even when cash is outside components
- `cash_dividend` is net cash, not gross minus synthetic tax; unassociated tax is not `ReturnInvestmentFee`
- Bank maintenance fee is not `ReturnInvestmentFee`
- **Case 42 specifically:** folded group Dietz equals a full per-group engine re-run, which is what licenses the single-pass implementation
- A period with poor rate coverage reports `partial` and a day count, or no rate at all — never a rate silently compounded from a minority of days

**Verify:** `go test` on the analysis packages. No desktop GUI.

---

## 6. Phase 2a — Asset Changes Wails APIs

**Status:** Complete (Asset Changes Wails APIs, memo, DTO plumbing, and Phase 2a exit criteria verified)

**Goal:** view-shaped Asset Changes reads over one memoized `PeriodAnalysisResult`, plus the shared API plumbing.

**Includes:**

- `internal/wailsapi/analysis/` registered next to existing analytics
- Wire DTOs: canonical amounts, `available` / `status` / `missingReason`, `valuationForced`
- Projections: AssetChange, AssetDriverDetail, AssetTrend, Categories, CategoryDetail
- Memo: query hash + in-process `analysisDataGeneration`, as a **bounded LRU of 2** (§11.2) — not `dirty_from`, not an unbounded map
- Increment or clear generation on activity, snapshot rebuild, quote/FX, and holding/account mutation
- Reuse `ensureClosedDaySnapshots` for the requested window
- Keep `HoldingGain` / `AccountGains` / `RealizedGain` / `DividendIncome` for now

**Does not include:** return projections, removing the old Analysis page, frontend queries beyond what tests need.

**Done when:**

- Each view method returns a projection of one compute, not a second full replay
- Native valuation forces base against `AnalysisUniverse` and returns `valuationForced` per response (§6.1)
- Asset Trend week/month aggregation follows §10.3 — levels end-of-period, flows summed, never averaged levels
- **Golden case 44 passes:** mutate an activity, re-query the identical window, assert the result changed; and again for a snapshot rebuild that restores a previous `dirty_from`
- The memo holds at most 2 entries under a filter-exploration workload
- Performance budgets in architecture §18.1 are met on a fixture household at the upper bound (~500 components, 3Y window), not on a three-account fixture
- A thin Go/Wails test asserts DTO shape and completeness flags

**Verify:** application + wailsapi tests; regenerate bindings. Frontend still shows Analysis.

---

## 7. Phase 2b — Return Wails APIs

**Goal:** the return half of the surface, reusing 2a's DTO and memo plumbing.

**Includes:**

- Projections: ReturnCalendar, ReturnDay, ReturnTrend, Contribution, ContributionItem
- `cells[]` carries the per-day `ReturnComponent` composition inline for hover (§12.1) — not one `ReturnDay` call per cell
- `ReturnCalendar.issues[]` for the period-level incomplete-data list
- `summary` carries beginning/ending invested value, return amount, return rate, and rate coverage
- `ContributionSort` enum, with rate sorts falling back to `amount_desc` on views whose `%` is omitted

**Done when:**

- Native valuation forces base against `InvestmentUniverse`, independently of the Asset Changes answer (**golden case 43**)
- Contribution `Unrealized` is range-end cost-basis floating gain, or the field is omitted — never `Total − Realized − Dividend`
- Contribution Total Return % is group Dietz; Realized and Dividend omit `%`
- Rate-bearing DTOs carry `ratedDays` / `totalDays`
- Performance budgets in §18.1 are met for Contribution over the same upper-bound fixture

**Verify:** application + wailsapi tests; regenerate bindings. Frontend still shows Analysis.

---

## 8. Phase 3 — Insights shell and Return Calendar

**Goal:** first user-visible replacement page.

**Includes:**

- Nav: `return-analysis` and `asset-changes` under Insights; keep `analytics` until Phase 6
- Typed `NavigationTarget` and session filter store (scope, valuation, cash, dates)
- Shared `AnalysisFilterBar`, completeness banner, empty/loading skeletons
- Return Calendar month / year, day sheet, Dietz % + amount, hover composition from `cells[]`
- Completeness banner that opens the period `issues[]` list — not a dead-end message
- `Today` renders muted with its explanatory state; it does not show a zero day
- i18n for this slice in all three locales + `additions.ts`, using the architecture §19 terminology table
- Asset Changes may be a titled placeholder that preserves filters (only if Phase 4 has not landed first)

**Does not include:** waterfall, contribution table, removing Analysis.

**Done when:**

- Calendar navigates month/year; day opens a right sheet
- Incomplete days are visually distinct from true zero
- A `%` whose `ratedDays < totalDays` is marked and explains its coverage
- Filters survive Calendar ↔ (placeholder) tab switches, and a forced base valuation on one tab does not overwrite the stored `valuation`
- Empty household-with-no-investments state matches the product copy
- Frontend tests cover calendar navigation, partial-day rendering, and rate-coverage marking

**Verify:** frontend unit tests; Wails desktop smoke of Calendar only.

Day-sheet link “View Asset Changes for This Day” may stay disabled until Phase 4.

---

## 9. Phase 4 — Asset Change Drivers

**Goal:** the second mental model: why value changed.

**Includes:**

- Asset Changes page header, period + scope. Owns the shared filter store if it lands before Phase 3; otherwise reuses it.
- Summary (beginning → ending, signed change)
- Waterfall (stacked bars) that reconciles, with `Unexplained difference` as its own bar, visually distinct from `Adjustments`
- Grouped attribution list (cash flow vs market vs other), Residual as its own row in `other`
- Driver detail sheet, including a residual detail that names the component and day
- Enable Calendar → Drivers deep-link once both pages exist

**Does not include:** Asset Trend, Categories, History.

**Done when:**

- Waterfall beginning + `AssetBucket` drivers + residual = ending
- Internal household transfers do not appear as wealth creation
- Zero drivers are omitted; non-zero Residual is visible and marks partial
- Residual has a designed surface, not a hidden one — the architecture treats it as first-class and the UI must too
- Driver sheet can later open Contribution (link may wait for Phase 5)

**Verify:** frontend tests for grouping and residual; desktop smoke of Drivers with a known fixture household.

---

## 10. Phase 5 — Remaining tabs

**Goal:** the other four tabs on the same kernel, without new math.

**Includes:**

- Return Trend: one series; cumulative amount / linked Dietz % / period amount (never average %)
- Contribution: ranked table; independent Total / Realized / Unrealized / Dividend views; item sheet; Total Return % = group Dietz; Realized and Dividend omit %; Unrealized % is range-end cost-basis ratio or omitted
- Asset Trend: one metric, day / week / month; levels = end-of-period, flows = sum, return amount = sum, return % = geometric Dietz
- Categories: type selector, ranked list, optional donut, category sheet (account or instrument grouping, not a budget tree)

**Does not include:** deleting Analysis, History `instrumentId` (that can land here if cheap, or in Phase 6).

**Done when:**

- Switching tabs does not reset shared filters
- Contribution types are not presented as a closed decomposition of Total
- Contribution Return % follows architecture §9.1 (Dietz only on Total Return; no `%` on Realized/Dividend)
- Week / month Asset Trend does not average Net Worth or sum Return %
- Empty charts are not rendered
- Query invalidation on activity/valuation changes covers the new keys

**Verify:** frontend tests per tab; desktop pass over all six tabs with one scope/date set.

Phases 3 and 4 must be present so the shell and two default tabs already exist. The four tabs in this phase can be implemented sequentially in the order above; they do not block each other once the shell is there.

---

## 11. Phase 6 — History, cutover, docs

**Goal:** the workspace replaces Analysis.

**Includes:**

- History accepts an initial filter payload (`kinds`, dates, `accountId`, optional `instrumentId`)
- Sheets’ “View in History” / `historyHint` actually navigate
- Cross-page links: Calendar day → Drivers; Drivers → Contribution
- Remove `analytics` nav item and `AnalyticsPage`
- Rewrite or replace `AnalyticsPage.test.tsx`
- Update [visual analytics](../visual-analytics-and-market-history.md) and [design README](../README.md) screen map

**Done when:**

- Sidebar has Return Analysis and Asset Changes only under Insights
- Analysis is gone; Investments still loads holding gains
- Deep-links preserve date range and scope
- Locale coverage and hardcoded-UI tests pass

**Verify:** full frontend suite, focused Go tests, desktop walk of Calendar → Drivers → Contribution → History.

---

## 12. Implementation order

Default serial order (safest):

```text
1a → 1b → 2a → 2b → 3 → 4 → 5 → 6
```

Allowed overlap, once 1a is green:

```text
1a ─┬→ 1b → 2b → 3 ─┐
    │                ├→ 5 → 6
    └→ 2a → 4 ───────┘
```

The overlap drawing omits the 2a → 2b edge (shared memo and DTO plumbing). Phase 2b still depends on Phase 2a as in §3.

Do not overlap 1a or 1b with UI. Do not start Phase 6 while any of the six tabs is still a placeholder.

Suggested landing points if the work is split across PRs:

1. Universe + classifier + Asset Changes identity + Price/FX, with golden tests (Phase 1a)
2. Return + Dietz + linking, with golden tests (Phase 1b)
3. Asset Changes Wails service, memo, DTO plumbing (Phase 2a)
4. Return Wails service (Phase 2b)
5. Nav + Calendar (Phase 3)
6. Drivers (Phase 4)
7. Other tabs (Phase 5, one PR or one PR per tab)
8. Cutover (Phase 6)

---

## 13. Out of scope for this plan

Same cuts as the architecture:

- No XIRR / money-weighted IRR. No intra-day mark-to-market TWR. Daily Modified Dietz + geometric linking.
- Tax engine
- Budget category tree and tags
- Multi-select scope
- Filters persisted across app restarts
- ECharts Calendar / Heatmap modules
- Per-currency native sections; multi-currency native forces base with a reason
- "Investment vs non-investment" and "include liabilities" filters
- Contribution data-table / export view
- Split-adjusted price history and spin-off cost-basis allocation (corporate actions are recognised only well enough not to fabricate Price)

If schema v10 quantity is required, it is part of Phase 1a, not a later cleanup.

---

## 14. Progress checklist

| Phase | Status |
|---|---|
| 1a Universe, classifier, identity, Price/FX | Complete |
| 1b Return, Dietz, linking | Complete |
| 2a Asset Changes Wails APIs | Complete |
| 2b Return Wails APIs | Not started |
| 3 Insights shell + Return Calendar | Not started |
| 4 Asset Change Drivers | Not started |
| 5 Remaining tabs | Not started |
| 6 History, cutover, docs | Not started |
