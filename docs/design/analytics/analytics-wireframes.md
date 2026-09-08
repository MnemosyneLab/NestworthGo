# Nestworth Analytics Redesign — Wireframe Specification

**Status:** Final wireframe document  
**Product:** Nestworth  
**Target:** Desktop application (Wails v3)  
**Navigation area:** Insights  
**Pages:** `Return Analysis`, `Asset Changes`  
**Companions:** [Product design](analytics-product-design.md), [Technical architecture](analytics-redesign-architecture.md)

---

# 1. Navigation

The existing `Analysis` item is removed from the sidebar.

Under the current `Insights` section:

```text
Insights
  Return Analysis
  Asset Changes
```

Recommended sidebar structure:

```text
Overview
Accounts
Portfolio
Instruments

Activity
  History

Insights
  Return Analysis
  Asset Changes

Management
  Directory

Settings
  Market Data
  Settings
```

There is no intermediate analytics landing page.

---

# 2. Return Analysis — Overall Page

Default secondary tab: `Return Calendar`

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ Return Analysis                                                              │
│                                                                              │
│ Understand investment performance and what contributed to it.                │
│                                                                              │
│ [ Return Calendar ]   [ Return Trend ]   [ Contribution ]                     │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│ Scope              Return Basis         Valuation             Cash            │
│ [ Household ▼ ]    [ Investment ▼ ]     [ Base CNY ▼ ]        [ Include ▼ ]  │
│                                                                              │
│                                               [ More Filters ]   [ Reset ]    │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

Notes:

- The filter row is shared across all Return Analysis tabs.
- Switching tabs preserves the current scope and filters.
- `More Filters` contains secondary dimensions such as account, asset class, currency, instrument, and member. Tags are omitted in v1.
- Return Basis is a typed query field fixed to `investment` in v1. The UI may hide that control entirely.
- Avoid placing another large bordered container around the entire page.

---

# 3. Return Analysis — Return Calendar / Month View

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ September 2026                                           [ ‹ ] [ Today ] [ › ]│
│                                                        [ Month | Year ]      │
│                                                                              │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │ THIS MONTH                                                               │ │
│ │                                                                          │ │
│ │ Total Return                                                             │ │
│ │ +¥12,430.28                     +2.51% ◇                             │ │
│ │                                                                          │ │
│ │ Price Change       Dividend & Interest       FX Impact       Fees         │ │
│ │ +¥9,820            +¥620                     +¥2,110         -¥119.72     │ │
│ │                                                                          │ │
│ │ Beginning Invested Value                 Ending Invested Value            │ │
│ │ ¥487,340                                ¥503,120                         │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │ Mon       Tue       Wed       Thu       Fri       Sat       Sun           │ │
│ │                                                                          │ │
│ │           1         2         3         4         5         6             │ │
│ │         +0.31%    -0.42%    +0.65%    +0.12%    +0.84%    +0.09%         │ │
│ │         +1,240    -1,680    +3,241      +620    +4,120      +430         │ │
│ │                                                                          │ │
│ │  7         8         9        10        11        12        13            │ │
│ │ -0.12%    +0.81%    +0.17%    ...                                      │ │
│ │  -580     +3,940      +820                                               │ │
│ │                                                                          │ │
│ │ 14        15        16        17        18        19        20            │ │
│ │ ...                                                                      │ │
│ │                                                                          │ │
│ │ 21        22        23        24        25        26        27            │ │
│ │ ...                                                                      │ │
│ │                                                                          │ │
│ │ 28        29        30                                                    │ │
│ │                                                                          │ │
│ │  Positive    Negative    Neutral / No change    Partial data             │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│ Top Contributors This Month                                                  │
│                                                                              │
│ QQQ                 +¥4,210        NVDA                +¥3,180               │
│ USD FX              +¥2,110        SOXQ                -¥1,320               │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

`Today` is not a closed snapshot: the preset selects the range, but the current day renders muted with an explanatory state. It is not a zero day and is not silently shifted to yesterday.

When `ratedDays` < `totalDays`, the summary **%** carries a marker (`◇`) whose tooltip states the covered day count. The amount is never marked by rate coverage alone.

