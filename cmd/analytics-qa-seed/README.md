# analytics-qa-seed (fixture `analytics-linux-qa-v3`)

Deterministic Insights Round-3 “mini family” for desktop/probe acceptance of the analytics test plan (§6 / FC-01–06). Extends the v2 AUD Cash + US Brokerage + AAPL story; env names are unchanged.

## Env

| Variable | Default | Notes |
|---|---|---|
| `NESTWORTH_QA_SCENARIO` | `complete` | `complete` \| `missing-price` \| `missing-fx` \| `missing-both` |
| `NESTWORTH_QA_ANCHOR` | `2026-07-26T00:00:00Z` | RFC3339 UTC history origin |
| `NESTWORTH_QA_RESET` | unset | Must be `1` to replace an existing QA database |
| `NESTWORTH_QA_OUTPUT_DIR` | `/workspace/nestworth-analytics-qa` | Artifact root; reset only allows DBs under this tree |
| `NESTWORTH_DATABASE_PATH` | `$OUTPUT_DIR/data/nestworth.db` | Isolated sqlite file |
| `NESTWORTH_QA_COMMIT` | `git rev-parse HEAD` | Recorded in `seed-results.json` |

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

## FC-07 loan (skipped)

`DebtDrawInput` / `DebtPaymentInput` exist, but a loan sleeve is **not** seeded. Adding a liability before `StartHistory` would expand origin components, FX/completeness surface, and the true-zero constraint. Round-3 mini-family stays on multi-account / multi-instrument / transfer / same-day trade. Revisit FC-07 in a later fixture if desktop needs drawdown / repayment / cash interest.

## Probe helpers

Existing `cmd/analytics-qa-probe` modes (`cash-include`, `co03`, `residual`, `fixtures`) read this DB without v2-only ID assumptions. No probe changes in v3.
