# analytics-qa-seed (fixture `analytics-linux-qa-v3`)

Deterministic Insights Round-3 “mini family” for desktop/probe acceptance of the analytics test plan (§6 / FC-01–06). Extends the v2 AUD Cash + US Brokerage + AAPL story; env names are unchanged.

## Env

| Variable | Default | Notes |
|---|---|---|
| `NESTWORTH_QA_SCENARIO` | `complete` | `complete` \| **`complete-usd`** \| **`complete-cny`** \| `missing-price` \| `missing-fx` \| `missing-both` \| **`loan-fc07`** \| **`loan-fc07-utc`** \| **`loan-fc07-cny`** \| **`loan-fc07-usd`** \| **`loan-fc07-week-sunday`** |
| `NESTWORTH_QA_ANCHOR` | `2026-07-26T00:00:00Z` | RFC3339 UTC history origin |
| `NESTWORTH_QA_RESET` | unset | Must be `1` to replace an existing QA database |
| `NESTWORTH_QA_OUTPUT_DIR` | `/workspace/nestworth-analytics-qa` | Artifact root; reset only allows DBs under this tree |
| `NESTWORTH_DATABASE_PATH` | `$OUTPUT_DIR/data/nestworth.db` | Isolated sqlite file |
| `NESTWORTH_QA_COMMIT` | `git rev-parse HEAD` | Recorded in `seed-results.json` |
| `NESTWORTH_QA_BASE_CURRENCY` | `AUD` | Optional `AUD` \| `USD` \| `CNY` for **`complete` / `complete-*` only**. `missing-*` stay AUD. |
| `NESTWORTH_QA_LOAN_CURRENCY` | scenario default | Optional `AUD` \| `CNY` \| `USD` override for `loan-*` |
| `NESTWORTH_QA_LOAN_TZ` | scenario default | Optional IANA timezone override for `loan-*` |
| `NESTWORTH_QA_WEEK_START` | scenario default | Optional `monday` \| `sunday` override for `loan-*` |
| `NESTWORTH_QA_WINDOW_W` | `1280` | Optional settings `window_width` for `loan-*` |

Smoke (all four scenarios):

```bash
ROOT=/tmp/nestworth-qa-v3
for s in complete missing-price missing-fx missing-both; do
  NESTWORTH_QA_OUTPUT_DIR="$ROOT/$s" \
  NESTWORTH_QA_RESET=1 \
  NESTWORTH_QA_ANCHOR=2026-07-26T00:00:00Z \
  NESTWORTH_QA_SCENARIO=$s \
  go run ./cmd/analytics-qa-seed
done
```

`seed-results.json` records `fixture_version`, scenario, IDs, and per-step PASS/FAIL.

§5 base currency (real onboarded DBs, default AUD `complete` unchanged):

```bash
# explicit scenario IDs
for s in complete-usd complete-cny; do
  NESTWORTH_QA_OUTPUT_DIR=/tmp/nestworth-qa-v3-$s NESTWORTH_QA_RESET=1 \
  NESTWORTH_QA_ANCHOR=2026-07-26T00:00:00Z NESTWORTH_QA_SCENARIO=$s \
  go run ./cmd/analytics-qa-seed
done

# equivalent env knob on complete
NESTWORTH_QA_OUTPUT_DIR=/tmp/nestworth-qa-v3-complete-usd NESTWORTH_QA_RESET=1 \
NESTWORTH_QA_SCENARIO=complete NESTWORTH_QA_BASE_CURRENCY=USD \
go run ./cmd/analytics-qa-seed
```

`complete-usd` / `complete-cny` keep the v3 account/instrument/activity timeline. FX preferences and manual quotes are written **against the household base** (USD: AUD/USD + SGD/USD derived from the AUD schedule; CNY: USD/CNY + AUD/CNY + SGD/CNY). `missing-*` stay AUD so the R3 completeness matrix does not move.

FC-07 loan is a **separate** scenario (`loan-fc07`) so the v3 complete / missing-* mini-family stays unchanged.

```bash
ROOT=/tmp/nestworth-qa-loan-fc07
NESTWORTH_QA_OUTPUT_DIR="$ROOT" \
NESTWORTH_QA_RESET=1 \
NESTWORTH_QA_ANCHOR=2026-07-26T00:00:00Z \
NESTWORTH_QA_SCENARIO=loan-fc07 \
go run ./cmd/analytics-qa-seed

NESTWORTH_DATABASE_PATH="$ROOT/data/nestworth.db" \
NESTWORTH_QA_OUTPUT_DIR="$ROOT" \
NESTWORTH_PROBE_MODE=loan-fc07 \
NESTWORTH_PROBE_OUT="$ROOT/seed/probe-loan-fc07.json" \
go run ./cmd/analytics-qa-probe
```