V1 does not navigate from Top Contributors to Contribution. Calendar day → Change Drivers is the v1 cross-page link.

## Calendar cell

Each day cell:

```text
┌──────────────┐
│ 3            │
│              │
│ +0.65%       │
│ +¥3,241      │
│              │
└──────────────┘
```

Optional status treatment:

```text
Complete day     normal
Positive day     subtle green background
Negative day     subtle red background
Near-zero day    neutral background
Partial day      small warning / incomplete marker
No data          muted
Future date      disabled / muted
```

Do not treat missing data as a true 0% return.

---

# 4. Return Calendar — Hover State

Hovering a day shows a compact popover:

```text
┌────────────────────────────┐
│ Sep 3, 2026                │
│                            │
│ Daily Return  +¥3,241      │
│ Daily Return % +0.65%      │
│                            │
│ Price Change  +¥2,581      │
│ FX Impact       +¥640      │
│ Dividend & Interest +¥20   │
│ Fees                ¥0      │
└────────────────────────────┘
```

The hover should be informational only.

Click opens the detailed side sheet.

---

# 5. Return Calendar — Daily Detail Sheet

```text
                                                       ┌───────────────────────┐
                                                       │ September 3, 2026   × │
                                                       │                       │
                                                       │ DAILY RETURN          │
                                                       │                       │
                                                       │ +¥3,241.18            │
                                                       │ +0.65%                │
                                                       │                       │
                                                       │ ────────────────────  │
                                                       │                       │
                                                       │ Return Composition    │
                                                       │                       │
                                                       │ Price Change +¥2,581  │
                                                       │ FX Impact       +¥640 │
                                                       │ Dividend & Interest   │
                                                       │                  +¥20 │
                                                       │ Fees               ¥0 │
                                                       │                       │
                                                       │ ────────────────────  │
                                                       │                       │
                                                       │ Top Contributors      │
                                                       │                       │
                                                       │ NVDA          +¥1,820 │
                                                       │ QQQ             +¥940 │
                                                       │ VOO             +¥351 │
                                                       │ SOXQ            -¥510 │
                                                       │                       │
                                                       │ [ View Full Return    │
                                                       │   Details ]           │
                                                       │                       │
                                                       │ [ View Asset Changes  │
                                                       │   for This Day → ]    │
                                                       └───────────────────────┘
```

Interaction:

- The page remains visible behind the sheet.
- Close via `×`, Escape, or outside click.
- `View Full Return Details` is later (not v1). It must not navigate to Contribution.
- `View Asset Changes for This Day` navigates to `Asset Changes > Change Drivers`.

---

# 6. Return Analysis — Return Calendar / Year View

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ 2026                                                    [ ‹ ] [ This Year ] [ › ]
│                                                        [ Month | Year ]      │
│                                                                              │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │ ANNUAL RETURN                                                            │ │
│ │                                                                          │ │
│ │ +¥46,280.16                     +9.34% ◇                             │ │
│ │                                                                          │ │
│ │ Price Change       Dividend & Interest       FX Impact       Fees         │ │
│ │ +¥38,210           +¥3,820                    +¥5,040        -¥789.84     │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│ ┌────────────────┐ ┌────────────────┐ ┌────────────────┐ ┌────────────────┐ │
│ │ January        │ │ February       │ │ March          │ │ April          │ │
│ │                │ │                │ │                │ │                │ │
│ │ +2.10%         │ │ -1.24%         │ │ +4.31%         │ │ +0.54%         │ │
│ │ +¥8,320        │ │ -¥4,210        │ │ +¥16,820       │ │ +¥2,140        │ │
│ └────────────────┘ └────────────────┘ └────────────────┘ └────────────────┘ │
│                                                                              │
│ ┌────────────────┐ ┌────────────────┐ ┌────────────────┐ ┌────────────────┐ │
│ │ May            │ │ June           │ │ July           │ │ August         │ │
│ │ ...            │ │ ...            │ │ ...            │ │ ...            │ │
│ └────────────────┘ └────────────────┘ └────────────────┘ └────────────────┘ │
│                                                                              │
│ ┌────────────────┐ ┌────────────────┐ ┌────────────────┐ ┌────────────────┐ │
│ │ September      │ │ October        │ │ November       │ │ December       │ │
│ │                │ │                │ │                │ │                │ │
│ │ +2.51%         │ │ --             │ │ --             │ │ --             │ │
│ │ +¥12,430       │ │                │ │                │ │                │ │
│ └────────────────┘ └────────────────┘ └────────────────┘ └────────────────┘ │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

