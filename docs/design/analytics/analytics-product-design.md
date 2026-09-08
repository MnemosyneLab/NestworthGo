# Nestworth Analytics Redesign — Product & Interaction Design

**Status:** Current product design  
**Product:** Nestworth  
**Target:** Desktop application (Wails v3)  
**Primary navigation area:** Insights  
**Pages:** `Return Analysis`, `Asset Changes`  
**Companions:** [Technical architecture](analytics-redesign-architecture.md), [Wireframes](analytics-wireframes.md)

---

## 1. Executive Summary

The existing **Analysis** page mixes several different analytical jobs into one long dashboard: net-worth trend, realized gain, dividend income, time-range controls, and breakdowns by account or instrument. This makes the page difficult to understand, difficult to extend, and weak as an analytical workflow.

The redesign removes the single **Analysis** navigation item entirely.

Under **Insights** in the left sidebar, Nestworth exposes two first-class pages:

- **Return Analysis** — answers: **“How much did my investments earn, and what contributed to the result?”**
- **Asset Changes** — answers: **“Why did my assets or net worth increase or decrease over a period?”**

These are intentionally separate because they represent different accounting and analytical concepts:

- **Asset change** is not the same as **investment return**.
- **Investment return** is not the same as **cash flow**.
- **FX impact** must be separately attributable where relevant.
- Internal transfers must not be mistaken for profit or loss.

The two pages share a common scope/filter model and support drill-down into daily details, instruments, accounts, currencies, categories, and ultimately the existing History records.

---

## 2. Product Goals

### 2.1 Primary goals

1. Make performance analysis understandable without requiring accounting knowledge.
2. Clearly separate:
   - investment performance,
   - External Flows,
   - income and spending,
   - FX impact,
   - fees,
   - transfers,
   - Dividend & Interest.
3. Provide useful daily, monthly, yearly, account-level, currency-level, and instrument-level analysis.
4. Make the default experience useful for a household portfolio, not only a brokerage account.
5. Allow users to move from a high-level number to the underlying reason and records with predictable drill-down.
6. Reuse the same analytical model across accounts, instruments, currencies, and asset classes.
7. Avoid creating one-off charts for every analytical dimension.

### 2.2 Non-goals

This redesign is **not** intended to become:

- a full personal budgeting system,
- a tax reporting engine,
- a professional portfolio risk terminal,
- a double-entry accounting UI,
- a trade execution interface,
- a replacement for History.

History remains the source for inspecting individual recorded activities.

---

## 3. Navigation & Information Architecture

### 3.1 Left sidebar

Remove the current **Analysis** navigation item.

Under the existing **Insights** group, add:

```text
Insights
  Return Analysis
  Asset Changes
```

Recommended English labels:

- `Return Analysis`
- `Asset Changes`

Recommended Chinese labels:

- `收益分析`
- `资产变动`

No additional top-level “Analytics” or “Analysis” landing page is required.

### 3.2 Page-level navigation

Each page has its own secondary tabs.

**Return Analysis**

```text
Return Calendar | Return Trend | Contribution
```

**Asset Changes**

```text
Change Drivers | Asset Trend | Categories
```

These secondary tabs should remain inside the content area, below the page title.

---

## 4. Core Analytical Concepts

The UX should consistently communicate the following model.

### 4.1 Asset change

Asset change answers:

> How did the value of this scope change between the beginning and end of the period?

Conceptually:

```text
Ending Value
- Beginning Value
= Total Asset Change
```

The total change may be caused by:

- External Flows,
- Income,
- Spending,
- Price Change,
- Dividend & Interest,
- FX Impact,
- Fees,
- Liability Impact,
- Adjustments,
- Unexplained difference,
- other supported activity types.

### 4.2 Investment return

Investment return answers:

> How much value was created or lost by investment performance, excluding ordinary external funding flows?

Typical components:

```text
Investment Return
= Price Change
+ Dividend & Interest
+ Optional FX Impact
+ Investment-related Fees   // already signed, e.g. −20; do not subtract again
```

Percentage return is **daily Modified Dietz linked geometrically**. It is not `amount / beginning` and not XIRR.

Interest credited on cash (deposit, savings, bond coupon) is **return**, not Income. Both 股息收入 and 利息收入 map to the Dividend & Interest bucket. Salary, gifts, and refunds remain Income.