## Household (AUD base)

| ID key | Name | Type / tracking | Currency |
|---|---|---|---|
| `acct_cash` | AUD Cash | bank / balance | AUD |
| `acct_broker` | US Brokerage | brokerage / holdings | USD cash (+ AUD sleeve after FC-01) |
| `acct_sg` | SG Brokerage | brokerage / holdings | SGD cash |
| `inst_aapl` / `holding_aapl` | Apple Inc | stock | USD |
| `inst_qqq` / `holding_qqq` | Invesco QQQ Trust | etf | USD |
| `inst_es3` / `holding_es3` | SPDR STI ETF | etf (SGD-quoted) | SGD |

Manual FX preferences: **USD/AUD** and **SGD/AUD**. Timezone for history: `Asia/Singapore`.

## Event timeline (offsets from anchor)

Local dates below assume the default anchor (`2026-07-26T00:00:00Z` → SGT local origin `2026-07-26`).

| Day | Local (default) | Event | Plan |
|---|---|---|---|
| 1 | 2026-07-27 | US Brokerage +20000 USD contribution | funding |
| 2 | 2026-07-28 | AUD Cash +5000 AUD contribution | funding |
| 3 | 2026-07-29 | AUD Cash → US Brokerage **1000 AUD** (same-currency sleeve) | **FC-01** |
| 4 | 2026-07-30 | SG Brokerage +8000 SGD contribution | funding |
| 5 | 2026-07-31 | Buy 10 AAPL ~1800 USD fee 5 | v2 continuity |
| 7 | 2026-08-02 | AUD Cash **+500 AUD interest** | **FC-05** |
| 8 | 2026-08-03 | QQQ buy 1 @ 400 then sell 1 @ 410 (same local day, ending qty 0) | **FC-03** |
| 10 | 2026-08-05 | AUD Cash +3000 AUD **income/salary** | **FC-05** vs interest |
| 12 | 2026-08-07 | Buy 2 QQQ ~810 USD (hold) | **FC-02** |
| 15 | 2026-08-10 | AUD Cash -200 AUD expense | v2 |
| 18 | 2026-08-13 | Buy 10 ES3 ~35 SGD | **FC-04** foreign SGD holding |
| 20 | 2026-08-15 | Buy 5 AAPL ~950 USD | v2 |
| 21 | 2026-08-16 | US Brokerage AUD sleeve → AUD Cash **200 AUD** | **FC-01** reverse |
| 22 | 2026-08-17 | Completeness gap day (omit price and/or FX per scenario) | §17 |
| 30 | 2026-08-25 | US Brokerage +1000 USD contribution | v2 |
| 32 | 2026-08-27 | Sell 3 AAPL gross 615 USD (partial, clear realized gain) | CO-02 / Realized Gain |
| 33 | 2026-08-28 | AAPL cash dividend +12.50 USD (no assumed withholding) | **FC-06** |
| 34 | 2026-08-29 | AUD Cash -15 AUD bank fee | CAT-03 |
| 36 | 2026-08-31 | True-zero day: flat AAPL/QQQ/ES3 + USD/AUD + SGD/AUD, no activity | RC-07 |

AAPL still has the v2 negative-price window on days 25–28.

## Completeness matrix

Quotes are written for days `0..44`. Through **day 22 inclusive**:

| Scenario | Instrument prices (AAPL, QQQ, ES3) | FX (USD/AUD, SGD/AUD) | Gap-day snapshot |
|---|---|---|---|
| `complete` | written | written | complete |
| `missing-price` | omitted | written | incomplete |
| `missing-fx` | written | omitted | incomplete |
| `missing-both` | omitted | omitted | incomplete |

Rebuild must produce the incomplete snapshot; the seed does **not** SQL-inject `complete=0`.

Household-scope Dietz `ratedDays` on a long Include-cash window can still be below `totalDays` even when every snapshot is complete (zero-capital or unrateable days). Use the gap-day snapshot flags and the seed `missing_quote_gap` step for §17, not period `ratedDays`.

## FC-07 loan (`NESTWORTH_QA_SCENARIO=loan-fc07*`)

