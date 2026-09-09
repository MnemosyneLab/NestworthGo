# Round-3 Phase B — Seed summary

**When:** 2026-09-09 ~15:44 SGT  
**Repo:** `qa/analytics-linux-r3-2026-09-09` @ `91d10c0` (seed v3)  
**Anchor:** `2026-07-26T00:00:00Z`  
**Output:** `/workspace/nestworth-analytics-qa-r3/`  
**Logs:** `logs/B-seed-<scenario>.log`  
**Results:** `seed/seed-results-<scenario>.json`

## Overall

| Scenario | OVERALL | DB |
|---|---|---|
| complete | **PASS** | `data/nestworth-complete.db` |
| missing-price | **PASS** | `data/nestworth-missing-price.db` |
| missing-fx | **PASS** | `data/nestworth-missing-fx.db` |
| missing-both | **PASS** | `data/nestworth-missing-both.db` |

All four: **4/4 OVERALL PASS**.

## Completeness / gap / true-zero

Gap day = **2026-08-17** (offset 22). True-zero day = **2026-08-31** (offset 36).

| Scenario | incomplete_days | gap complete | gap missing_items | true-zero (Aug 31) |
|---|---:|---:|---:|---|
| complete | 0 | 1 | 0 | PASS — status=ok amount=0 rate=0 |
| missing-price | 18 | 0 | 3 | PASS — status=ok amount=0 rate=0 |
| missing-fx | 22 | 0 | 5 | PASS — status=ok amount=0 rate=0 |
| missing-both | 22 | 0 | 5 | PASS — status=ok amount=0 rate=0 |

Notes:
- `missing_items` from seed check `missing_quote_gap` (`COUNT` of `daily_valuation_snapshot_items` with `complete=0` on the gap snapshot).
- `incomplete_days` from `verify_snapshots` (`COUNT` of incomplete snapshot days in the rebuilt window).
- For `missing-fx`, `verify_snapshots` note may show a lower `missing=` on the snap string (component-level) than `missing_items` from the gap check; both checks still **PASS** for the scenario contract.

## Fixture shape (all scenarios)

- Accounts: AUD Cash, US Brokerage, SG Brokerage  
- Instruments: AAPL (USD), QQQ (USD), ES3 (SGD)  
- Activities: contrib/transfer/buy/sell/salary/interest/dividend/fee (v3 mini-family)  
- Rebuild: 45 snapshots `2026-07-26..2026-09-08`

