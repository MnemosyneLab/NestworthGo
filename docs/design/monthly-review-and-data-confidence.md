# Monthly Review and Data Confidence

- Owner: Product (scope and real-use acceptance); implementation spans application, persistence, Wails, and frontend
- Status: **Planned**. This document is not an implementation claim.
- Prerequisite: Insights (Return Analysis and Asset Changes) is in use, correctness regressions stay covered by tests, and the named desktop gates on the current release line are closed or explicitly accepted
- Scope: a personal, local-first monthly maintenance loop. It does not add multi-user, cloud, or commercial requirements.
- Companions: [Analytics architecture](analytics/analytics-redesign-architecture.md), [Backup, restore, and CSV](backup-restore-and-csv-portability.md), [Product vision](../product/product-vision.md)

Insights answers what happened. This increment helps the user finish a trustworthy maintenance pass:

> I reviewed the important assets for this period, I know which figures are still unreliable, and I know which conclusions to revisit after I backfill history.

The first version is not a mandatory month-end task, not daily bookkeeping, and not a budget or accounting close. The user may review the latest closed day mid-month. Dates are History Origin local dates, not the browser timezone or a fixed 24-hour day.

## 1. What to add

Reuse the analysis kernel, History Origin / closed-day snapshots, existing Preview/Record/Fix paths, backup status, and create-only Accounts/Holdings CSV. Do not add a second financial engine. Bank-statement matching is a later, separate plan.

New capabilities:

1. Surface data issues in one place, with concrete next actions.
2. Reconcile cash balances, holding quantities, and manual valuations against the same as-of date.
3. Save the review range, results, and open items.
4. After related history changes, show that the saved review basis is stale.

“No missing quotes”, “the user confirmed balances”, and “a backup verified when it was created” are three different kinds of evidence. The page must not collapse them into one green “all data is correct”.

## 2. Entry, period, and layout

First version: an Overview action such as “Review assets” opens a dedicated page. Do not add a permanent sidebar destination. The user can leave and return; resolving issues must not require staying in a wizard.

- Default to the last complete calendar month. If that month has no history, use the coverable range and say so.
- The as-of date must not be after the last closed day in the History Origin timezone.
- The range start must not precede History Origin. The first recorded month is labelled a partial month.
- A range with no history coverage explains itself; it must not invent zero balances or an empty “review succeeded”.

Three independent regions:

| Region | Contents |
| --- | --- |
| Data readiness | History coverage, snapshot state, missing quotes/FX, stale valuations, unexplained residuals |
| Account review | As-of balances/holdings, confirmed state, differences, and next actions |
| Period conclusion | Links into Asset Changes and Return Analysis, open items, save, latest backup evidence |

Opening the page reads local data only. It must not auto-refresh market providers, invent activities, create a backup, or spawn background jobs from browsing. If a reused analysis call maintains snapshots, separate a light check from a user-started rebuild so implicit writes are not hidden behind a “read-only check”.

## 3. Data readiness

Organize issues by stable type, date range, and account or instrument identity. Distinguish “affects calculation”, “needs the user to confirm”, and “maintenance reminder”. Do not invent a composite health score.

| Issue | What the user sees | Next step |
| --- | --- | --- |
| Range starts before History Origin | Which dates are covered | Adjust the range |
| Snapshots pending | Which dates need rebuild | Start or continue the existing rebuild, with progress and failure |
| Missing historical quote or FX | Which instrument or currency and which dates | Open market history or a manual quote |
| Current price is stale | Quote time and source | Explicit refresh or inspect the source |
| Manual asset not valued recently | Last valuation date | Update valuation, or keep as an open item |
| Residual beyond tolerance | Which component and days | Open Drivers detail and related History |
| No recent backup record | Missing backup evidence only | Open the existing backup flow |

Rules:

