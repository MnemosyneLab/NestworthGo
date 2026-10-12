# Historical panorama P1 acceptance

Owner: Historical panorama engineering and QA. Status: automated implementation
checks passed; independent review and native acceptance pending.

Scope: category comparison, valuation evidence and navigation context. Based on
main `0c68ad47c59e460e5fb14cf11a6b5eb1d317b8f1` after P0 PR #54.
Use isolated synthetic profiles only. No test or acceptance step may read or
write a real user's financial profile.

## Independent review matrix

| Area | Expected behavior | Automated evidence | Native review |
| --- | --- | --- | --- |
| Categories | Go B−A amounts, backend share percentage points; assets/debt separate | `historical_categories_test.go`, `HistoricalCategoryComparison.test.tsx` | Cross-check cards, tooltip and table against captured values |
| Unknown/absent/scope | No missing category zero-fill, no partial aggregate delta; reasons visible | Same-category different missing sets, one-side absence, exclusion, inclusion changes, same-date unknown | Inspect both sides and retained affected leaves |
| Decimal/denominator | Very small values remain exact; zero/negative denominator suppresses share comparison | Tiny amounts, negative debt/assets, zero with positive other category, exact sorting/geometry regressions | Long/tiny values at wide/narrow/zoom; signs visible |
| Coverage | Asset and liability denominator completeness independent; partial shares say known subtotal | Role/category coverage and P0 missing-asset leaf regressions | Missing FX only in debt must not downgrade asset composition |
| Evidence | Selected price/rate, observation ID, source key, effective/market date and stored basis; inverse FX uses divide | Frozen current/replay, unretained observation, explicit holding IDs, concrete missing price/FX tests | No universal close label; missing provenance remains absent |
| Internal drill-down | Category→accounts→parent/child detail; return restores role/dimension/position/focus | Category-return page test, duplicate-key chart tests, shared Sheet tests | Mouse plus Enter/Space/Escape/Close, narrow account cards |
| External return | Explicit identity/date filters; current range visibly ends at last closed day; frozen bundle survives Back | UTC/Asia-Shanghai fake-clock month-boundary App trail regression and reverse-range page test | Leave to History then Back; dates/filter/detail/scroll restored |
| Capture/race/error | Same-batch local reads; no provider/snapshot writes, latest selection wins, manual refresh atomic | Query-only SQLite and existing lifecycle/race/date/refresh suites | Offline reading, rapid date/refresh, failure/retry, exit/re-enter |
| Locale/theme | en/zh-CN/zh-TW, light/dark, shared chart theme | Locale fixtures and P0 UI/theme/chart regressions | Contrast, long labels and percentage-point wording |

## Linux native gate

On a GTK4/WebKitGTK capable host, use the reviewed immutable head and a new
synthetic database/settings profile. Test complete, missing-price, missing-FX,
negative, zero, one-side absent, archived/excluded, tiny-value and debt profiles.
Record commit, profile, locale, theme, content width and zoom for each screenshot.

1. At wide desktop, about 375px and 320px content widths, compare A/B categories
   in assets and liabilities, then both dimensions. Check wrapping, shared axis,
   missing slots, source table, amount signs and full currency/percentage points.
2. At 200% native zoom, open detail before any corrective pan. Verify immediate
   fit, visible Close, body scroll, Escape and returned trigger focus. Check a
   regular editable Sheet separately for shared-component regression.
3. Navigate category→account→child detail→History→Back. Check dates, dimension,
   filters, frozen capture timestamp, selected detail and prior scroll position.
   Close detail and return to categories; verify keyboard focus and position.
4. Test reverse dates, same-date complete/unknown, current capture, rapid
   selection, failed refresh/retry and offline reads. Confirm old financial
   values never appear under new dates and current only changes on explicit read.
5. Repeat labels/evidence paths in English and both Chinese locales, light/dark
   and a second accent. DOM tests do not establish native visual acceptance.

## Execution record

Automated results and environment limits are recorded in the PR description.
Native screenshots, Linux/macOS GUI acceptance and user-task studies are pending;
no build or component test is evidence that these manual gates passed.