The exact inclusion rules should follow the selected analysis basis.

### 4.3 Cash flow

Cash flow is money entering or leaving the selected analytical scope.

Examples:

- salary paid into the household: Income (not External Flows),
- household purchase: Spending,
- interest credited on cash: Dividend & Interest (not Income),
- transfer from DBS to MooMoo: internal transfer at household scope,
- transfer into MooMoo from outside the selected account: External Flows at account scope.

Therefore the classification depends on **scope**.

### 4.4 Internal transfers

Internal transfers must not be treated as return.

At household scope:

```text
DBS -> MooMoo SG
Household net-worth change: 0
Investment return: 0
Internal transfer: yes
```

At the MooMoo account scope:

```text
Account asset change: +amount
Investment return: 0
External Flows: +amount
```

### 4.5 FX impact

For foreign-currency holdings, Nestworth must support two analytical views:

**Local / original currency basis**

> “Did the asset itself gain or lose value in its own currency?”

FX movement is excluded.

**Household base-currency basis**

> “What was the actual impact on household wealth after conversion to the household base currency?”

FX movement is included and may be shown separately.

Example:

```text
USD asset return in USD:               0
USD/CNY FX impact in household CNY: +2,000
Household-base-currency contribution: +2,000
```

---

## 5. Shared Analysis Controls

Both pages should use a common analytical scope model where applicable.

Avoid scattering unrelated filter controls throughout cards.

### 5.1 Scope

Primary scope selector:

```text
Scope
[ Entire Household ▼ ]
```

Available scope types:

```text
Entire Household

Accounts
  MooMoo SG
  DBS
  China Merchants Bank
  ...

Currencies
  CNY
  USD
  SGD
  ...

Instruments
  QQQ
  NVDA
  VOO
  ...

Asset Classes
  Cash
  Stocks
  ETFs
  Funds
  Bonds
  Crypto
  Gold
  Property
  ...
```

Multi-select may be added later, but the initial implementation can use one logical scope at a time.

### 5.2 Currency / valuation basis

When relevant:

```text
Valuation
[ Household Base Currency (CNY) ▼ ]
```

Options:

```text
Household Base Currency
Original / Native Currency
```

If the selected scope spans multiple currencies, native-currency aggregation is not meaningful as a single number. Native is then **forced to base** with a visible reason. The native option stays selected but is shown as overridden; the stored valuation choice is not silently flipped. Do not split the page into per-currency sections.

### 5.3 Cash inclusion

For return analysis:

```text
Cash
[ Include ▼ ]
```

Options:

```text
Include Cash
Exclude Cash
```

This control affects the selected investment universe but should not alter unrelated household cash-flow classification.

### 5.4 More filters

Advanced filters should be collapsed under:

```text
[ More Filters ]
```

Possible filters:

- account,
- asset class,
- currency,
- instrument,
- member / owner.

V1 does not include tags, an “investment vs non-investment” filter, or an “include liabilities” toggle. Scope, these filters, and Cash inclusion are the universe controls.

The default view should not expose all advanced controls.

### 5.5 Filter persistence

Each page should remember its most recent filter state during the current application session.

Switching secondary tabs should preserve filters.

Switching between `Return Analysis` and `Asset Changes` may preserve compatible scope and date settings where possible.

---

# 6. Page 1 — Return Analysis

## 6.1 Purpose

Return Analysis answers:

> How much did my selected portfolio earn or lose, and where did that result come from?

It should support three analytical modes:

```text
Return Calendar
Return Trend
Contribution
```

Default tab: **Return Calendar**

---

## 6.2 Return Analysis — Page Header

Recommended structure:

```text
Return Analysis

Understand investment performance and the assets that contributed to it.

[ Return Calendar ] [ Return Trend ] [ Contribution ]
```

Below the tabs:

```text
Scope              Return Basis          Valuation              Cash
[ Household ▼ ]    [ Investment ▼ ]      [ Base CNY ▼ ]         [ Include ▼ ]

                                              [ More Filters ] [ Reset ]
```

### Return basis

Return Basis is a typed query field fixed to `investment` in v1. The UI **may hide this control entirely**.

If shown, the only value is:

```text
Investment Return
```

Realized Gain, Unrealized Gain, and Dividend & Interest are Contribution **views**, not global return-basis modes. They are independent of Total Return and are not a closed decomposition of it.

---

# 7. Return Calendar