- Refreshing today’s quotes does not repair last month’s missing history. Live and historical sources stay distinct.
- Historical prices are not historical holdings. Do not backfill current quantities before History Origin to draw a curve.
- Manual property valuations, periodically updated balances, and exchange quotes have different freshness. Do not apply one 24-hour threshold to every asset. Reminder thresholds do not change financial calculations.
- Residual uses the existing component/day tolerance. Offsetting residuals still keep an issue entry.
- Navigation must not mark an issue fixed. Only a fresh authoritative read can clear it.

## 4. Balance and holding comparison

The user compares Nestworth with a bank, broker, or personal record and confirms the as-of values. The first version is manual; it does not require uploaded statements.

| Object | Compare | Do not confuse |
| --- | --- | --- |
| Single-currency cash | Native as-of balance | Base-currency amounts hiding a native difference |
| Multi-currency cash | Each currency’s native balance | Summing currencies then declaring a match |
| Holdings | Quantity of the same instrument; cost basis only as extra context | Correct quantity meaning correct market value, cost, or return |
| Manual valuation | Amount, currency, valuation date | A self-confirmed valuation is not an exchange print |
| Liability | Outstanding amount on the same as-of date | Ledger sign conventions versus signed net-worth contribution |

System and external values must share date, currency, and object. The latest live balance is not an as-of month-end check. Historical quantity comes from ledger replay or another existing authority, never “historical market value divided by price” in the React app.

Comparisons use exact decimals under the money, native-amount, and quantity contracts. Display rounding is not equality. If a small difference is allowed, name that rule; do not reuse analysis residual tolerance to auto-pass a balance review.

On a difference, the first version offers: inspect History, record or fix an activity through existing preview, open reconciliation/adjustment preview, or leave the item open with a reason.

**A difference is never auto-posted as income, return, or Adjustments.** An unexplained residual is not a generated journal amount. Saving a review does not create or mutate activities. User-confirmed financial corrections still follow History mutability rules and must not bypass Starting Point or immutable activity rules.

## 5. Period conclusion and save

Reuse Asset Changes and Return Analysis figures for beginning, ending, net change, and main drivers, with links to those pages and to the calculation vocabulary. Distinguish net-worth change, period investment return, current unrealized gain, and cash flow. State scope, currency, as-of date, and coverage. A partial month or saved open items must not be worded as a complete, fully confirmed month. This increment does not add six new charts or recompute rates.

A saved review records range, as-of date, items, open issues, notes, and basis version. “Saved with open items” and “saved with none” stay distinct. This is not a period lock. Later backfills and corrections remain allowed; old review rows stay; current validity is recomputed.

## 6. Review records and later changes

Store a small review aggregate in the business database so backup/restore keeps it. Do not keep it only in frontend `localStorage` or a sidecar.

Conceptual fields (names and schema version are chosen at implementation time):

| Record | Contents |
| --- | --- |
| Review | Household, period, history timezone, scope, valuation, created/saved at, notes, superseded review id |
| Review item | Account/holding/currency, as-of date, kind, authoritative value, optional user reference, result, notes |
| Open issue | Stable type, object/date, status at save, user reason |
| Review basis | Calculation-contract version, signatures of relied-on inputs, minimal source identities |

Saved versions are append-only. Drafts may update. The first version stores no statement attachments or institution file paths. A review is not a second balance sheet and does not enter net worth. Saved amounts are evidence; the ledger and valuation path remain authoritative.

Two status dimensions, not one conflicting enum:

| Dimension | States |
| --- | --- |
| Progress | Draft / saved with open items / saved with none |
| Basis validity | Matches current data / related inputs changed, re-check / temporarily unverifiable |

“Related inputs changed” means the old conclusion needs a look, not that every total moved. “Temporarily unverifiable” (for example a read failure) must not auto-become valid and must not delete the saved row.

Validity must survive process restart; do not reuse in-process `analysisDataGeneration`. On open and on save, compute an input signature for that review range: activities that affect beginning or the period (including correction chains, not only new rows this month); snapshots and their revisions; valuation and FX evidence; account, instrument, ownership, and History Origin metadata; calculation-contract version. Offsetting corrections still change the basis. Theme, window size, and unrelated display settings must not. Ordinary activity after the period that does not affect history must not invalidate every older month.