Interaction:

```text
Year -> click month -> Month View
Month -> click day -> Daily Detail Sheet
```

Do not open a side sheet when clicking a month.

---

# 7. Return Analysis — Return Trend

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ Return Analysis                                                              │
│                                                                              │
│ [ Return Calendar ]   [ Return Trend ]   [ Contribution ]                     │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│ Scope              Return Basis         Valuation             Cash            │
│ [ Household ▼ ]    [ Investment ▼ ]     [ Base CNY ▼ ]        [ Include ▼ ]  │
│                                                                              │
│ Range                                                                        │
│ [ 30D ] [ YTD ] [ 1Y ] [ 3Y ] [ All ]                       [ Custom ]       │
│                                                                              │
│ Display                                                                      │
│ [ Cumulative Return ] [ Return % ] [ Period Return ]                          │
│                                                                              │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │ Period Return                                                           │ │
│ │ +¥46,280                       +9.34% ◇                              │ │
│ │                                                                          │ │
│ │ +50K ┤                                                    ╭──────        │ │
│ │ +40K ┤                                         ╭──────────╯              │ │
│ │ +30K ┤                              ╭──────────╯                         │ │
│ │ +20K ┤                   ╭──────────╯                                    │ │
│ │ +10K ┤        ╭──────────╯                                               │ │
│ │    0 ┼────────╯                                                          │ │
│ │       Jan        Mar        May        Jul        Sep                     │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│ Return Sources                                                               │
│                                                                              │
│ Price Change                       +¥38,210                82.6%             │
│ Dividend & Interest                 +¥3,820                 8.3%             │
│ FX Impact                           +¥5,040                10.9%             │
│ Fees                                  -¥790                -1.7%             │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

Chart rules:

- One primary metric at a time.
- Do not show net worth, total assets, liabilities, and investment returns on the same chart.
- Hover displays values for the selected point.
- Range changes update both chart and return-source summary.

---

# 8. Return Trend — Hover

```text
┌─────────────────────────────┐
│ Aug 12, 2026                │
│                             │
│ Cumulative Return +¥32,840  │
│ Daily Return          +¥920 │
│ Daily Return %       +0.19% │
└─────────────────────────────┘
```

---

# 9. Return Analysis — Contribution

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ Return Analysis                                                              │
│                                                                              │
│ [ Return Calendar ]   [ Return Trend ]   [ Contribution ]                     │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│ Scope              Valuation             Cash                                │
│ [ Household ▼ ]    [ Base CNY ▼ ]        [ Include ▼ ]                      │
│                                                                              │
│ Return Type                  Group By                 Sort                    │
│ [ Total Return ]             [ Instrument ▼ ]         [ Contribution ▼ ]     │
│ [ Realized Gain ] [ Unrealized Gain ] [ Dividend & Interest ]                │
│                                                                              │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │ Instrument                         Contribution        Return %            │ │
│ │                                                                          │ │
│ │ Defiance Quantum ETF               +¥14,056            +8.2%             │ │
│ │ ███████████████████████                                                  │ │
│ │                                                                          │ │
│ │ Vanguard S&P 500 ETF               +¥10,005            +4.3%             │ │
│ │ █████████████████                                                        │ │
│ │                                                                          │ │
│ │ NVDA                                 +¥7,820           +12.4%             │ │
│ │ █████████████                                                            │ │
│ │                                                                          │ │
│ │ QQQ                                  +¥5,940            +6.1%             │ │
│ │ ██████████                                                               │ │
│ │                                                                          │ │
│ │ USD Cash                             +¥1,810            +1.2%             │ │
│ │ ███                                                                      │ │
│ │                                                                          │ │
│ │ SOXQ                                 -¥3,521            -3.8%             │ │
│ │ ◀██████                                                                  │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