## 7.1 Monthly view

Monthly view is the default experience.

Header:

```text
September 2026                         [ ‹ ] [ Today ] [ › ]
                                      [ Month | Year ]
```

### 7.1.1 Monthly summary

```text
This Month

Total Return
+¥12,430.28       +2.51% ◇

Price Change       Dividend & Interest       FX Impact       Fees
+¥9,820            +¥620                     +¥2,110         -¥119.72

Beginning Invested Value                   Ending Invested Value
¥487,340                                  ¥503,120
```

The summary should prioritize:

1. total amount,
2. percentage return (daily Modified Dietz, geometrically linked; never `amount / beginning`, never XIRR),
3. attribution.

When the period % covers fewer days than the amount (`ratedDays` < `totalDays`), the **%** carries a marker (shown as `◇` above) and its tooltip states the covered day count. The amount is never marked by rate coverage alone.

Do not overload the card with too many portfolio statistics.

### 7.1.2 Calendar layout

Desktop:

```text
Mon        Tue        Wed        Thu        Fri        Sat        Sun

           1          2          3          4          5          6
         +0.31%     -0.42%     +0.65%     +0.12%     +0.84%     +0.09%
         +1,240     -1,680     +3,241       +620     +4,120       +430

7          8          9         10         11         12         13
-0.12%    +0.81%     +0.17%     ...
 -580     +3,940       +820

...
```

Each day cell contains:

- day number,
- return percentage,
- return amount.

Optional future additions:

- tiny dot for dividend event,
- tiny dot for fee event,
- incomplete-data indicator.

### 7.1.3 Day-cell color treatment

Use low-saturation semantic backgrounds:

- positive: subtle green tint,
- negative: subtle red tint,
- near zero: neutral,
- unavailable/incomplete: muted gray / hatch / status indicator,
- today (not a closed snapshot): muted, with an explanatory state — not a zero day, and not silently shifted to yesterday.

Do not use highly saturated trading-terminal colors.

### 7.1.4 Hover

Hover shows a compact tooltip:

```text
Sep 3

Daily Return       +¥3,241
Daily Return %     +0.65%
Price Change       +¥2,581
FX Impact            +¥640
Dividend & Interest   +¥20
Fees                   ¥0
```

Hover composition is returned inline with the calendar cells (Price Change, FX Impact, Dividend & Interest, Fees). It does not require a per-cell round trip.

### 7.1.5 Click

Click opens a right-side detail sheet.

Do not navigate away.

### 7.1.6 Top Contributors This Month

Below the calendar, show a compact ranked list of the largest contributors for the visible month (from the calendar response). This is an orientation aid, not a second Contribution page.

```text
Top Contributors This Month

QQQ                 +¥4,210        NVDA                +¥3,180
USD FX              +¥2,110        SOXQ                -¥1,320
```

V1 does not navigate from this list to Contribution. Drill-down from a calendar **day** goes to Change Drivers.

---

## 7.2 Daily Detail Sheet

Example:

```text
September 3, 2026                                  ×

Daily Return
+¥3,241.18                         +0.65%

Return Composition

Price Change                              +¥2,581
FX Impact                               +¥640
Dividend & Interest                      +¥20
Fees                                      ¥0

Top Contributors

NVDA                                  +¥1,820
QQQ                                     +¥940
VOO                                     +¥351
SOXQ                                    -¥510

[ View Full Return Details ]

[ View Asset Changes for This Day → ]
```

### Interaction rules

- `View Asset Changes for This Day` navigates to Asset Changes → Change Drivers with the same date and scope. This is a v1 cross-page link.
- `View Full Return Details` is later (not v1). It must not be documented as navigating to Contribution.
- Clicking a contributor does not navigate to Contribution in v1.
- The sheet should be closable with:
  - close icon,
  - Escape,
  - click outside, where platform conventions allow.

---

# 8. Return Calendar — Year View

Year view presents 12 months as a 3 × 4 grid on a wide desktop layout.

Header:

```text
2026                                     [ ‹ ] [ This Year ] [ › ]
                                         [ Month | Year ]
```

Summary:

```text
Annual Return

+¥46,280.16       +9.34% ◇

Price Change       Dividend & Interest       FX Impact       Fees
+¥38,210           +¥3,820                    +¥5,040         -¥789.84
```

Month cards:

