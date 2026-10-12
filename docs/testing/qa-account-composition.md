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
interaction mapping. English remainder labels use singular/plural forms.

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
- Mouse hover/selection, keyboard focus, screen-reader grouping controls,
  expand/collapse state, and the full data table.
- Partial, all-missing, zero, and invalid synthetic fixtures; no misleading pie.
- Overview/Portfolio chart behavior and Historical Panorama views after the
  main update. No real household profile is needed.

No merge, release, tag, packaging, or publication is authorized by this task.