Do not hash the whole database file. Reads use a consistent view. On save, re-check the basis inside existing write coordination and commit atomically. If inputs moved, require a refresh rather than confirming a concurrent correction. The first version checks on open; no background poll of every month. Later caching may use persisted revision or dirty ranges, not by omitting inputs.

## 7. Backup evidence

Reuse `LastBackupStatus` and the existing backup actions. Show recorded time, filename, and verification at creation. That record is historical: it does not prove the file still exists or that it contains the review just saved. Missing or old backup offers an action; it does not block saving a review. “Backup now” after save is explicit. Once review metadata is in the database, whole-database backup includes it. Upgrade, restore of older backups, and validity re-check after restore are in scope. Restore drills use isolated paths, not the real household database, and are not a monthly chore.

## 8. Implementation constraints

- Add narrow application and Wails reads. React owns interaction and does not copy valuation or return formulas.
- Overview reads are bounded batches, not one full-period analysis replay per account.
- Snapshot rebuild keeps the existing 31-day batching, partial/failure/resume behaviour, and no provider HTTP under a long write lock.
- Schema migration follows the existing versioned path, adds only required storage, and leaves existing data readable.
- Older-schema backup compatibility stays on the existing recovery contract; empty-database tests of new review tables are not enough.
- New interaction includes loading, empty, error, and unavailable states, the three locales, and keyboard use.

A useful first slice is the data-readiness list. Account comparison reduces work. Saved versions with basis validity are what make the record trustworthy over time. Product acceptance is a real household month: the user can review a period, keep open items, and later see that related data changed.

## 9. Acceptance scenarios

A spanning fixture includes two currencies, cash, the same instrument in two accounts, a loan, and a manual valuation, plus salary, spending, internal transfer, trades, dividend, loan repayment, and a noon in-kind transfer. Expected net worth, cash, quantities, return, and capital reuse the analysis math; this document does not define new return formulas.

| Scenario | Must prove |
| --- | --- |
| History starts mid-month | Review only the coverable range; unknown early balances are not zero |
| Missing history quotes, live quotes present | Refreshing current prices does not claim historical gaps are fixed |
| Fully missing, partially missing, and true zero | Distinguishable in comparison, analysis, and review status |
| Same cash, different holding quantity | Matching total market value is not a complete review |
| Two currencies in one account | Native comparison per currency, no cross-currency offset |
| Offsetting residuals | Issue entry remains; zero net residual is not auto-closed |
| Backfill last month or an earlier beginning activity | Affected saved reviews need re-check; old rows remain |
| Two offsetting corrections | Basis changes even if the final total does not |
| Adjustment after a completed review | Save re-checks; an old version is not the latest confirmation |
| Unrelated next-month activity or theme change | Does not invalidate every historical month |
| Process restart or backup restore | Review data and validity reload without in-memory generation |
| Save with open items | Clearly “has open items”, not fully confirmed |

## 10. Non-goals

- Bank or broker connections, background sync, timed fetches, automatic statement upload
- Period lock, accounting close, multi-role approval, team permissions
- Auto-posting differences as income, return, or Adjustments
- Generic bill parsing, receipts, spending budgets, tax reports
- Paid LLM or remote analysis for a monthly summary
- Complex reminder systems, month-end cron, or full-database background audit

## 11. Deferred follow-ons

Not delivery criteria for this increment. Priority depends on the largest friction after one real monthly review.

- **Institution import and matching** when manual statements dominate time. Keep current Accounts/Holdings CSV create-only. Fuzzy matches are candidates, never silent merges.
- **Asset usability** (ready cash, needs sale, locked, not for sale) as user-set attributes that do not change book value.
- **Deterministic monthly summary** from Go amounts and coverage, with residuals and partial periods kept. Optional Markdown export; no LLM.
- **Search and saved views** for scope and period, separate from review evidence and Insights session filters.
