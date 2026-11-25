# Gio and Wails capability and layout checklist

This is the Stage 1–6 checklist for the opt-in Gio preview. Implemented cells
describe runnable Gio behavior. Evidence cells link to the fixture and platform
records in the [roadmap](roadmap.md). Those records distinguish shared Go
comparison, Gio interaction tests, Linux X11, and native Mac interaction.
Each gap remains open until its workflow has implementation and verification.

Compare the Wails page title, navigation, section order, primary actions,
compact layout and feedback. A deliberate layout departure needs a reason and
its own evidence. Native Mac, VoiceOver and IME acceptance apply across the
in-scope pages. Earlier Mac runs cover the individual workflows named in the
roadmap.

Chart development and integration are paused pending separately coordinated
shared components. The accessible tables cover the available data paths.
Chart rendering and interactions remain open.

| Wails entry and stage | Implemented in Gio and layout | Verified evidence | Open capabilities and layout |
| --- | --- | --- | --- |
| [Shell and navigation](../../../frontend/src/app/navigation.ts) (2) | Sidebar groups, Back, Forward and Recent above page content. Wide sidebar, narrow rail and expanded narrow sidebar. Localized control names and shared window size restore. Visits retain supported filters, drafts, position and focus. | [Navigation and directory](roadmap.md#navigation-and-directory-object-milestone) · [Recent and icon navigation](roadmap.md#institution-icons-and-recent-milestone) | Compare placement of attention counts, breadcrumbs, actions and detail contexts. Verify retained keyboard focus and scroll for every new route. |
| [Blocked startup](../../../frontend/src/features/startup/BlockedStartupPage.tsx) and [Onboarding](../../../frontend/src/features/onboarding/OnboardingPage.tsx) (3) | Startup gate precedes workspace. Verified synthetic profile enforcement, occupied and blocked states, isolated recovery and onboarding fields. Onboarding confirms persisted household and enters Accounts. | [Synthetic startup](roadmap.md#synthetic-startup-and-ledger-ownership-milestone) · [Household onboarding](roadmap.md#household-onboarding-milestone) · [Recovery](roadmap.md#disposable-startup-recovery-milestone) | Compare remaining blocked-state paths, localized errors and native quit/reopen. Real profile selection, migration and production recovery remain gated. |
| [Overview](../../../frontend/src/features/overview) (3) | Landing page displays current valuation, base currency, read time, net worth, assets, liabilities and evidence. Empty, partial, unavailable, refresh and retry states. | [Live Overview](roadmap.md#live-overview-milestone) | Compare Wails allocation and detail navigation, history and comparison entry points, and their positions at compact widths. Add linked supporting workflows and verify retained context. |
| [Accounts](../../../frontend/src/features/accounts/AccountsPage.tsx) (3–5) | Institution-first list and account detail. Catalog creation, maintenance, ownership, archive and restore. Scoped balance reconciliation and Holdings trade/cash actions. Review and retry through the shared operations. | [Accounts and Catalog](roadmap.md#stage-3-progress) · [Ledger write gates](roadmap.md#stage-4-exit-review) · [Investments](roadmap.md#stage-5-review-checkpoint) | Match Wails filters, account cards and action grouping for unimplemented actions. Add backdated/manual value changes, debt actions, card purchase/refund, account Starting Point and broader Holdings operations. |
| [Ledger activity](../../../frontend/src/features/history/HistoryPage.tsx) (1, 4) | Activity list, account/instrument/type/date filters, keyset paging, origin and details. Scoped posting, repeat, correction and reversal. Shows immutable linked outcomes and maintains supported visits and retry. | [Ledger activity](roadmap.md#stage-1-acceptance) · [History filters and activity detail](roadmap.md#history-filters-and-activity-detail-milestone) · [Stage 4 exit review](roadmap.md#stage-4-exit-review) | Date presets, supported endpoint edits, repeat list entry, more posting types, additional calculators, product actions and linked adjustments. Compare action placement and compact sheet behavior. |
| [Directory](../../../frontend/src/features/directory/DirectoryPage.tsx) (2) | Members, Institutions and Groups tabs with search, archive filtering, editor and detail page. Create/edit/archive/restore guardrails, inline institution creation, and linked accounts. Back retains tab, filter, and focus. | [Directory and General settings](roadmap.md#stage-2-progress) | Verify compact layouts and keyboard/IME for search, icon and edit flows. Compare Wails list overflow, compact actions and object-details presentation. |
| [Settings](../../../frontend/src/features/settings/SettingsPage.tsx) (2, 6) | Tabbed preferences, appearance/language/theme, date and number formats, market data preferences and available diagnostics. Shared draft, explicit save/discard/reset, key-specific save, durable reopen. | [General settings](roadmap.md#stage-2-progress) · [Current Gio behavior](../README.md#settings-and-sidebar) | Compare order and responsive placement for in-scope controls. Provider coverage and operations belong to Market data and Data health. Advanced integrations are paused under Stage 7. |
| [Investment assets](../../../frontend/src/features/portfolio/PortfolioPage.tsx) and [Instrument detail](../../../frontend/src/features/investments/InstrumentDetailPage.tsx) (5) | Holdings list and linked instrument details, shared value/cost, missing evidence, and supported buys, sales, liquidation and dividends from account details. Empty/error/reload and retained search/focus. | [Holdings and instrument detail](roadmap.md#holdings-and-instrument-detail) · [Stage 5 review](roadmap.md#stage-5-review-checkpoint) | Portfolio grouping and allocation, instrument catalog and maintenance, performance, import and multi-position opening, cross-currency and additional trade and dividend actions. Compare section order and compact detail navigation. |
| [Available funds](../../../frontend/src/features/liquidity/AvailableFundsPage.tsx) (5) | Today view with coverage and source state, standard bank reservations and release, and term deposit terms, maturity receipt and linked History. Confirmed receipt and reservation changes survive reopen. | [Read-only Available Funds](roadmap.md#read-only-available-funds) · [Term deposit receipt](roadmap.md#confirm-actual-term-deposit-receipt) | Additional horizons, policy and reservation editing across source types, new/existing product setup, early redemption, renewals, standalone interest and other managed products. Compare horizon and action placement and product detail states. |
| [Tasks and checks](../../../frontend/src/features/tasks/TasksPage.tsx) (5) | Upcoming, Investment review and Reconciliation tabs. Term deposit schedules, published pending review, and eligible open card sessions link to the selected records. Back restores tab, list and links. | [Term deposit Tasks](roadmap.md#term-deposit-tasks) · [Investment and reconciliation Tasks](roadmap.md#investment-review-and-card-reconciliation-tasks) | Other upcoming actions and reminders, coverage/data checks, review rules and lifecycle, multi-account reconciliation and exceptions. Compare badges, tab summary, and navigation across the complete workflows. |
| [Wealth history](../../../frontend/src/features/historical-overview/HistoricalOverviewPage.tsx) (6) | Date controls precede the bounded daily table, using the household base currency and History timezone. Partial and provisional results, coverage, retry, and retained capture. The initial table is a deliberate keyboard-first presentation of the Wails Trend data. | [Read-only Wealth History](roadmap.md#read-only-wealth-history) | Match Wails capture controls and Allocation, Change, Accounts and Trend layout. Add charts and endpoint evidence/drilldown. The current table alone leaves these Wails entry points open. |
| [Return analysis](../../../frontend/src/features/insights/ReturnAnalysisPage.tsx) (6) | Title, Calendar/Trend/Contribution tabs, then a shared filter card with scope, valuation, dates, cash toggle and additional account/currency filters. Calendar has month controls, summary, week grid, contributors and a right-hand day detail. Trend and Contribution use keyboard-scrollable lists for their first read-only views. Month controls stop at the start month and the current month so the synthetic preview browses its recorded period. | [Return Calendar](roadmap.md#read-only-return-analysis-calendar) · [Shared filters and projections](roadmap.md#return-analysis-filters-trend-and-contribution) | Year view, date shortcuts, additional instrument/member/asset-class filters, linked-rate/chart parity, Contribution ranking/search/details and context links. See the concrete cross-page checklist below. |
| [Asset changes](../../../frontend/src/features/insights/AssetChangesPage.tsx) (6) | Title, Asset trend/Change drivers/Categories tabs and the shared filter card. Change drivers shows the value summary and grouped amounts. Asset trend and Categories expose selected shared reads in keyboard-scrollable lists. The Calendar day link opens Change drivers with the selected day and supported Return filters. Back restores the calendar and day focus. | [Asset Changes and Return handoff](roadmap.md#asset-changes-and-the-return-day-link) | Additional metrics, driver and category drill-down, detailed missing evidence, and links from those details to History and Return Analysis. Charts are intentionally paused. |
| [Market data](../../../frontend/src/features/marketdata/MarketDataPage.tsx) and [Data health](../../../frontend/src/features/data-health/DataHealthPage.tsx) (6) | Provider preferences exist in Settings. Operational pages are pending. | Pending | Implement sources, coverage, explicit provider refresh, progress/failure and recoverability, manual/Agent evidence, and Data Health repair/verification. Match Wails filters and navigation, action placement and compact layout. |

## Return Analysis and related navigation

The reference [Wails interaction regression](../../../frontend/src/features/insights/ReturnAnalysisPage.test.tsx)
covers choosing a historical month, retaining the effective period, and
carrying instrument, account, currency, native valuation and cash options.
Its tab-switch scenario moves from September to August 2026, passes
`2026-08-01` through `2026-08-31` to Trend or Contribution, then returns to
August Calendar with the same selection.

- [x] Open Return Analysis and place month navigation above the calendar
  summary, grid and contributors. Open and close the right-hand day detail.
- [x] Align page title → Calendar / Trend / Contribution tabs → shared
  filters → active content. Compare compact wrapping and keyboard traversal.
- [x] Set a historical month, instrument scope, additional account and
  currency filters, native valuation and cash inclusion. Switch Calendar,
  Trend and Contribution. Verify the effective closed range and the same
  filter values through the owning Go request and Wails reference.
- [x] From a calendar day, open Asset Changes for that day with the
  supported scope, valuation, cash and additional filters. Back restores
  the originating month, active filters, scroll and day focus.
- [ ] From a contribution detail, open Ledger activity with the supported
  dates, account, instrument and kinds. Explain any unsupported filter
  such as currency or asset class. Back restores Contribution, its selection
  and focus.

## Paused Stage 7

Advanced integrations, backup/restore/export, mobile pairing, MCP, the
built-in assistant, native integrations and distribution remain paused.
Synthetic startup and recovery evidence belongs to Stage 3. Activation on a
real ledger, production startup ownership, distribution and default-shell
cutover retain separate gates.