```text
┌────────────────┐
│ January        │
│                │
│ +2.10%         │
│ +¥8,320        │
│                │
└────────────────┘
```

States:

- positive,
- negative,
- near zero,
- future,
- no data,
- partial data.

### Click behavior

Clicking a month navigates to that month’s **Month View**.

Do not use a sheet for month selection.

Hierarchy:

```text
Year -> Month -> Day Detail
```

---

# 9. Return Trend

## 9.1 Purpose

Return Trend answers:

> How has investment performance evolved over time?

### Controls

```text
Range
[ 30D ] [ YTD ] [ 1Y ] [ 3Y ] [ All ] [ Custom ]

Display
[ Cumulative Return | Return % | Period Return ]
```

### Primary chart

Default:

```text
Cumulative Return
```

A single line chart is preferred.

Do not simultaneously draw:

- assets,
- liabilities,
- net worth,
- returns,
- dividends,

on one graph.

### Supporting summary

Below or above the chart:

```text
Period Return         +¥46,280
Return %              +9.34% ◇
Dividend & Interest    +¥3,820
FX Impact              +¥5,040
Fees                     -¥790
```

Return % is the geometrically linked daily Modified Dietz rate for the range. Mark it when `ratedDays` < `totalDays`.

### Return-source breakdown

Use compact rows rather than another large chart:

```text
Price Change                      +¥38,210       82.6%
Dividend & Interest                +¥3,820        8.3%
FX Impact                          +¥5,040       10.9%
Fees                                 -¥790       -1.7%
```

### Hover

Chart hover should show:

```text
Aug 12, 2026

Cumulative Return     +¥32,840
Period Return            +¥920
Return %                 +0.19%
```

---

# 10. Contribution

## 10.1 Purpose

Contribution answers:

> Which instruments, accounts, currencies, or asset classes produced the result?

### Controls

```text
Return Type
[ Total Return ] [ Realized Gain ] [ Unrealized Gain ] [ Dividend & Interest ]

Group By
[ Instrument ▼ ]

Sort
[ Contribution ▼ ]
```

If range-end cost basis is unavailable, the Unrealized Gain view is omitted
from the v1 control rather than rendered as an unavailable error page. It must
not be approximated as `Total − Realized − Dividend`.

These four return types are **independent views**. They are not a partition of Total Return and are not expected to sum to it.

- **Total Return:** period investment return (Dietz amount).
- **Realized Gain:** average-cost sell gains in range. Not “Return”.
- **Unrealized Gain:** market value minus remaining cost basis **at range end**. Never `Total − Realized − Dividend`.
- **Dividend & Interest:** cash dividends and interest (`interest` reason) in range. Interest is not lumped into Income.

### Group By options

```text
Instrument
Account
Currency
Asset Class
```

Potential later additions:

- owner / family member,
- region,
- custom group.

Tags are omitted in v1.

### Primary table / bar list

Prefer a ranked analytical table with inline contribution bars.

**Total Return** (Dietz % on each group as its own investment universe — never `row amount / opening`):

```text
Instrument                     Contribution       Return %

Defiance Quantum ETF            +¥14,056            +8.2%
Vanguard S&P 500 ETF            +¥10,005            +4.3%
NVDA                             +¥7,820           +12.4%
QQQ                              +¥5,940            +6.1%
USD Cash                         +¥1,810            +1.2%
SOXQ                             -¥3,521            -3.8%
```

**Unrealized Gain** may show a % only when remaining cost is available and positive (`unrealized / remaining cost`). Otherwise omit the column.

**Realized Gain** and **Dividend & Interest** have **no % column** in v1 — do not leave a blank column:

```text
Instrument                     Contribution

Defiance Quantum ETF            +¥14,056
Vanguard S&P 500 ETF            +¥10,005
NVDA                             +¥7,820
QQQ                              +¥5,940
```

Use green/red only as a semantic accent.

### Why a table-first design

A table is preferable to a large bar chart because it:

- handles many instruments better,
- supports exact values,
- scales to sorting,
- can later add secondary columns,
- is more useful for serious portfolio analysis.

### Row click

Click opens a detail sheet for the **active return type only**. Do not stack Realized Gain, Unrealized Gain, and Price / Dividend / FX / Fees as if they partitioned Total Return.

For **Total Return**, the sheet shows that view’s composition:

