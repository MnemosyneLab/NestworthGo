# Historical reconciliation validation

Use isolated synthetic data and the checkout’s pinned toolchain.
Design: [endpoint bridge](../development/historical-reconciliation.md).

## Accounting evidence

The application regression matrix covers:

- exact identity: total = sum(category contributions) + residual;
- corrected income at original economic time, normalized subsequent balance
  observations, later undo and reversed/same-day comparisons;
- neutral internal transfers and debt principal, fees and debt interest;
- USD income at later FX, stable holding price and opening FX without overlap:
  income 80 + price 32 + FX 120 = movement 232 CNY, with exact negation on reverse;
- an isolated SQLite CNY/USD transfer: movement 20 CNY, supported 0, residual
  20, with foreign exposure and imbalance reasons; reversed residual −20;
- liability FX signs, missing price/rate evidence, inclusion/state changes,
  zero-net uncertainty, quantity round trips and a 0.000000000001 residual;
- inverse FX division at existing precision and a 23-hour DST interval with
  start-exclusive/end-inclusive selection;
- input work-budget refusal, pre-cancellation, and immutable input serialization.

Existing overview/series tests continue to cover read-only SQL, shared captures,
missing provider coverage/backfill, civil-day discontinuities and cancellation.
The reconciliation uses the same batch, adding no queries or financial writes.

## UI evidence

Frontend regression tests cover all three locales, exact tiny/negative amounts,
unknown versus zero, retained uncertainty, keyboard Enter/Space expansion using
the shared Button and four rapid date selections followed by stale replies.
The UI uses the existing capture hook, global tabs/date pickers and theme tokens.
No new chart or external provider is introduced.

## Timestamp continuity regression (review follow-up)

At initial PR head `13a4c9560c7c40a2a758453293b9b015957cedb0`, supported Agent
`close` imports for an unchanged two-unit CNY holding (10 to 12 per unit)
reproduced a gap in both `date_label` → `source_timestamp` and the reverse basis
transition. Comparison reported supported price 4, residual 0, `reconciled`,
while the unchanged trend reported `BasisChanged=true`, `ConnectNetWorth=false`.
Source, source key, currency, observation kind, price policy and quantity stayed
constant; no preference or inclusion/state change explains the difference.

The timestamp authority is intentionally distinct: the Agent import contract
in `docs/development/mcp.md` states that a date-label anchor is not an asserted
midnight market quote. Price comparison now requires identical timestamp basis,
as FX comparison and the trend already do. The integration regression uses
actual Agent import APIs and isolated SQLite, then enables `query_only`. It
checks partial attribution with supported 0, residual 4 and `basis_changed`,
ordinary/trend-bundle agreement, reversed residual −4 and exact identity.
Same-basis controls retain price contribution 4, residual 0 and connected trend.
No trend, valuation, ledger, provider, frontend or binding contract is weakened.

## Trading and quantity reconciliation (`endpoint_bridge/2`)

Baseline investigation used a separate worktree at `1701e5d` and synthetic
retained batches. The original algorithm reported supported 0 / residual 5010
for a 100 g gold position (500 to 550 per gram) plus a 1 g purchase for 540.
A flat-NAV money fund with an unpriced one-share remeasurement reported
supported 0 / residual 1. Both reported `quantity_activity`. These independent
probes confirm the whole-position exclusion and the missing monetary quantity
adjustment; they do not inspect or explain any real USDC/USDT ledger difference.

The regression suites use isolated synthetic SQLite and supported commands,
plus adversarial immutable capture fixtures. Representative results:

| Synthetic case | Supported reconciliation |
| --- | --- |
| Gold, 100 g plus 1 g purchase at 540, quote 500 → 550, fee 1 | Valuation after principal 5010; fee −1; residual 0 |
| Fund, two purchases (10 and 20 shares), fees 0.1 + 0.2, NAV 1 → 1.1 | Valuation after principal 12; fees −0.3 once; residual 0 |
| Flat-NAV money fund, 100 → 102 shares | Recorded adjustment 2; income 0; residual 0 |
| Sell four of ten units, gross 44, quote 10 → 12, fee 1 | Valuation after principal 16; fee −1; residual 0 |
| Buy/sell round trip, ending quantity unchanged | Recorded execution spread remains in valuation after principal; fees counted once |
| New holding and newly-created instrument | Proven opening zero; one initial preference is not a basis switch; later/repeated preferences still block attribution |
| USD buy and complete liquidation | Later-rate principal and valuation plus opening FX, without overlap |
| Partial/full same-instrument transfer across accounts, CNY and USD | Opposite paired principal; no income or external flow; household valuation/FX retained |
| Unpriced reverse split (100 × 10 → 50 × 20) | No inferred income/adjustment; partial status and restatement uncertainty despite zero residual |
| Inverse FX division | Exact −0.0000000000000001 residual retained |

Tests also cover retained trade corrections and undo, cash dividends, ordinary
income and cash remeasurement without double counting; reverse comparisons and
trend-bundle parity; quantity gaps, ambiguous multi-leg principal, missing trade
detail, missing FX/unknown totals, price/FX authority changes and native valuation
mismatches. Leaf identities and household sums are asserted, including zero-sum
paired principal. Related records and known leaf evidence survive incomplete
household totals. Input serialization checks prove the capture is unchanged.
SQLite integration comparisons run with `PRAGMA query_only=ON`.

MCP tests retrieve details and their separately pageable records from the same
frozen capture. A 120-record fixture verifies that notes/records cannot inflate
the reconciliation header or be duplicated/lost across pages. Existing complete
wire-size, cursor and read-only historical checks remain in place.

UI tests exercise exact audit decimals, unknown versus zero, English/Simplified
Chinese/Traditional Chinese labels, account/instrument grouping, related effects,
internal-ID non-disclosure and Enter/Space collapse behavior. Chromium checks of
the actual React component with synthetic data cover 1280 px Simplified Chinese
light mode, 390 px Traditional Chinese dark mode and 390 px English light mode.
All three have no page errors or document horizontal overflow, and retain the
exact tiny residual and keyboard controls. This is browser evidence, not native
macOS or VoiceOver acceptance.

Validation environment: cloud Linux, Go 1.27.3, Node 24.22.0, pnpm 12.14.0,
Wails 3.1.0. The prepared Linux container supplies GTK4/WebKit6 headers for the
canonical desktop tests/build and official binding generation. No generated
bindings or frontend bundles are committed. No real household data was opened,
created from a copy, uploaded or used for validation.

Additional reconciliation work and retained detail are linear in the bounded
capture's accounts, holdings, instruments, leaves, records/effects and metadata
observations. Existing 100000-work and time/cancellation limits remain; there
are no new queries, providers or financial writes. Large-ledger performance and
native macOS packaging/GUI acceptance were not measured by these small fixtures.