These four return types are independent views. They are not a partition of Total Return and are not expected to sum to it.

**Total Return** includes the Return % column (group Dietz, never amount / opening). **Unrealized Gain** may include % only as remaining-cost ratio when remaining cost is positive; otherwise omit the column. **Realized Gain** and **Dividend & Interest** have no % column — do not leave a blank:

```text
Instrument                         Contribution

Defiance Quantum ETF               +¥14,056
Vanguard S&P 500 ETF               +¥10,005
NVDA                                 +¥7,820
QQQ                                  +¥5,940
```

`Group By` options:

```text
Instrument
Account
Currency
Asset Class
```

A table-first layout is preferred over a giant horizontal bar chart.

---

# 10. Contribution — Detail Sheet

```text
                                                       ┌───────────────────────┐
                                                       │ QQQ                 × │
                                                       │                       │
                                                       │ TOTAL RETURN          │
                                                       │ +¥5,940               │
                                                       │ +6.1%                 │
                                                       │                       │
                                                       │ Independent views —   │
                                                       │ not expected to sum   │
                                                       │ to Total Return.      │
                                                       │                       │
                                                       │ ────────────────────  │
                                                       │                       │
                                                       │ Composition           │
                                                       │                       │
                                                       │ Price Change +¥5,120  │
                                                       │ Dividend & Interest   │
                                                       │                +¥480  │
                                                       │ FX Impact      +¥420  │
                                                       │ Fees            -¥80  │
                                                       │                       │
                                                       │ ────────────────────  │
                                                       │                       │
                                                       │ Accounts              │
                                                       │ MooMoo SG    +¥5,400  │
                                                       │ IBKR           +¥540  │
                                                       │                       │
                                                       │ [ View in History ]   │
                                                       └───────────────────────┘
```

The sheet follows the **active** return type only. Do not stack Realized Gain and Unrealized Gain next to Price / Dividend / FX / Fees as if they partitioned Total Return. Realized Gain and Dividend & Interest sheets omit %. Unrealized Gain shows the range-end cost-basis figure.

---

# 11. Asset Changes — Overall Page

Default secondary tab: `Change Drivers`

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ Asset Changes                                                                │
│                                                                              │
│ Understand why assets and net worth increased or decreased.                  │
│                                                                              │
│ [ Change Drivers ]   [ Asset Trend ]   [ Categories ]                        │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│ Period                                                                       │
│ [ This Month ▼ ]        Sep 1, 2026 — Sep 30, 2026                          │
│                                                                              │
│ Scope              Valuation             Cash                                │
│ [ Household ▼ ]    [ Base CNY ▼ ]        [ Include ▼ ]                      │
│                                               [ More Filters ]               │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

The session store is shared with Return Analysis, so this header uses the same filter set (Period, Scope, Valuation, Cash, More Filters). Return Basis may be hidden entirely in v1.

---

