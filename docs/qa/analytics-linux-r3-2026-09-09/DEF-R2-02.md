# DEF-R2-02 — Insights load fail / panic on incomplete Trend **All**

| Field | Value |
|---|---|
| ID | **DEF-R2-02** |
| Severity | P1 (Round-2 release hold) |
| Found | Round-2 desktop + engine path (missing-both + Return Trend **All**) |
| Status | **FIXED** (probe reverify PASS) — **pending desktop confirm** (Phase A3 / D7) |
| Repo / rev | `/workspace/NestworthGo-fe-be-audit` branch `qa/analytics-linux-r3-2026-09-09` @ `ea94f48` (classifier soft-FX path present; sibling test commit `85f7f8b`) |
| Artifact root | `/workspace/nestworth-analytics-qa-r3/` |

---

## Summary

On the Round-2 **missing-both** fixture, selecting Return Trend preset **All** (History Origin → last closed day) surfaced a generic UI error **"Insights could not be loaded"** and the legacy analytics probe could panic with an FX-rate / valuation conversion failure. Product fix softens missing FX conversion in `convertValuationAmount` so a partial period can still load.

---

## Steps to reproduce (historical)

1. Seed / use DB `data/nestworth-missing-both.db` (incomplete FX + price gaps).
2. Open Insights → Return Trend.
3. Select preset **All** (range ≈ origin `2026-07-26` → last closed `2026-09-08`).
4. **Before fix:** UI hard-fails with generic load error; probe could panic (`fxRate unavailable` / hard error from valuation conversion).
5. **After fix:** analysis returns with coverage **partial** (no panic).

Automated repro (no GUI):

```bash
env -u NESTWORTH_PROBE_MODE \
  NESTWORTH_DATABASE_PATH=/workspace/nestworth-analytics-qa-r3/data/nestworth-missing-both.db \
  go run ./cmd/analytics-qa-probe
# (omit NESTWORTH_PROBE_FROM/TO → legacy All = origin → MAX(local_date))
```

---

## Expected vs actual

| | Before (Round-2) | After (Round-3 Phase A probe) |
|---|---|---|
| Expected | Partial coverage / incomplete UX; Insights remains usable | Same |
| Actual | Generic **"Insights could not be loaded"**; probe panic risk on FX | Probe **PASS**: exit 0, `panicked=false`, coverage **partial 21/45**, no panic |
| UI | Still needs desktop one-shot (A3) | **Pending desktop confirm** |

---

## Root cause / fix (product)

**Fix location:** `internal/application/analysis_classifier.go` — `convertValuationAmount`.

Missing FX rate on a **base-valuation** foreign cash amount is treated as **unknown** (`known=false`, no error) so a partial period can still load instead of failing the whole analysis.

```go
// convertValuationAmount … A missing FX rate on a base-valuation foreign amount is
// treated as unknown so a partial period can still load.
```

Cited lineage: present at `ea94f48`; dedicated classifier coverage commit `85f7f8b` (`test(analytics): cover return analysis classification`).

---

## Probe reverify (Round-3 Phase A)

| Evidence | Result |
|---|---|
| `logs/03-probe-missing-both-all.out` | `coverage rated=21 total=45 status=partial from=2026-07-26 to=2026-09-08`; contribution types load (partial); **no panic** |
| `logs/03-probe-missing-both-all.err` | empty |
| `logs/03-probe-exit.txt` | `PROBE_EXIT:0` |
| `logs/03-probe-summary.json` | `"panicked": false`, `"exitCode": 0`, coverage partial 21/45 |
| Re-run (A-trend `mb-all`) | Same: exit 0, partial 21/45, no panic |

**Verdict:** engine/API path for DEF-R2-02 is **FIXED** on probe. Desktop Trend **All** screenshot + exact UI copy remain **pending** before calling the defect fully closed in RESULTS.

---

## Related

- Round-2 open note: `docs/qa/analytics-linux-2026-09-09/REPORT.md` §9 / STATUS hold on DEF-R2-02.
- Trend preset matrix (All / Custom mid / 30D-ish × missing-both + complete): `probes/A-trend-presets.json`.
- Phase A summary: `logs/A-phase-summary.md`.

## Desktop reverify (2026-09-09)
**FIXED** — Trend All loads Partial coverage 21/45; 30D Partial 21/30; no generic load error.
Evidence: `screenshots/def-r2-02/RESULTS.json`, `01-trend-all.png`, `02-trend-30d-or-mid.png`.