FC-07 is a **separate** scenario family so the v3 complete / missing-* mini-family stays unchanged. Default `loan-fc07` is AUD + `Asia/Singapore` + week start Monday (R3 continuity). Round-4 matrix siblings:

| Scenario | Base currency | History timezone | `week_start` |
|---|---|---|---|
| `loan-fc07` | AUD | Asia/Singapore | monday |
| `loan-fc07-utc` | AUD | UTC | monday |
| `loan-fc07-cny` | CNY | Asia/Singapore | monday |
| `loan-fc07-usd` | USD | Asia/Singapore | monday |
| `loan-fc07-week-sunday` | AUD | Asia/Singapore | sunday |

Single-currency so the loan fixture does not invent FX fills. Env overrides (`NESTWORTH_QA_LOAN_CURRENCY`, `NESTWORTH_QA_LOAN_TZ`, `NESTWORTH_QA_WEEK_START`) apply to any `loan-*` scenario.

Fixture version `analytics-linux-qa-loan-fc07`. Household: `{CCY} Cash` (bank / balance, initial 20000) + `{CCY} Loan` (`TypeLoan` / `RoleLiability` / balance, initial 0). Default anchor `2026-07-26T00:00:00Z`.

| Day | Local (SGT default) | Event | Oracle (test-plan FC-07 / Cases 16–17) |
|---|---|---|---|
| 1 | 2026-07-27 | `DebtDrawInput` 100000 | Cash +P, Debt +P, **net worth change = 0**, not investment return |
| 3 | 2026-07-29 | `DebtPaymentInput` principal 10000 | Cash −P, Debt −P, **net worth change = 0** |
| 5 | 2026-07-31 | `DebtPaymentInput` principal 1000 + `InterestOrFee` 500 | Cash −1500, debt principal −1000, **Spending −500**, net worth −500; must **not** be Dividend & Interest / investment return |

Seed JSON asserts OVERALL plus named steps `fc07_draw_nw0`, `fc07_repay_nw0`, `fc07_interest_spending`. Principal on the interest day is **1000** (API requires principal > 0; `InterestOrFee`-only is rejected). Amounts match `TestAnalysisReviewCases16And17RealDebtPaymentPath`.

The v3 complete / missing-* family still omits a loan sleeve. Use `loan-fc07*` when probes or desktop need draw / repay / cash interest.

Round-4 timezone/DST Origin DBs are **not** this seed. Use `NESTWORTH_PROBE_MODE=timezone-r4` (builds Origin DBs for `Asia/Singapore`, `UTC`, `America/Los_Angeles` and asserts local calendar days around 2026-03-08 / 2026-11-01, including LA gap/ambiguity rejection). `NESTWORTH_PROBE_MODE=r4-matrix` aggregates timezone-r4 + settings week/window/locale Validate + loan extras + complete-DB instrument/cash-include when those DBs exist.

```bash
ROOT=/tmp/nestworth-qa-loan-fc07
for s in loan-fc07 loan-fc07-utc loan-fc07-cny loan-fc07-usd loan-fc07-week-sunday; do
  NESTWORTH_QA_OUTPUT_DIR="$ROOT/$s" \
  NESTWORTH_QA_RESET=1 \
  NESTWORTH_QA_ANCHOR=2026-07-26T00:00:00Z \
  NESTWORTH_QA_SCENARIO=$s \
  go run ./cmd/analytics-qa-seed
done
```

## Probe helpers

`cmd/analytics-qa-probe` modes:

| `NESTWORTH_PROBE_MODE` | Reads |
|---|---|
| `cash-include`, `co03`, `residual`, `fixtures`, `quantitative`, `reconcile` | v3 / engine fixtures |
| `loan-fc07` | loan DB: FC-07 oracles + Base/Native + household/account scope + Include/Exclude on the draw day + settings week/currency |
| `timezone-r4` | self-contained TZ/DST DBs + in-memory oracles (no FX fills) |
| `r4-matrix` | timezone-r4 + settings.Validate matrix + loan extras (`NESTWORTH_QA_LOAN_DB`) + complete extras (`NESTWORTH_QA_COMPLETE_DB`) + `complete-usd`/`complete-cny` (`NESTWORTH_QA_COMPLETE_USD_DB` / `NESTWORTH_QA_COMPLETE_CNY_DB`); SKIP missing DBs; BLOCKED desktop/macOS/locale-visual rows |

SKIP and BLOCKED do not fail `allPass`. FAIL does.