# 12. Asset Changes — Change Drivers

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │ NET WORTH CHANGE                                                        │ │
│ │                                                                          │ │
│ │ Beginning                     Ending                                    │ │
│ │ ¥476,684.62        →          ¥494,817.08                               │ │
│ │                                                                          │ │
│ │ Change                                                                   │ │
│ │ +¥18,132.46                   +3.80%                                    │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│ Change Drivers                                                               │
│                                                                              │
│             +20,000                                                         │
│               ┌───┐                         +4,830                           │
│               │   │             +620         ┌───┐                          │
│ ¥500K ┤       │   │   +5,000     ┌───┐       │   │         ┌────────────┐  │
│       │       │   │    ┌───┐     │   │       │   │         │ 494,817    │  │
│ ¥490K ┤       │   │    │   │     │   │       │   │         │ Ending     │  │
│       │       │   │    │   │     │   │       │   │         └────────────┘  │
│ ¥480K ┤ ┌────────────┐ │   │     │   │       │   │                        │
│       │ │ 476,684    │ │   │     │   │       │   │                        │
│ ¥470K ┤ │ Beginning  │ │   │ -8,200  │       │   │ -3,797  -320 ░37    │
│       │ └────────────┘                                                           
│       └──────────────────────────────────────────────────────────────────   │
│        Start ExtFlow Income Spend  Div  Price   FX  Fees Resid End           │
│                                                                              │
│ Asset Change Composition                                                     │
│                                                                              │
│ CASH FLOW                                                                    │
│ External Flows                                        +¥20,000              │
│ Income                                                 +¥5,000              │
│ Spending                                               -¥8,200              │
│                                                       ─────────              │
│ Net External Flows                                    +¥16,800              │
│                                                                              │
│ MARKET & INVESTMENT                                                         │
│ Price Change                                           +¥4,830              │
│ Dividend & Interest                                      +¥620              │
│ FX Impact                                              -¥3,797              │
│ Fees                                                     -¥320              │
│                                                       ─────────              │
│ Investment & Market Impact                             +¥1,333              │
│                                                                              │
│ OTHER                                                                        │
│ Unexplained difference                                   +¥37               │
│                                                                              │
│ TOTAL ASSET CHANGE                                    +¥18,170              │
└──────────────────────────────────────────────────────────────────────────────┘
```

The Unexplained difference waterfall bar is hatched (or otherwise visually distinct) from Adjustments. Show it only when above tolerance. Do not merge it into Adjustments.

Important:

- Cash flow and investment/market impact are visually separated.
- Internal transfers at household scope do not appear as wealth creation.
- Zero-value categories can be omitted.
- The waterfall must reconcile beginning value to ending value.

---

# 13. Change Driver — Detail Sheet

Example: click `Price Change`.

```text
                                                       ┌───────────────────────┐
                                                       │ Price Change        × │
                                                       │                       │
                                                       │ Sep 1 — Sep 30, 2026  │
                                                       │                       │
                                                       │ TOTAL                 │
                                                       │ +¥4,830               │
                                                       │                       │
                                                       │ ────────────────────  │
                                                       │                       │
                                                       │ By Instrument         │
                                                       │                       │
                                                       │ QQQ          +¥2,340  │
                                                       │ NVDA         +¥1,820  │
                                                       │ VOO            +¥810  │
                                                       │ SOXQ           -¥140  │
                                                       │                       │
                                                       │ ────────────────────  │
                                                       │                       │
                                                       │ By Account            │
                                                       │ MooMoo SG    +¥4,690  │
                                                       │ IBKR           +¥140  │
                                                       │                       │
                                                       │ [ Open Return         │
                                                       │   Analysis → ]        │
                                                       └───────────────────────┘
```

Unexplained difference (when above tolerance):

```text
                                                       ┌───────────────────────┐
                                                       │ Unexplained         × │
                                                       │ difference            │
                                                       │                       │
                                                       │ Sep 1 — Sep 30, 2026  │
                                                       │                       │
                                                       │ TOTAL                 │
                                                       │ +¥37                  │
                                                       │                       │
                                                       │ QQQ · Sep 12          │
                                                       │ unpriced qty change   │
                                                       │                       │
                                                       │ [ View in History ]   │
                                                       └───────────────────────┘
```

Navigation target:

```text
Return Analysis
Tab = Contribution
Date range = preserved
Scope = preserved
Return Type = relevant type
```

---

# 14. Asset Changes — Asset Trend

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ Asset Changes                                                                │
│                                                                              │
│ [ Change Drivers ]   [ Asset Trend ]   [ Categories ]                        │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│ Granularity                         Metric                                    │
│ [ Day ] [ Week ] [ Month ]          [ Net Worth ▼ ]                         │
│                                                                              │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │                                                                          │ │
│ │ ¥510K ┤                                             ╭──────              │ │
│ │       │                                ╭────────────╯                    │ │
│ │ ¥500K ┤                     ╭──────────╯                                 │ │
│ │       │          ╭──────────╯                                            │ │
│ │ ¥490K ┤     ╭────╯                                                       │ │
│ │       │─────╯                                                            │ │
│ │ ¥480K ┤                                                                  │ │
│ │       └────────────────────────────────────────────────────────────      │ │
│ │        1      5      10      15      20      25      30                  │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│ Period Summary                                                               │
│                                                                              │
│ Net Worth Change          +¥18,132                                           │
│ Net External Flows        +¥16,800                                           │
│ Price Change               +¥4,830                                           │
│ FX Impact                  -¥3,797                                           │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

Metric selector:

```text
Assets
  Net Worth
  Total Assets
  Total Liabilities

