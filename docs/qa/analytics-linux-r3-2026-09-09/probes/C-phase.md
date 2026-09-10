# Round-3 Phase C — Probe pack

**When:** 2026-09-09 15:46 SGT  
**Repo:** `qa/analytics-linux-r3-2026-09-09` @ `91d10c0`  
**Against:** complete DB unless noted; missing-both for C4  
**Counts:** **9 PASS / 0 FAIL / 0 SKIP** for the automated probe pack. Native desktop matrix remains a separate NOT_RUN gate.

## Results

| ID | Name | Status | Artifact / note |
|---|---|---|---|
| C1 | Fixtures A–D | **PASS** | `seed/probe-fixtures.json` — 4 PASS / 0 FAIL |
| C2 | IncludeCash FL-18/19 | **PASS** | `seed/probe-cash-include.json` — salary capital differs include vs exclude |
| C3 | True-zero Aug 31 | **PASS** | `logs/C3-true-zero-complete.out` — `2026-08-31 rate=0 status=ok` |
| C4 | missing-both All | **PASS** | `logs/C4-missing-both.out` — partial 21/45, no panic |
| C5 | Scope triangulation | **PASS** | `NESTWORTH_PROBE_MODE=quantitative`; independent boundary snapshot totals reconcile household, accounts, instruments + explicit cash |
| C6 | Native vs Base | **PASS** | Same quantitative probe; mixed account currencies force Base and instrument Native uses quote currency |
| C7 | Return sources | **PASS** | Same quantitative probe; Price/FX/Dividend/Fee source sum equals period return with coverage emitted |
| C8 | Transfer neutrality | **PASS** | Same quantitative probe; fixed-snapshot counterfactual removes in-range `cash_transfer` activities, then compares household change/external flow, Price/FX/Fee, Return sources, and both endpoint accounts. 2026-08-16 endpoints are `+200/-200 AUD`; every with-vs-without delta is `0`. |
| C9 | Residual | **PASS** | `residual` clean copy has issues=0; isolated corrupt copy adds +100 AUD and retains ±100 AUD residual details |

## Existing probe modes used

`NESTWORTH_PROBE_MODE` values present in `cmd/analytics-qa-probe`: `fixtures`, `cash-include`, `co03`, `residual`, `reconcile`, `quantitative`, plus legacy/default. The quantitative C8 case now runs an explicit with/without-transfer counterfactual; it does not close transfer neutrality from `external_flow=0` alone.

The raw run output is summarized in `REMEDIATION-2026-09-09.json`; the residual mode always works on a copied database. `reconcile` keeps exact DTO-style decimal values and returns non-zero only for a top-level mismatch or probe error.

## Notes

- C3 All-range on complete still reports Analyze `status=partial` (origin UNAVAIL + early partial days) even though seed `incomplete_days=0` (snapshot completeness ≠ Dietz day rating). True-zero assertion still holds.
- C4 reconfirms DEF-R2-02 fix path: All-range on missing-both returns partial coverage without panic.
- The prior C9 clean-only run remains historical; the current clean/corrupt positive and negative controls are recorded in `REMEDIATION-2026-09-09.json`.
- No new native Wails GUI run was performed in this remediation turn; missing-price/missing-fx and zh-CN desktop checks remain NOT_RUN.
