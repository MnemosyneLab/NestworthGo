# Round-3 Phase C — Probe pack

**When:** 2026-09-09 15:46 SGT  
**Repo:** `qa/analytics-linux-r3-2026-09-09` @ `91d10c0`  
**Against:** complete DB unless noted; missing-both for C4  
**Counts:** **4 PASS / 0 FAIL / 5 SKIP**

## Results

| ID | Name | Status | Artifact / note |
|---|---|---|---|
| C1 | Fixtures A–D | **PASS** | `seed/probe-fixtures.json` — 4 PASS / 0 FAIL |
| C2 | IncludeCash FL-18/19 | **PASS** | `seed/probe-cash-include.json` — salary capital differs include vs exclude |
| C3 | True-zero Aug 31 | **PASS** | `logs/C3-true-zero-complete.out` — `2026-08-31 rate=0 status=ok` |
| C4 | missing-both All | **PASS** | `logs/C4-missing-both.out` — partial 21/45, no panic |
| C5 | Scope triangulation | **SKIP** | No DB-backed scope probe mode (fixture D only in C1) |
| C6 | Native vs Base | **SKIP** | No probe mode / env flag |
| C7 | Return sources | **SKIP** | No sources-sum mode; bonus co03 `PASS_BOTH` on complete |
| C8 | Transfer neutrality | **SKIP** | No transfer probe mode |
| C9 | Residual | **SKIP** | Needs corrupt residual DB; complete clean (`issues=0`) |

## Existing probe modes used

`NESTWORTH_PROBE_MODE` values present in `cmd/analytics-qa-probe`: `fixtures`, `cash-include`, `co03`, `residual`, plus legacy/default.

No modes for: scope triangulation, native/base forced flag, return-sources reconciliation, transfer-day ≈0.

## Notes

- C3 All-range on complete still reports Analyze `status=partial` (origin UNAVAIL + early partial days) even though seed `incomplete_days=0` (snapshot completeness ≠ Dietz day rating). True-zero assertion still holds.
- C4 reconfirms DEF-R2-02 fix path: All-range on missing-both returns partial coverage without panic.
- C9 residual probe was executed once against complete for evidence (`seed/probe-residual.json`); status recorded as SKIP (not FAIL) per Phase C guidance — corrupt residual fixture not part of the four scenario DBs.
- No GUI; no push.