- selected item,
- Total Return amount and Dietz %,
- Price Change, Dividend & Interest, FX Impact, Fees (these sum to this view’s Total Return, not to Realized + Unrealized),
- account allocation,
- link to relevant History.

A short line of copy: these four return types are independent views and are not expected to sum to Total Return.

Switching Return Type changes both the table and the sheet. Realized Gain and Dividend & Interest sheets show amount only (no %). Unrealized Gain shows the range-end cost-basis figure, and % only when remaining cost allows.

---

# 11. Page 2 — Asset Changes

## 11.1 Purpose

Asset Changes answers:

> Why did the selected household, account, currency, or asset scope become more or less valuable?

Secondary tabs:

```text
Change Drivers
Asset Trend
Categories
```

Default tab: **Change Drivers**

---

# 12. Asset Changes — Page Header

```text
Asset Changes

Understand why assets and net worth increased or decreased.

[ Change Drivers ] [ Asset Trend ] [ Categories ]
```

The session store is shared with Return Analysis, so the header uses the same filter set:

```text
Period
[ This Month ▼ ]    Sep 1, 2026 — Sep 30, 2026

Scope              Valuation              Cash
[ Household ▼ ]    [ Base CNY ▼ ]         [ Include ▼ ]

                                              [ More Filters ]
```

Preset period options:

```text
Today
This Week
This Month
Last Month
YTD
This Year
Custom
```

`Today` is not a closed snapshot. The preset selects the range but the current day renders muted with an explanatory state — not a zero day, and not silently shifted to yesterday.

---

# 13. Change Drivers

## 13.1 Summary

Top summary card:

```text
Net Worth Change

Beginning                          Ending
¥476,684.62          →             ¥494,817.08

Change
+¥18,132.46
+3.80%
```

If the selected scope is not the whole household, the label should adapt:

- `Account Value Change`
- `USD Asset Value Change`
- `Investment Asset Change`
- etc.

---

## 13.2 Waterfall Chart

The core visual is a waterfall chart.

Conceptual example:

```text
Beginning
¥476,684

+ External Flows
+ Income
+ Spending              // already signed, e.g. −800
+ Dividend & Interest
+ Price Change
+ FX Impact             // already signed
+ Fees                  // already signed, e.g. −20
+ Liability Impact      // when applicable
+ Adjustments
+ Unexplained difference  // only when above tolerance; visually distinct from Adjustments

Ending
¥494,817
```

Labels may still read “Spending” or “Fees”. Chart and engine **sum signed contributions**; they must not apply a second minus. Calculation contract: [architecture §4](analytics-redesign-architecture.md).

Recommended categories:

1. Beginning value
2. External Flows
3. Income
4. Spending
5. Dividend & Interest
6. Price Change
7. FX Impact
8. Fees
9. Liability Impact, when applicable
10. Adjustments
11. Unexplained difference, when above tolerance
12. Ending value

The precise categories displayed should depend on the selected scope and available data.

### Important behavior

Zero-value categories may be hidden by default.

Unexplained difference is its own waterfall bar and its own row in the `other` group. It is visually distinct from Adjustments, present only when above tolerance, and must not be folded into Adjustments.

A `Show zero categories` option is unnecessary for the first version.

---

# 14. Change Attribution Table

Below the waterfall, show a structured attribution list.

Example:

```text
Cash Flow

External Flows                           +¥20,000
Income                                    +¥5,000
Spending                                  -¥8,200
                                         ────────
Net External Flows                       +¥16,800


Market & Investment

Price Change                              +¥4,830
Dividend & Interest                         +¥620
FX Impact                                 -¥3,797
Fees                                        -¥320
                                         ────────
Investment & Market Impact                +¥1,333


Other

Unexplained difference                       +¥37


Total Asset Change                       +¥18,170
```

This grouping is a critical part of the experience.

It lets the user immediately distinguish:

> “My assets increased because I saved money”

from:

> “My assets increased because my investments performed well.”

---

# 15. Change Driver Drill-Down

Click any change driver to open a right-side sheet.

Example:

```text
Price Change                                           ×

Sep 1 — Sep 30, 2026

Total
+¥4,830

By Instrument

QQQ                                  +¥2,340
NVDA                                 +¥1,820
VOO                                    +¥810
SOXQ                                   -¥140

By Account

MooMoo SG                            +¥4,690
IBKR                                   +¥140

[ Open Return Analysis → ]
```