Cash Flow
  Net External Flows
  Income
  Spending
  Transfers

Performance
  Investment Return
  Dividend & Interest
  FX Impact
  Fees
```

Only one primary metric is plotted by default.

---

# 15. Asset Changes — Categories

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ Asset Changes                                                                │
│                                                                              │
│ [ Change Drivers ]   [ Asset Trend ]   [ Categories ]                        │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│ Category Type                                                                │
│ [ Income ] [ Spending ] [ Investment Return ] [ Dividend & Interest ] [ Fees ]│
│                                                                              │
│ SPENDING  (grouped by account)                                               │
│                                                                              │
│ Total                                                                        │
│ ¥8,230                                                                       │
│                                                                              │
│ ┌────────────────────────────┐    Account        Amount        Share          │
│ │                            │                                              │
│ │       compact ring         │    CMB            ¥3,420        41.6%         │
│ │          chart             │    DBS            ¥2,800        34.0%         │
│ │                            │    MooMoo SG      ¥1,200        14.6%         │
│ └────────────────────────────┘    Alipay           ¥510         6.2%         │
│                                  Other            ¥300         3.6%         │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

The ranked category list is the primary analytical element.

The chart is optional and secondary.

v1 groups income / spending / fees by account, and return / Dividend & Interest by instrument or asset class. No budget category tree.

---

# 16. Category Detail Sheet

Example: click `China Merchants Bank`.

```text
                                                       ┌───────────────────────┐
                                                       │ China Merchants     × │
                                                       │ Bank                  │
                                                       │                       │
                                                       │ TOTAL                 │
                                                       │ ¥3,420                │
                                                       │                       │
                                                       │ Records               │
                                                       │                       │
                                                       │ Sep 03                │
                                                       │ Ole Supermarket -¥320 │
                                                       │                       │
                                                       │ Sep 05                │
                                                       │ Restaurant      -¥260 │
                                                       │                       │
                                                       │ Sep 08                │
                                                       │ Pharmacy        -¥410 │
                                                       │                       │
                                                       │ [ View in History ]   │
                                                       └───────────────────────┘
```

Final activity-level drill-down should reuse History.

---

# 17. More Filters — Suggested Popover

Both pages may use the same general interaction pattern.

```text
┌──────────────────────────────────────────────┐
│ More Filters                                 │
│                                              │
│ Account                                      │
│ [ All Accounts ▼ ]                           │
│                                              │
│ Currency                                     │
│ [ All Currencies ▼ ]                         │
│                                              │
│ Asset Class                                  │
│ [ All Asset Classes ▼ ]                      │
│                                              │
│ Instrument                                   │
│ [ All Instruments ▼ ]                        │
│                                              │
│ Member / Owner                               │
│ [ All Members ▼ ]                            │
│                                              │
│                       [ Clear ] [ Apply ]     │
└──────────────────────────────────────────────┘
```

The default page should not expose all these fields simultaneously.

---

# 18. Native Currency Scope Example

Native is allowed only when every valued component the projection reads shares one currency. Otherwise the API returns base with `valuationForced: "base"`. Do not invent multi-currency native totals, and do not split the page into per-currency sections.

When native cannot be honoured:

```text
Valuation
[ Native Currency ]     selected
Forced to Household Base (CNY)
This view mixes currencies
```

The native option stays selected but is shown as overridden. Switching tabs must not mutate the stored `valuation` choice.

When native is allowed (USD-only investment universe):

```text
USD Investment Return

+$2,840
+3.12%

FX Impact
Excluded from this view
```

When the user chooses Household Base Currency (CNY):

```text
USD Asset Contribution to Household

+¥24,310

Underlying Asset Return       +¥18,920
FX Impact                       +¥5,390
```

This distinction should be explicit in the summary, not hidden in a tooltip.

---

# 19. Account Scope Example

Example scope:

```text
Scope
MooMoo SG
```

The page adapts terminology.

Return Analysis:

```text
MooMoo SG Return
```

Asset Changes:

```text
Account Value Change

