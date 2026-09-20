# Household navigation and actionable data health

Implementation of feature ideas 1, 8, 9 and 12 (2026-09-20).

## Overview

- Household net worth trend supports 30D, YTD, 1Y and All. Today's live valuation is explicitly marked. Missing observations break the chart line.
- Period change comes from the backend's exact query boundaries. Missing either boundary suppresses the change; interior gaps remain visible even when boundary change is available. Net worth change is not investment return.
- Institution, account type, member and group breakdowns open a filtered Accounts list. Member filtering selects whole accounts with that owner; it does not allocate balances by ownership percentage.
- Missing valuation inputs and individual health issues open the affected object. Trend gaps open Data Health for that date.

## Directory

Members, Institutions and Groups each retain their own Show archived toggle. Archived rows can be restored directly, with pending/error feedback. Restore changes the directory object's archived flag only; account relationships and ownership remain intact.

## Insights navigation

Account headers and holding rows offer View return and View asset changes. An account holding uses instrument scope plus an account filter; an aggregate holding uses instrument scope across accounts. Account analysis includes cash; instrument analysis excludes it.

Navigation replaces the complete analysis query so unrelated old filters cannot leak in. The current period is retained when available; otherwise the default is the last 30 closed days within recorded history. Back restores the source page, selection, filters and scroll position within the current session. Calendar, driver, contribution and category detail sheets close before their cross-page callbacks so retained source pages cannot leave a modal over the destination. Sidebar navigation starts a fresh path. This is temporary navigation, not a saved preset or refresh-persistent route.

## Data Health

Health destinations carry object IDs, the affected date range and the reason. Instrument gaps open that instrument, FX gaps open the currency pair, and missing account values open the account's update form. The Insights completeness sheet offers a date-specific health link; partial Return Trend and Change Drivers results link to Data Health for their selected period.

Manual quote forms preselect the affected instrument/pair and first missing date. They send a date-only value unchanged; the backend interprets it in the immutable History timezone, or UTC before History starts. Explicit RFC3339 timestamps keep their original instant. Historical quote views keep the requested gap range visible. A current quote refresh does not claim to fill historical gaps.

Targeted market-data repair previews the backend's actual work before starting. Its scope is all repair work for the selected object, not only the originating date range. Provider prerequisites and unresolved items remain visible. Unsupported/non-executable issues do not offer targeted repair, and empty previews cannot be confirmed. A filtered Data Health page explicitly labels Repair all as household-wide. Focused scans retain blocked snapshots and show the other household prerequisites even when their dates fall outside the selected range. Snapshot rebuilding is a separate action and cannot manufacture missing or unpublished market prices. Rebuilds run in transactions of at most 31 days; a retry resumes at the failed chunk.

Manual-value accounts with observations older than 180 days receive a warning. This is a reminder, not an expiry policy: their values remain included. Accounts lacking an observation are actionable even before History starts. Editing values/quotes invalidates health queries so returning to the source reflects a fresh scan.

## Validation

- Automated: full `wails3 task check` (Go tests/vet/build, generated bindings, frontend build/lint/typecheck/tests, whitespace checks), PASS; 56 frontend test files and 415 tests passed.
- Added coverage: strict financial boundaries, missing/stale manual valuations, full-query replacement, account/instrument scope, navigation return state, archived restore, chart gaps, snapshot chunk retry, cross-page modal dismissal, blocked snapshot dependencies, empty/unsupported repair, period health links, and date-only manual quote semantics across timezones and DST boundaries.
- Browser: independent temporary QA database, Overview account-type drilldown, account holding → scoped Return Analysis, return to the source account, group archive → show archived → restore, and Calendar day detail → Asset Changes → back (no lingering modal).
- Native Wails desktop, VoiceOver and live provider repair were not exercised. Browser verification uses the Wails server build and synthetic data, not the user's household database.