Unexplained difference uses the same sheet pattern, with a visually distinct treatment from Adjustments:

```text
Unexplained difference                                 ×

Sep 1 — Sep 30, 2026

Total
+¥37

Component / day
QQQ · Sep 12     closing quantity change unpriced

[ View in History ]
```

### Cross-page linking

`Open Return Analysis` (from a Price Change / return driver, not from Unexplained difference) navigates to the Contribution tab with:

- same date range,
- same scope,
- relevant return type,
- optional contributor filter.

This creates a coherent analytical workflow instead of isolated reports.

---

# 16. Asset Changes — Asset Trend

## 16.1 Purpose

Asset Trend answers:

> How has this metric evolved by day, week, or month?

### Controls

```text
Granularity
[ Day ] [ Week ] [ Month ]

Metric
[ Net Worth ▼ ]
```

### Metric selector

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

### Chart behavior

Use one primary metric at a time.

Do not recreate the existing multi-line “assets / liabilities / net worth” chart as the default.

If comparison is later required, provide an explicit `Compare` interaction rather than always showing multiple lines.

### Summary

For example:

```text
Period Change             +¥18,132
Net External Flows        +¥16,800
Price Change               +¥4,830
FX Impact                  -¥3,797
```

---

# 17. Asset Changes — Categories

## 17.1 Purpose

Categories answers:

> What types of income, spending, fees, or returns explain the selected total?

Top control:

```text
Category Type

[ Income ] [ Spending ] [ Investment Return ] [ Dividend & Interest ] [ Fees ]
```

v1 groups **income, spending, and fees by account**, and **return / Dividend & Interest by instrument or asset class**. There is no budget-style category tree.

### Example: Spending (by account)

```text
Spending

Total
¥8,230
```

Then:

```text
Account                   Amount        Share

China Merchants Bank      ¥3,420        41.6%
DBS                       ¥2,800        34.0%
MooMoo SG                 ¥1,200        14.6%
Alipay                      ¥510         6.2%
Other                       ¥300         3.6%
```

A compact donut / ring chart may be used as a secondary visualization, but the ranked list is the primary analytical component.

Avoid relying on charts alone.

---

# 18. Category Drill-Down

Click a row (an account for spending/income/fees, or an instrument / asset class for return and Dividend & Interest):

```text
China Merchants Bank                                ×

Total
¥3,420

Records

Sep 03    Ole Supermarket             -¥320
Sep 05    Restaurant                  -¥260
Sep 08    Pharmacy                    -¥410
...

[ View in History ]
```

Do not invent budget subcategories (Household / Travel / Dining / Groceries).

The final drill-down should reuse **History** rather than implementing a second activity browser.

---

# 19. Cross-Page Interaction Model

The two pages should feel like two perspectives on the same dataset.

## 19.1 Return Analysis -> Asset Changes

Examples:

- click a day in Return Calendar,
- open daily sheet,
- click `View Asset Changes for This Day`.

Target:

```text
Asset Changes
Date = selected day
Scope = preserved
Tab = Change Drivers
```

## 19.2 Asset Changes -> Return Analysis

Examples:

- click `Price Change`,
- inspect instrument breakdown,
- click `Open Return Analysis`.

Target:

```text
Return Analysis
Date range = preserved
Scope = preserved
Tab = Contribution
```

## 19.3 Analysis -> History

Any final transaction/activity-level drill-down should lead to History with compatible filters pre-applied where possible.

---

# 20. Data Completeness & Trust

Financial analytics are useful only if users trust the numbers.

Nestworth should explicitly communicate incomplete analytical periods.

Possible causes:

- missing daily quote,
- missing FX rate,
- asset without price data,
- legacy activity without sufficient attribution metadata,
- manually entered values,
- unsupported instrument valuation.

### UI pattern

Use a subtle warning near the summary. The banner is openable — a banner with no way to see what is missing is not a trust affordance:

```text
⚠ Some values are estimated or incomplete for this period.
[ View details ]
```

`View details` opens the incomplete-data drawer (period `issues[]`): which day, which component, and which missing input (quote, FX, or an unpriced quantity change).

Do not silently show a precise percentage when the underlying dataset is incomplete.

When a period’s **%** covers fewer days than its **amount** (`ratedDays` < `totalDays`), the % carries a marker and its tooltip states the covered day count. The amount is never marked by rate coverage alone.

