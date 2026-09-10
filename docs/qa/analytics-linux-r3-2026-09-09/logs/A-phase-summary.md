# Round-3 Phase A summary

**When:** 2026-09-09 ~15:32 SGT  
**Repo:** `qa/analytics-linux-r3-2026-09-09` @ `ea94f48`  
**Artifact root:** `/workspace/nestworth-analytics-qa-r3/`  
**Binary:** `binaries/nestworth` present (no rebuild)

## Done

| ID | Work | Outcome |
|---|---|---|
| A1 | missing-both + All (origin→last closed) | **PASS** — `logs/03-probe-missing-both-all.out`: coverage **partial 21/45**, exit 0, no panic |
| A2 | Trend presets × missing-both + complete | **PASS** — see `probes/A-trend-presets.json`; logs under `logs/A-trend/` |
| A3 | Desktop Trend All screenshot | **Not done** this pass (pending) |
| A4 | Defect card | **Written** — `report/DEF-R2-02.md` |

## DEF-R2-02

- **Status: FIXED** (probe) — pending desktop confirm.
- Fix: classifier `convertValuationAmount` soft-fails missing FX on base valuation.
- Cite: `logs/03-probe-missing-both-all.out` (partial 21/45, no panic).

## A2 quick table

| Scenario | All | Custom mid (08-01..09-07) | ~30D (08-10..09-08) |
|---|---|---|---|
| missing-both | partial 21/45 | partial 20/38 | partial 21/30 |
| complete | partial 39/45* | **ok** 38/38 | **ok** 30/30 |

\*complete All still partial due to origin UNAVAIL + early Jul partial days in seed window.

## Notes

- Probe already supports `NESTWORTH_PROBE_FROM` / `NESTWORTH_PROBE_TO` in `runLegacyProbe` — no code change.
- Restored `data/nestworth-complete.db` from r2 (r3 copy was truncated → `panic: household was not found`).
- PLAN copied to repo: `docs/qa/analytics-linux-r3-2026-09-09/PLAN.md`.
- No push; seed v3 not implemented.

## Exit A

DEF-R2-02 is reproducible without GUI and verified fixed on probe; other phases can cite `report/DEF-R2-02.md` + `probes/A-trend-presets.json`. Desktop A3 still open.
