# Nestworth Insights QA — Round-3 Plan

| Field | Value |
|---|---|
| Base | `main` @ `367c3b0` (+ merge PR #13 docs when convenient) |
| Prior | Round-2 `docs/qa/analytics-linux-2026-09-09/` — reverify+deep PASS; **DEF-R2-02** open |
| Goal | Upgrade from “fix-reverify + P0 smoke” to **acceptance-grade** coverage for Insights |
| Out | Full timezone/DST matrix; macOS; real household data; product code fixes (unless asked) |
| Artifact root | `/workspace/nestworth-analytics-qa-r3/` |
| Docs target | `docs/qa/analytics-linux-2026-09-09-r3/` or `…-2026-09-10/` on a new branch |

---

## 0. Principles

1. **Seed + probe first**, desktop second — assert amounts/status in JSON; screenshots only for UX/i18n/deep-link.
2. **One narrative household**, four completeness variants — not four unrelated DBs with different stories.
3. **Pin known failures early** — DEF-R2-02 must be a reproducible automated case before adding scenery.
4. **Stay inside test-plan v1** — implement §6 events + FC-01–07 + incomplete matrix; do not invent product requirements.
5. **Ship as docs PR** — report + seed tooling changes if seed lives in-repo.

---

## 1. Phases (execution order)

### Phase A — Repro harness for DEF-R2-02 (½–1 day)
**Why first:** Round-2 found Trend **All** on missing-both → “Insights could not be loaded”; probe panics `fxRate unavailable`.

| ID | Work | Done when |
|---|---|---|
| A1 | Scripted probe: missing-both DB + range = History Origin → last closed (All) | Exit non-zero with **caught** error string (no raw panic), JSON artifact |
| A2 | Same for Trend presets: 30D / YTD / 1Y / 3Y / All / Custom mid-range | Table: preset → ok \| partial \| error |
| A3 | Desktop one-shot: Trend All screenshot + exact UI copy | PNG + note in RESULTS |
| A4 | File defect card `DEF-R2-02.md` (steps, expected vs actual, hypothesis labeled as hypothesis) | In report folder |

**Exit A:** DEF-R2-02 is reproducible without GUI; other phases can cite it.

---

### Phase B — Seed v3 “mini family” (1–2 days)
Extend `cmd/analytics-qa-seed` (fixture version `analytics-linux-qa-v3`).

#### B1 Household shape (align plan §6, keep AUD base for continuity unless we deliberately switch)
```
AUD Cash          bank / balance     AUD
US Brokerage      holdings           USD cash sleeve
SG Brokerage      holdings           SGD cash sleeve   (new)
(optional) Loan   liability          AUD               (Phase B2 if time)
```

Instruments:
```
AAPL  USD stock   (keep)
QQQ   USD stock   (new)
ES3   SGD stock/ETF (new)   — or simplest SGD-quoted instrument the domain allows
```

#### B2 Events to construct (map to plan §6.3 / FC-*)

| Event | Plan | Min assertion |
|---|---|---|
| Multi contribution / withdrawal | §6 | Capital moves visible in Dietz when Include Cash |
| Salary vs interest | FC-05 | Income attribution ≠ investment return |
| Internal transfer Cash↔Broker | FC-01 | Net worth ≈0 for transfer; not double-count return |
| Buy + partial sell + same-day buy/sell | FC-02/03 | Realized path CO-02 non-empty |
| Trade fee + bank fee | §6 | Fee in Investment Fee vs Categories Fees correctly split |
| Dividend (+ tax if API allows) | FC-06 | CO-03 + Drivers dividend |
| FX conversion (if supported) | §6 | FX Impact non-zero, no crash |
| Loan draw/repay/interest | FC-07 | Optional stretch; skip if domain friction high |
| True-zero day | RC-07 | amount=0 rate=0 |
| Deliberate residual corrupt day | 17.4 | Residual UI path (separate DB flag) |

#### B3 Completeness matrix (same story, four DBs)
| Scenario env | Incomplete behavior |
|---|---|
| `complete` | 0 incomplete days |
| `missing-price` | Gap day(s): omit equity quotes only |
| `missing-fx` | Gap day(s): omit FX only |
| `missing-both` | Gap day(s): omit both (regression for DEF-R2-02) |

Also: **short gap** (1 day) vs **long gap** (multi-day through window) as seed knobs or second missing-both variant.

**Exit B:** `NESTWORTH_QA_SCENARIO=…` builds all four DBs; `seed-results-*.json` OVERALL PASS; IDs documented.

---

### Phase C — Probe pack (1 day, parallelizable with B polish)

| Probe | Asserts |
|---|---|
| C1 Fixtures A–D | Still 4/4 on engine (no regression) |
| C2 IncludeCash FL-18/19 | Salary capital differs include vs exclude |
| C3 True-zero | Aug (or seeded) day 0/0 |
| C4 Scope triangulation | Portfolio vs Account(US) vs Instrument(AAPL) amounts reconcile |
| C5 Native vs Base | Single-currency instrument Native OK; mixed portfolio forced-base flag |
| C6 Return sources | Price+FX+Div+Fee sum vs period return (tolerance) |
| C7 Trend presets | Date bounds + error/partial status per scenario (ties to A) |
| C8 Transfer neutrality | FC-01: transfer day return ≈0 (investment basis) |
| C9 Residual | Corrupt DB → residualIssueCount>0; clean → 0 |

**Exit C:** `probes/SUMMARY.json` with pass/fail per ID; failures block “acceptance” claim.

---

### Phase D — Desktop acceptance (1 day)
Only after B+C green on complete; incomplete runs after A.

| Block | Cases | Screenshots |
|---|---|---|
| D1 Smoke SM-01–15 on v3 complete | Nav, tabs, sheets | thin set |
| D2 Scope switch Account / Instrument | FL-02/04/06/07 | 3–4 |
| D3 Include Cash toggle + Categories Income | FL-16–20 smoke | 2 |
| D4 Contribution Realized + Dividend + group-by | CO-02/03 + sort | 3 |
| D5 Drivers waterfall + Residual (if corrupt DB) | 13.x / 17.4 | 2–3 |
| D6 History deep-link field check | from/to/account/instrument/kinds | annotate |
| D7 Incomplete matrix UX |◇ / banners / zh-CN; **Trend All** on each scenario | per scenario |
| D8 Soft invalidation | edit quote or add activity → back to Insights | 1–2 |

**Exit D:** `screenshots/**/RESULTS.json`; no unexplained crash; DEF-R2-02 status updated (fixed / still fail / accepted).

---

### Phase E — Report + PR (½ day)
- Full `REPORT.md` (env, case tables, defects, exit criteria, screenshot index)
- `STATUS.md` one-pager
- Branch `qa/analytics-linux-r3-YYYY-MM-DD` + PR
- Memory: Round-3 verdict

**Exit E:** PR open; verdict explicit (acceptance met / not met + why).

---

## 2. Explicit non-goals (this round)

- Fixing DEF-R2-02 product code (separate ask → cloud agent / 斧正)
- Full DST / multi-timezone matrix (note as Round-4)
- 1440/responsive deep visual
- Replacing Round-2 docs; R3 is additive

---

## 3. Exit criteria (Round-3 “acceptance-grade”)

**Met only if all true:**
1. Four scenario DBs build cleanly from one seed story  
2. Probe pack C1–C9 has no unexpected FAIL (DEF-R2-02 may be **known FAIL** if product unfixed — then acceptance = “documented blocker”, not green)  
3. Desktop D1–D8 executed with RESULTS  
4. Scope / transfer / multi-instrument at least one quantitative probe each  
5. Report published on branch/PR  

**Green acceptance** additionally requires DEF-R2-02 fixed or product-owner waiver.

---

## 4. Suggested calendar

| Day | Focus |
|---|---|
| Day 1 | Phase A + start B1/B2 seed |
| Day 2 | Finish B3 matrix + Phase C probes |
| Day 3 | Phase D desktop + Phase E report/PR |

Slip: cut Loan (FC-07) and long-gap variant before cutting Scope/Transfer/DEF-R2-02.

---

## 5. Immediate next action (when you say go)

1. Implement Phase A probe harness against existing `nestworth-missing-both.db`  
2. Spike seed v3 account/instrument creates on a branch of the seed tool  
3. Do **not** expand desktop until A+B complete DB exists  

---

## 6. Decision log (defaults I’m taking)

- Keep **AUD** base (continuity with R1/R2); CNY from plan §6 is documentation ideal, not blocking  
- SG instrument = simplest SGD-quoted type the API accepts  
- Loan = stretch goal  
- Product fix for DEF-R2-02 = out of scope unless you order it  