### Daily calendar state

A day may be:

- complete,
- partial,
- unavailable.

Partial days should be visually distinguishable from true zero-return days.

---

# 21. Empty States

## 21.1 No investment assets

Return Analysis:

```text
No investment assets are available for this scope.

Add an investment position or choose a different scope.
```

## 21.2 No historical data

```text
Not enough historical data to calculate returns for this period.

Try a more recent period.
```

## 21.3 No category activity

```text
No spending was recorded in this period.
```

Avoid showing empty charts with axes.

---

# 22. Loading States

Use skeletons for:

- summary cards,
- calendar grid,
- ranked contribution rows,
- waterfall chart,
- category rows.

Do not replace the entire page with a blocking spinner when switching tabs or filters.

Keep the existing page layout stable during reload.

---

# 23. Responsive Desktop Behavior

Nestworth is a desktop application, so optimize primarily for approximately:

```text
1280 px and wider
```

### Wide layout

- filter controls on one row,
- 7-column calendar,
- 4-column annual month grid,
- detail sheet on the right,
- charts use full content width.

### Narrow desktop layout

At smaller window widths:

- filter controls wrap,
- annual grid can become 3 × 4 or 2 × 6,
- detail sheet remains overlay,
- tables keep essential columns and hide secondary ones.

Avoid mobile-style full-screen navigation unless the application later targets mobile.

---

# 24. Visual Design Guidance

The current application already has a light, restrained financial-product aesthetic. The analytics redesign should preserve that direction but reduce large empty cards and excessive chart whitespace.

### Principles

- Prefer information density over oversized cards.
- Use cards only when they create meaningful grouping.
- Avoid putting every section inside a bordered card.
- Use typography hierarchy before borders.
- Keep positive/negative colors semantic and restrained.
- Use tabular numerals for financial values.
- Align currency values consistently.
- Keep percentages and amounts visually paired.
- Avoid decorative charts.

### Positive / negative semantics

Recommended:

```text
Positive return: green
Negative return: red
Neutral / flow / balance: standard text color
FX / informational categories: neutral or category color
```

Do not use green to mean “money entered the account” if that might be mistaken for investment profit.

---

# 25. Recommended Component Structure

Conceptual component tree:

```text
Insights
├── ReturnAnalysisPage
│   ├── PageHeader
│   ├── ReturnAnalysisFilters
│   ├── ReturnCalendarTab
│   │   ├── PeriodNavigator
│   │   ├── ReturnSummary
│   │   ├── MonthlyCalendar
│   │   ├── AnnualCalendar
│   │   └── DailyReturnSheet
│   ├── ReturnTrendTab
│   │   ├── RangeSelector
│   │   ├── ReturnTrendChart
│   │   └── ReturnSourceSummary
│   └── ContributionTab
│       ├── ContributionControls
│       ├── ContributionTable
│       └── ContributionDetailSheet
│
└── AssetChangesPage
    ├── PageHeader
    ├── AssetChangeFilters
    ├── ChangeDriversTab
    │   ├── AssetChangeSummary
    │   ├── WaterfallChart
    │   ├── AttributionTable
    │   └── DriverDetailSheet
    ├── AssetTrendTab
    │   ├── GranularitySelector
    │   ├── MetricSelector
    │   └── TrendChart
    └── CategoriesTab
        ├── CategoryTypeSelector
        ├── CategorySummary
        ├── CategoryTable
        └── CategoryDetailSheet
```

This is a product-level structure, not a required code architecture.

---

# 26. Recommended Default States

## Return Analysis

Default:

```text
Tab: Return Calendar
Scope: Entire Household
Valuation: Household Base Currency
Cash: Include
Calendar: Current Month
```

If the household has no investment assets, use the first valid investment scope.

## Asset Changes

Default:

```text
Tab: Change Drivers
Period: Current Month
Scope: Entire Household
Valuation: Household Base Currency
Cash: Include
```

---

# 27. Terminology

Canonical names (must match architecture §19):

