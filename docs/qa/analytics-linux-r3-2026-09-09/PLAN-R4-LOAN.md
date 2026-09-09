# Nestworth Insights QA — Round-4 Loan + Timezone Plan

| Field | Value |
|---|---|
| Base | Branch `qa/analytics-linux-r3-2026-09-09` (PR #15); do not rewrite Round-3 `REPORT.md` |
| Prior | Round-3 mini-family v3 (`complete` / `missing-*`); FC-07 loan skipped; full TZ/DST matrix deferred |
| Goal | Enable Linux QA to run **seed → probe → desktop** for FC-07 Loan and Round-4 timezone/DST |
| Out | Rewriting R3 reports; inventing FX fills; weakening Cases 16/17 or history_regression oracles |
| Artifact root (suggested) | `/workspace/nestworth-analytics-qa-loan-fc07/` and `…/timezone-r4/` |
| Docs target | `docs/qa/analytics-linux-r3-2026-09-09/` (this plan + optional STATUS stub only) |

---

## 0. Principles

1. **Separate scenario, not a v3 graft** — `NESTWORTH_QA_SCENARIO=loan-fc07` must not change complete / missing-price / missing-fx / missing-both.
2. **Seed + probe first**, desktop second — JSON PASS/FAIL is the gate; screenshots only after probes are green.
3. **Stay inside test-plan v1 FC-07** — draw / repay / cash interest numbers are acceptance oracles.
4. **Reuse existing DST oracles** — `TestResolveLocalDateTimeRejectsDSTGapAndAmbiguity`, `TestAnalysisReturnDST*`, `activityLocalDate` / Case 40. Do not invent calendar rules.
5. **AUD base** — continuity with R3; single-currency loan fixture so no FX quotes are required.

---

## 1. Product contracts (FC-07)

From `docs/testing/nestworth-analytics-test-plan.md` and `TestAnalysisReviewCases16And17RealDebtPaymentPath`:

| Step | Ledger | Net worth | Must not be |
|---|---|---|---|
| Drawdown | Cash +100000, Debt +100000 | change = 0 | Investment return |
| Principal repayment | Cash −10000, Debt −10000 | change = 0 | Investment return |
| Cash interest (`InterestOrFee` on `DebtPaymentInput`) | Cash −(principal+500), debt principal −principal only | −500; **Spending −500** | Dividend & Interest / investment return |

API: `DebtDrawInput`, `DebtPaymentInput` (principal required > 0; interest/fee optional). Account `TypeLoan` / `RoleLiability`. Interest-day principal in the fixture is **1000** (same as Case 17) because principal cannot be 0.

---

## 2. Phases

### Phase L — Loan seed (`cmd/analytics-qa-seed`, scenario `loan-fc07`)

| ID | Work | Done when |
|---|---|---|
| L1 | AUD Cash + AUD Loan before `StartHistory`; tz `Asia/Singapore`; anchor `2026-07-26T00:00:00Z` | DB created under output dir |
| L2 | Day 1 draw 100000; day 3 repay 10000; day 5 pay 1000 + interest 500 | Activities persist; rebuild window covers all three |
| L3 | Seed JSON named steps `fc07_draw_nw0`, `fc07_repay_nw0`, `fc07_interest_spending` | OVERALL PASS |

**Exit L:** `go run ./cmd/analytics-qa-seed` with `NESTWORTH_QA_SCENARIO=loan-fc07` writes DB + `seed/seed-results.json` OVERALL PASS.

### Phase P — Probes (`cmd/analytics-qa-probe`)

| ID | Mode | Asserts |
|---|---|---|
| P1 | `loan-fc07` | Open loan DB; draw/repay day net-worth/attribution neutrality; interest day Spending present; return not inflated |
| P2 | `timezone-r4` | Origin TZ `Asia/Singapore`, `UTC`, `America/Los_Angeles`; local calendar day for known UTC instants around DST spring-forward **2026-03-08** and fall-back **2026-11-01**; DST gap/ambiguity rejected; LA day length 23h / 25h |

Fall-back 2026-11-01 is **after** a 2026-09-09 wall clock, so snapshot rebuild cannot cover November. P2 therefore: (a) builds spring DBs (closed days), (b) asserts fall-back via `activityLocalDate` + in-memory `ComputeAnalysis` (same as unit oracles). No FX fills.

**Exit P:** both modes write JSON `allPass=true` and exit 0.

### Phase D — Desktop (Linux QA agent, after this tooling lands)

| ID | Work | Done when |
|---|---|---|
| D-FC07 | Insights on `loan-fc07` DB: draw/repay days net-worth flat; interest day Categories Spending −500 | RESULTS.json |
| D-TZ | Optional: open a timezone-r4 DB and confirm History Origin tz + activity local date | RESULTS.json |

**Exit D:** not this change; leave full `REPORT.md` to the Linux QA agent after runs.

---

## 3. Exit criteria (tooling ready)

**Met only if all true:**

1. `loan-fc07` seed produces DB + OVERALL PASS including the three named FC-07 steps  
2. `NESTWORTH_PROBE_MODE=loan-fc07` PASS on that DB  
3. `NESTWORTH_PROBE_MODE=timezone-r4` PASS (domain + in-memory + spring Origin DBs)  
4. v3 `complete` / `missing-*` scenarios still green  
5. This plan committed; Round-3 REPORT / REPORT-RETEST / STATUS.md **unchanged**

**Green product acceptance** additionally requires the Linux QA agent to run seed → probe → desktop and write the Round-4 report.

---

## 4. Suggested runbook

```bash
ROOT=/tmp/nestworth-qa-loan-fc07
NESTWORTH_QA_OUTPUT_DIR="$ROOT" NESTWORTH_QA_RESET=1 \
NESTWORTH_QA_ANCHOR=2026-07-26T00:00:00Z \
NESTWORTH_QA_SCENARIO=loan-fc07 \
go run ./cmd/analytics-qa-seed

NESTWORTH_DATABASE_PATH="$ROOT/data/nestworth.db" \
NESTWORTH_QA_OUTPUT_DIR="$ROOT" \
NESTWORTH_PROBE_MODE=loan-fc07 \
NESTWORTH_PROBE_OUT="$ROOT/seed/probe-loan-fc07.json" \
go run ./cmd/analytics-qa-probe

NESTWORTH_QA_OUTPUT_DIR="$ROOT" \
NESTWORTH_PROBE_MODE=timezone-r4 \
NESTWORTH_PROBE_OUT="$ROOT/seed/probe-timezone-r4.json" \
go run ./cmd/analytics-qa-probe
```

Keep v3 smoke (four scenarios) unchanged; see `cmd/analytics-qa-seed/README.md`.

---

## 5. Decision log

- AUD (not CNY) for R3 continuity; Case 16/17 numbers are currency-agnostic oracles  
- Separate scenario rather than a flag on v3 (avoids origin-component / true-zero / completeness coupling)  
- Interest day uses principal 1000 + fee 500 because `DebtPaymentInput` rejects zero principal  
- Fall-back DST via in-memory analysis: November 2026 is not a closed day on a Sep 2026 wall clock  
- Product code for debt/DST is **out of scope** unless probes FAIL against the existing oracles
