# Account composition: type and detail views

This feature is separate from Historical Panorama. Composite asset accounts
open **By type** (按类型 / 按類型); **By detail** shows the first ten original
components and a distinct “Remaining N items” entry. Show all expands the list
and Show top 10 collapses it. The ring retains ten entries plus the remainder.
The accessible data table always contains every original component exactly
once, including zero and unavailable values, and never includes the remainder.

Type mode shows every present canonical type, ordered by exact household
currency value, with stable key tie-breaking. Cash currencies form one cash
group. ETF and mutual fund remain separate. Managed term deposits and locked
products retain `bank_investment_product`, labeled **Investment products & term
deposits** / **理财及定期** / **理財及定期**. An archived instrument's retained
holding uses its stored type; missing or unrecognized metadata is **Unknown
type**, separate from explicit `other`. Names and symbols do not classify an
instrument. Product contract subtypes and liability pies are outside this change.

## Precision and data boundary

`AccountValuation.composition` is calculated by the backend from the same
`PortfolioSnapshot` and exact `ValuationComponent.BaseAmountExact` values as
account valuation. It includes currency, exact valued total, completeness,
status, valued/missing component counts, all types, original details, and the
precomputed detail slices. Each entry includes a namespaced stable key, type
key, detail references, exact household amount, display MoneyView, backend
share basis points, and valued/missing counts. Original details retain native
amount/currency. Type/remainder native amounts are absent because unlike
currencies must never be added together.

Amounts are summed before rounding. Shares use the same exact valued total;
backend largest-remainder allocation assigns the displayed basis points once
per original detail, breaking ties by stable detail key. The collapsed remainder
adds those assigned detail shares; unchanged details keep the same percentages
in the collapsed list, expanded list, source table, and tooltip. Types are a
different partition: their shares are allocated from exact aggregated amounts,
not rounded leaf shares. Each partition totals 10,000 BPS; type rounding can
differ from the sum of individually rounded detail percentages. Equal 30/30 or
600/600 type totals remain 50%/50%, even when leaf rounding sums to 50.1%/49.9%
or 52%/48%.
The ring uses the exact amount strings converted to numbers only for drawing.
Frontend code does not recompute financial totals or shares. In particular,
two `0.00005` components remain two 50% details and a `0.0001` type total even
when each component's MoneyView is `0`.

An unavailable exact amount is absent, while a real zero is `"0"`. All missing
values produce an unavailable state; all valued zeroes produce an empty state.
Partial valuation discloses missing components. Any negative component makes
the composition invalid; it is retained in the source table and no pie is drawn.
The reserved `account-composition:remainder` key cannot collide with real types
or original holding/currency keys. Generic chart collapsing remains the default
on Portfolio and Overview; the account panel explicitly opts into its backend
projection and full source table.

The grouping mode survives a valuation refresh and resets to type for a new
account. Refresh and mode changes clear chart hover, selection, and focus state.
Grouping and expansion are local presentation state and request no quotes.
The grouping selector uses native single-choice radios with one tab stop and
arrow-key navigation, matching existing account controls. Expanded original
rows retain their detail identities; tail hover/focus/selection highlights the
remainder wedge. Expanding/collapsing clears account-chart interactions while
leaving the source table open. Other chart consumers do not opt into this
interaction mapping. ECharts selection is synchronized with explicit unselect/
select actions: merging `data.selected: false` alone retains its internal
selected map and can leave a wedge pulled out. Deselect, collapse and reset
clear the actual pie selection, as well as the legend. English remainder labels
use singular/plural forms.

The shared EChart policy defaults emphasis item fills to `inherit`. zrender
6.1.0 cannot lift CSS OKLCH colors, which otherwise replaces highlighted pie
and bar fills with an undefined color. ECharts' inheritance path keeps the
normal/selected theme fill and alpha without converting colors or adding a
palette. Explicit emphasis colors, line/area styles, focus and scaling remain
intact. Real renderer tests reproduce the original disappearance and exercise
stock/remainder pies, bars and line markers across every shipped OKLCH token.

## Automated checks

Use only isolated synthetic test fixtures. The regression coverage includes
canonical types and cash currencies, ETF/fund separation, both managed product
kinds, archived and unknown metadata, exact sub-cent values, near-maximum and
tiny amounts, FX, 0/1/10/11/many details, more than six types, partial/all-missing/
zero/invalid values, wire serialization, source-table uniqueness, mode/refresh/
account state, keyboard operation, and unchanged generic chart collapsing.