Beginning          Ending
¥120,000     →     ¥138,000
```

A transfer from DBS into MooMoo is:

```text
External Flows
```

even though it is an internal transfer at household scope.

---

# 20. Data Quality Warning

The banner is not a dead end: `[ View Details ]` opens the incomplete-data list below (which day, which component, which missing input: quote, FX, or unpriced quantity change).

```text
┌──────────────────────────────────────────────────────────────────────┐
│ ⚠ Some values are incomplete for this period.                       │
│   Missing market prices or FX rates may affect the result.           │
│                                                [ View Details ]       │
└──────────────────────────────────────────────────────────────────────┘
```

Suggested detail:

```text
Incomplete Data

Sep 12
NVDA closing price unavailable

Sep 18
USD/CNY FX rate unavailable

Sep 21
QQQ unpriced quantity change

Legacy activity
2 records cannot be fully attributed
```

Do not present missing values as precise zeroes.

---

# 21. Empty States

## Return Analysis — no investments

```text
┌────────────────────────────────────────────────────────────┐
│ No investment assets are available for this scope.         │
│                                                            │
│ Add an investment position or choose another scope.        │
└────────────────────────────────────────────────────────────┘
```

## No sufficient history

```text
┌────────────────────────────────────────────────────────────┐
│ Not enough historical data to calculate returns.           │
│                                                            │
│ Try a more recent period.                                  │
└────────────────────────────────────────────────────────────┘
```

## Categories — no spending

```text
No spending was recorded in this period.
```

Do not render empty charts.

---

# 22. Loading States

Preferred skeleton behavior:

```text
Return summary
██████████
██████

Calendar
┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐
│    │ │    │ │    │ │    │ │    │ │    │ │    │
└────┘ └────┘ └────┘ └────┘ └────┘ └────┘ └────┘
...
```

Keep:

- title,
- tabs,
- controls,
- page structure

visible while content reloads.

Do not replace the entire page with a full-screen spinner.

---

# 23. Narrow Desktop Layout

At narrower desktop widths:

```text
Scope            Return Basis
[ Household ]    [ Investment ]

Valuation        Cash
[ Base CNY ]     [ Include ]

[ More Filters ]
```

Annual calendar may shift:

```text
4 columns -> 3 columns -> 2 columns
```

The monthly calendar remains seven columns as long as usable.

The side sheet remains an overlay rather than becoming a new page.

---

# 24. Cross-Page Navigation Map

```text
Return Analysis
│
├─ Return Calendar
│   ├─ Year
│   │   └─ Month
│   │       └─ Day Detail Sheet
│   │              │
│   │              └──────────────► Asset Changes / Change Drivers   (v1)
│   │
│   ├─ Day sheet “View Full Return Details”
│   │          └───────────────────────► Contribution                 (later)
│   │
│   └─ Top Contributors This Month
│          └───────────────────────► Contribution                     (later)
│
├─ Return Trend
│
└─ Contribution
    └─ Contributor Detail
            └──────────────────────► History


Asset Changes
│
├─ Change Drivers
│   └─ Driver Detail Sheet
│          └───────────────────────► Return Analysis / Contribution   (v1)
│
├─ Asset Trend
│
└─ Categories
    └─ Category Detail
           └───────────────────────► History
```

The analytical hierarchy should consistently be:

```text
Overview
   ↓
Attribution
   ↓
Dimension breakdown
   ↓
Specific item
   ↓
