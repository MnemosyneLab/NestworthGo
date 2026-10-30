# Localization review — 2026-10-30

Baseline: `3a1e7d9007bec8cf84a85d8dbf796cc9348f2dcd` (includes #179 and #180).
Scope: shipped frontend English, Simplified Chinese and Traditional Chinese.

## Results

This review confirms **14 Traditional Chinese string defects** (0.41% of the
3,393 effective interface keys) caused by two copied destination names, plus
**one missing key in all three languages**. This is a repeated naming defect
across several screens, not evidence of widespread translation failure.
All 15 distinct affected keys are fixed here. Financial behavior is unchanged.

| Finding | Severity | Owning resource and affected keys | Visible consumers |
| --- | --- | --- | --- |
| Simplified destination names in Traditional Chinese: 9 keys | Low | `frontend/src/i18n/locales/zh-TW.json`: `nav.history`, `nav.historicalOverview`; `historicalOverview.{title,loading,error,openHistory,leaveCaptureNote,historyClosedRangeNote,noOriginHint}` | `app/navigation.ts`, `App.tsx`, Overview button, HistoricalOverviewPage title/loading/error/empty states and history handoff |
| Same copied history name: 5 keys | Low | `frontend/src/i18n/additions.ts`, `additionsZhTW`: `history.description`, `overview.openHistory`, `insights.{viewInHistory,historyRangeWider,historyContributionFilterWider}` | HistoryPage intro/toast, Overview actions, ContributionTab, CategoriesTab, ChangeDriversTab |
| Missing `history.unitCost`: 1 key × 3 languages | Medium | `frontend/src/i18n/additions.ts`, all three `history` trees | `features/history/ActivityDetailSheet.tsx:effectLine`: `holding_cost` effects on cost adjustments and reversals displayed the raw key. Supported persisted activity/effect kinds are defined in `internal/domain/change.go`. |
| Coverage guard did not match production | Test gap | `frontend/src/i18n/localeCoverage.test.ts` reconstructed a different merge, omitted `investmentAssets`, and merged error codes into the translation namespace | Fixed to inspect the actual initialized resource bundles, including empty values, field-label targets, plural forms and interpolation alignment |

The two reviewed names are now consistently `帳務紀錄` and `資產回溯` in
Traditional Chinese. Changes were semantically reviewed rather than automatically
transliterated.

## Inventory and traceable coverage

`frontend/src/i18n/index.ts` loads JSON, then additions (including
historicalReconciliation and availableFunds), rebuildData, upcomingActions,
homepage, investmentAssets, and tasks, in that order. `errorCode` is a separate
namespace. Baseline resources contain **3,393 translation + 51 error-code leaves
per language**; after adding unitCost, **3,394 + 51**. Effective key sets, empty
strings, interpolation placeholders and English plural pairs pass. Legacy printf
placeholder types/order also match across all locales. Nested `$t` targets exist.

The source inventory inspected 254 production TS/TSX files: **3,025 literal
translation calls and 303 nonliteral calls**. The static literal-key scan found
only unitCost missing. Its regression now checks each language's own resources,
without fallback. A supplemental scan of dotted key literals found only that
missing key and two nontranslation Agent event names.

| Surface | Literal / nonliteral calls inventoried | Inspected paths and behavior |
| --- | --- | --- |
| Navigation | 18 / 5 (App + app) | NAV_GROUPS, App breadcrumbs, AppShell language/appearance options; actual navigation keys now have a real-resource regression |
| Overview | 95 / 4 | OverviewPage, NetWorthTrendCard; current/partial/error states and history actions |
| Ledger/history | 290 / 22 | HistoryPage, ActivityDetailSheet, activitySentence, productActivityText, RecordChangeForm; effect directions, kind/reason maps, liability sentences, unknown-name fallbacks |
| Account forms | 262 / 12 | AccountCreateWizard, AccountForm, AccountActionSheets; tracking-mode keys, preview and submission recovery text |
| Holdings/instruments | 246 / 18 | InstrumentForm, HoldingGroupsList/Header, PricePerformancePanel; later investmentAssets overrides, group/type keys, quote-kind and unavailable fallbacks |
| Reconciliation | 202 / 39 | ReconciliationPage, LedgerCheckpoint, product correction and recovery views; status/outcome/kind keys and unknown-state fallback |
| Settings | 197 / 31 | SettingsPage, AgentSection, backup/retention/secret sections; option mapping, save/reset/error states and backend-state fallback |
| Historical overview | 282 / 47 | HistoricalOverviewPage, HistoricalPanorama/Trend/CategoryComparison/Reconciliation; loading/error/empty and history navigation |
| Other shipped features/shared code | 1,433 / 125 | Insights, liquidity, market data, investment review, tasks, upcoming actions, onboarding, startup, common controls and formatting/errors |

These are call-site counts, not counts of unique keys or executed GUI paths.
Conditional literals and finite mapping keys were cross-checked against resources;
open-ended backend keys were reviewed for unknown/default behavior.
303 nonliteral expressions are **not** a proof that every possible backend value
was dynamically executed.

## Exclusions and limits

- Of 376 baseline CN/TW-identical values containing Han text, 6 were among the
  confirmed defects above; 370 were legitimate shared text, such as `股票`,
  `日期`, `分析`, market names and numeric templates. Equality alone is not a bug.
- English-identical values such as Nestworth, Yahoo Finance, ETF, ISIN,
  Cloudflare credential field names, date-format patterns, currency/unit symbols
  and interpolation-only templates are intentional technical/proper-name text.
- A JSX text/accessibility-attribute inventory found shared A/B comparison
  markers, brand names and financial units, not additional confirmed hardcoded
  sentences. Computed strings and server-supplied messages remain a coverage limit.
- `insights.placeholder` and `insights.notAvailableYet` are unused legacy copy:
  no production reference, and inspected dynamic Insights label maps do not select
  them. They are not counted as visible defects and are retained; other keys with
  no literal caller are not declared dead because dynamic callers may own them.
- English is the configured fallback. The two copied names were present in the
  Traditional Chinese resources, so fallback did not cause them. Existing fallback
  architecture is unchanged. Two inspected raw-code fallback paths (`ReturnTrendTab.sourceLabel` and
  `ProductPreview` activity kind) remain unverified candidates for unsupported
  future values, not confirmed current defects. No exhaustive native GUI, layout, or linguistic certification
  is claimed.

## Validation

Real-resource regressions were run before the fixes: the destination-name guard
failed, cost-adjustment rendering failed in all three languages and the new
literal-call guard reported exactly three missing unitCost translations. After
fixing resources, the affected i18n/history checks passed (5 files, 40 tests).
The pre-existing navigation test asserted the incorrect Simplified names for
Traditional Chinese; its three TW expectations now assert the corrected names,
while CN assertions remain unchanged. Full frontend validation results are recorded
in the PR. All fixtures are synthetic.
Native macOS GUI acceptance and packaging are not performed by this Linux task.