Run Go tests and relevant race tests, frontend tests, typecheck, lint,
binding generation/check, and production frontend/server builds. In a headless
Linux environment without GTK/WebKit, `CGO_ENABLED=0` can run binding generation
with `GOFLAGS='-mod=readonly -tags=server'`; race tests require CGO enabled.
Generated bindings/dist follow the repository's artifact policy.

## Remaining native acceptance

The parent owns native macOS acceptance and independent Astra/Atlas review.
On the reviewed commit, check:

- Single-type 100% accounts and accounts with all nine canonical types.
- By detail at 10/11/many entries; expand, collapse, refresh, and account switch.
- Narrow window controls, long names, large monetary values, light/dark themes,
  and all three locales. Verify wrapping without clipping and readable amounts.
- Hover stock and the expanded managed-product tail, and hover wedges directly.
  Account and Portfolio pie fills must remain visible while emphasizing. Check
  Overview pies, historical bars, trend lines/markers and quote-history charts.
- Mouse hover/selection, keyboard focus, screen-reader grouping controls,
  expand/collapse state, and the full data table.
- Partial, all-missing and zero valid native fixtures; no misleading pie.
  Invalid-negative and unknown-metadata cases are unit-only under current valid
  database constraints, and do not block the native GUI matrix.
- Overview/Portfolio chart behavior and Historical Panorama views after the
  main update. No real household profile is needed.

No merge, release, tag, packaging, or publication is authorized by this task.

## Account total display currencies

The account list and detail header show the account's configured
`defaultCurrency` first, with household `baseCurrency` below in smaller type.
Identical currencies use one amount line. Account type, bank name, country and
the first holding never select the display currency. Holding rows continue to
show the asset's native currency. Known native values remain visible when only
household FX is missing; they are not mislabeled as missing prices.

`AccountValuation.displayValue` is an additional accounts-only backend
projection, attached by `ValueAccountCompositions` and `ValuationService.Account`.
It has its own currency, exact total, rounded MoneyView, completeness,
valued/missing counts, missing inputs and selected FX evidence. It includes all
cash and holding components, including managed products. Balance/manual-value
accounts also convert a latest recorded value whose currency differs from the
configured account currency. Missing native values never become zero. Known
zero and same-currency components need no FX; an empty holdings account is zero.
Partial account or household amounts are explicitly labeled “Valued subtotal”.
An unavailable account total keeps an account-currency unavailable label in the
primary position; an available household amount stays secondary.

Conversion uses persisted direct or inverse observations under the existing
pair source preference, configured provider and deterministic observation
ordering. If no eligible pair quote exists, and no explicit preference exists
for that pair, the only fallback is a two-leg bridge through household currency.
Each leg honors its own source preference and provider selection. An explicit
pair preference never falls back to another source/path. No arbitrary currency
graph traversal, network refresh, inference from institution metadata, or
reverse conversion of rounded household values occurs. Native decimals and FX
intermediates use the existing decimal arithmetic; components are summed before
the existing four-decimal MoneyView boundary and localized currency formatting.
Non-terminating inverse division retains the existing decimal division precision.

`BaseValue`, household completeness and missing inputs are not overwritten.
Overview, Portfolio, historical valuation and ledger facts retain their existing
semantics. Institution grouping, ordering and displayed group totals continue to
use household values. Composition charts and their shares remain household-based
and now state that currency explicitly. The existing group subtotal behavior for
partial household values is outside this display-currency change.

Focused checks: `TestAccountDisplayValue*`,
`TestAccountDisplayValueWireIndependentCompleteness`, `accountDisplayMoney`,
`AccountsPage`, `AccountCompositionPanel`, `accountGroups` and locale coverage.
Native macOS layout/accessibility acceptance remains a separate manual gate;
jsdom checks do not establish native rendering or keyboard geometry.

### Refresh dependencies for account display

The shared required-refresh collector includes account display FX dependencies
for nonzero current native components. An explicit native/account-currency pair
preference makes that pair required; non-provider preferences remain skipped.
Without that preference, the collector adds the account/household bridge leg to
the existing native/household target. All three ordinary refresh entrypoints
(All, Missing or stale, and Required FX) share these targets and existing cache,
provider selection and rate-limit handling. Empty accounts and same-currency
native components do not add a new display dependency. Reads remain network-free.
`TestAccountDisplayRefreshTargets` and `TestAccountDisplayRefreshToValuation`
cover collection, preferences and the refresh-to-display integration.