History
```

---

# 25. Final Desktop Layout — Return Analysis

Full-page conceptual wireframe:

```text
┌──────────────┬───────────────────────────────────────────────────────────────┐
│              │ Return Analysis                                               │
│ Nestworth    │ Understand investment performance and contributors.           │
│              │                                                               │
│ Overview     │ [ Return Calendar ] [ Return Trend ] [ Contribution ]          │
│ Accounts     │                                                               │
│ Portfolio    │ Scope        Return Basis    Valuation       Cash              │
│ Instruments  │ [Household]  [Investment]    [Base CNY]      [Include]         │
│              │                                      [More Filters] [Reset]   │
│ Activity     │                                                               │
│   History    │ September 2026                          [‹] [Today] [›]         │
│              │                                        [Month | Year]         │
│ Insights     │                                                               │
│ > Return     │ ┌───────────────────────────────────────────────────────────┐ │
│   Analysis   │ │ Total Return                                              │ │
│   Asset      │ │ +¥12,430.28     +2.51% ◇                                │ │
│   Changes    │ │ Price +¥9,820 | Div & Int +¥620 | FX +¥2,110 | Fees     │ │
│              │ └───────────────────────────────────────────────────────────┘ │
│ Management   │                                                               │
│   Directory  │ ┌───────────────────────────────────────────────────────────┐ │
│              │ │ Mon Tue Wed Thu Fri Sat Sun                               │ │
│ Settings     │ │     1   2   3   4   5   6                               │ │
│   Market     │ │    +   -   +   +   +   +                                │ │
│   Data       │ │                                                           │ │
│   Settings   │ │ ...                                                       │ │
│              │ └───────────────────────────────────────────────────────────┘ │
│              │                                                               │
│              │ Top Contributors                                              │
│              │ QQQ +¥4,210    NVDA +¥3,180    USD FX +¥2,110               │
│              │                                                               │
└──────────────┴───────────────────────────────────────────────────────────────┘
```

---

# 26. Final Desktop Layout — Asset Changes

```text
┌──────────────┬───────────────────────────────────────────────────────────────┐
│              │ Asset Changes                                                 │
│ Nestworth    │ Understand why assets and net worth changed.                  │
│              │                                                               │
│ Overview     │ [ Change Drivers ] [ Asset Trend ] [ Categories ]             │
│ Accounts     │                                                               │
│ Portfolio    │ Period        Scope         Valuation        Cash             │
│ Instruments  │ [This Month]  [Household]   [Base CNY]       [Include]        │
│              │                                      [More Filters]           │
│ Activity     │                                                               │
│   History    │ ┌───────────────────────────────────────────────────────────┐ │
│              │ │ Net Worth Change                                         │ │
│ Insights     │ │ ¥476,684  →  ¥494,817                                   │ │
│   Return     │ │ +¥18,132   +3.80%                                       │ │
│   Analysis   │ └───────────────────────────────────────────────────────────┘ │
│ > Asset      │                                                               │
│   Changes    │ Change Drivers                                                │
│              │                                                               │
│ Management   │ ┌───────────────────────────────────────────────────────────┐ │
│   Directory  │ │ Beg → ExtFlow → Income → Spend → Price → FX → Resid → End │ │
│ Settings     │ │                  WATERFALL CHART                          │ │
│   Market     │ └───────────────────────────────────────────────────────────┘ │
│   Data       │                                                               │
│   Settings   │ Cash Flow                                                     │
│              │ External Flows   +¥20,000                                    │
│              │ Income            +¥5,000                                    │
│              │ Spending          -¥8,200                                    │
│              │                                                               │
│              │ Market & Investment                                           │
│              │ Price Change       +¥4,830                                    │
│              │ Dividend & Interest  +¥620                                    │
│              │ FX                 -¥3,797                                    │
│              │ Fees                 -¥320                                    │
│              │                                                               │
│              │ Other                                                         │
│              │ Unexplained difference +¥37                                   │
│              │                                                               │
└──────────────┴───────────────────────────────────────────────────────────────┘
```

---

# 27. Wireframe Design Rules

Use these rules during implementation:

1. `Return Analysis` and `Asset Changes` are separate sidebar pages under `Insights`.
2. Do not reintroduce an `Analysis` landing page.
3. Keep a maximum of three secondary tabs per page in the initial release.
4. Keep shared filters at the top of the page.
5. Preserve filter state across secondary tabs.
6. Prefer one primary chart per analytical task.
7. Prefer ranked tables for contribution/category analysis.
8. Use right-side sheets for local drill-down.
9. Use page navigation for hierarchy changes such as Year -> Month.
10. Reuse History for final record-level inspection.
11. Keep cash flow and investment return visually distinct.
12. Treat FX impact as a first-class attribution dimension.
13. Never classify internal household transfers as return.
14. Never silently treat missing data as zero.
15. Use restrained positive/negative color semantics.
16. Avoid an endless vertical dashboard of unrelated cards.