| Concept | English UI | 中文 |
|---|---|---|
| Page | Return Analysis | 收益分析 |
| Page | Asset Changes | 资产变动 |
| Tab | Return Calendar | 收益日历 |
| Tab | Return Trend | 收益趋势 |
| Tab | Contribution | 标的贡献 |
| Tab | Change Drivers | 变化原因 |
| Tab | Asset Trend | 资产趋势 |
| Tab | Categories | 分类统计 |
| Bucket | External Flows | 外部资金流动 |
| Bucket | Income | 收入 |
| Bucket | Spending | 支出 |
| Bucket | Dividend & Interest | 股息与利息 |
| Bucket | Price Change | 价格变动 |
| Bucket | FX Impact | 汇率影响 |
| Bucket | Fees | 费用 |
| Bucket | Liability Impact | 负债影响 |
| Bucket | Adjustments | 手动调整 |
| Bucket | Unexplained difference | 未解释差额 |
| Contribution view | Total Return | 总收益 |
| Contribution view | Realized Gain | 已实现收益 |
| Contribution view | Unrealized Gain | 未实现收益 |

Notes:

- Return Trend is 收益趋势 and Asset Trend is 资产趋势. Neither is bare Trend / 时间趋势.
- Cost-basis views use **Gain**; Dietz views use **Return**. Do not mix Realized Return with Realized Gain.
- External Flows everywhere in UI. Not Inflow, not External Cash Flow.
- `P&L` never appears in UI copy. Use Return.
- 股息收入 and 利息收入 both map to Dividend & Interest. Interest is not Income.

---

# 28. Migration from the Existing Analysis Page

Existing sections should be redistributed rather than preserved as-is.

| Existing feature | New location |
|---|---|
| Wealth trend | Asset Changes -> Asset Trend |
| Period summary | Page-specific summary blocks |
| Realized gains | Return Analysis -> Contribution |
| Dividend income | Return Analysis -> Contribution / Asset Changes -> Categories |
| Account breakdown | Contribution -> Group By Account |
| Instrument breakdown | Contribution -> Group By Instrument |
| Time range selector | Local page / tab control |
| Asset / liability / net-worth graph | Asset Changes -> Asset Trend, metric selector |

The old `Analysis` route and navigation entry should be removed after the replacement pages are functional.

---

# 29. Product Acceptance Criteria

## Navigation

- The left sidebar no longer contains `Analysis`.
- `Return Analysis` and `Asset Changes` appear under `Insights`.
- Each page is directly navigable and can be deep-linked internally.

## Return Calendar

- User can switch month/year.
- User can navigate previous/next period.
- Monthly view shows daily amount and percentage.
- Annual view shows 12 monthly results.
- Clicking a month opens its monthly calendar.
- Clicking a day opens a right-side detail sheet.
- Daily detail can link to Asset Changes.

## Return filters

- Scope can be changed.
- Base-currency vs native-currency logic is supported where meaningful.
- Cash can be included/excluded.
- Compatible filters persist across secondary tabs.

## Return Trend

- User can choose period.
- User can view cumulative amount, percentage, or period return.
- Chart shows one primary metric by default.
- Return components are visible.

## Contribution

- User can switch total / realized / unrealized / dividend-interest views.
- User can group by instrument, account, currency, or asset class.
- Rows support drill-down.

## Asset Changes

- User can select time period and scope.
- Beginning, ending, amount change, and percentage change are displayed.
- Waterfall shows the major drivers of change.
- Cash flow and market/investment impact are visually separated.

## Asset Trend

- User can select day/week/month granularity.
- User can select one analytical metric.
- Existing wealth trend capability is preserved here.

## Categories

- User can inspect income, spending, investment return, dividend/interest, and fees.
- Category rows can drill down to activity records.
- Final activity-level inspection reuses History.

## Trust

- Partial or incomplete data is visibly indicated.
- Missing-data days are not shown as zero-return days.
- Internal transfers are not misclassified as investment return at household scope.

---

# 30. Final Product Direction

The redesign should not be treated as “a better analytics dashboard.”

It is an **analytics workspace with two distinct mental models**:

```text
Return Analysis
“How much did I earn, and what contributed to that return?”

Asset Changes
“Why did my wealth or account value change?”
```

The shared design pattern is:

```text
High-level result
    ↓
Attribution
    ↓
Dimension breakdown
    ↓
Specific item
    ↓
History records
```

This hierarchy should guide future analytics features as well.

New analytical capabilities should generally be added as:

- a new metric,
- a new grouping dimension,
- a new attribution category,
- or a new drill-down,

rather than as another standalone dashboard card.

That keeps Nestworth extensible while preserving a clear mental model for household wealth analysis.
