# Code Review Findings — 2026-08-21

This document records the findings from the 2026-08-21 review of the Go +
Fyne implementation (bugs, logic issues, gaps against the
[v0.1.1 release contract](../releases/v0.1.1.md), and UI observations), and
tracks the status of each item as it was fixed in the same pass. Rows are
kept (not deleted) so the history of what was found and why stays visible.

Status values: `Open`, `Fixed`, `Deferred`, `Won't fix`.

## Bugs and logic issues

| ID | Severity | Finding | Location | Status |
| --- | --- | --- | --- | --- |
| BUG-1 | High | Members have no name-uniqueness constraint, but the "owner" field in the Account create/edit forms matched owners by exact name text. Two members with the same name made ownership entry ambiguous and could block account creation. | `internal/ui/live_pages.go` (`accountForm`, `showAccountEditDialog`) | **Fixed** — replaced the free-text owner/percentage fields with a per-member checkbox + optional percentage selector keyed by `domain.MemberID` (`newOwnershipFields`/`collectOwnership`), so duplicate names can never collide. |
| BUG-1b | High | Editing an Account that has an **archived** member as an owner always failed. The edit dialog only knew active members' names, so an archived owner's ID was shown as raw text and could never round-trip back through the name-based lookup, even without changing ownership. | `internal/ui/live_pages.go` (`showAccountEditDialog`) | **Fixed** — `ownershipEditorCandidates` now offers every active member plus any archived member who already owns the Account being edited, matching what `validateOwnershipMembersForUpdate` already allowed server-side. |
| BUG-2 | High | Hardcoded English strings bypassed the i18n catalog and appeared even when the UI language was Chinese: owner placeholder `"Alice, Bob"`, ownership-shares placeholder `"60%, 40% (optional)"`, and the `"unknown owner: %s"` validation message. | `internal/ui/live_pages.go` | **Fixed** — removed by the BUG-1/BUG-1b redesign (the free-text owner/shares fields no longer exist); the new placeholder/hint text is localized (`accounts.ownershipSharePlaceholder`, `accounts.ownershipHint`). |
| BUG-2b | Medium | Broader than BUG-2: nearly every `domain.Error` message (`"must not be empty"`, `"at least one owner is required"`, etc.) is a hardcoded English string surfaced verbatim via `err.Error()` in UI feedback labels, regardless of the selected UI language. | `internal/domain/model.go`, throughout `internal/ui/*.go` (`feedback.SetText(err.Error())`) | **Fixed** — implemented per Phase 1 of [the design doc](deferred-issues-design-2026-08-21.md#1-bug-2b--localized-error-messages): `internal/i18n/errors.go` adds `TranslateError` plus a message-text → catalog-key table covering every distinct error message in `domain`/`application`/`infrastructure/sqlite`; all 12 UI call sites now use it. Also picked up 14 previously-missing enum translations (`internal/i18n/enum_i18n.go`) and a hardcoded string in the (unreachable) demo dashboard along the way. |
| BUG-3 | Medium | `Controller.Refresh()` rebuilds the entire window content (`window.SetContent(c.Content())`) on every state change, including typing in a filter or toggling a checkbox. This can cause flicker and loses focus/scroll position, and gets worse as the account list grows. | `internal/ui/shell.go` (`Controller.Refresh`) | **Fixed** — implemented per Phase 1 of [the design doc](deferred-issues-design-2026-08-21.md#2-bug-3--controllerrefresh-rebuilds-the-whole-window): `internal/ui/region.go` adds the opt-in `region` cache; `Refresh()`/`RefreshContent()` are split; the Accounts create form (the confirmed data-loss case) now uses `accountFormRegion()` and is only invalidated on successful create or on navigating away. Regression test `TestAccountsCreateFormSurvivesContentRefresh` proves in-progress form input survives an unrelated filter-triggered refresh. Phase 2 (auditing Members/Institutions/Groups for the same pattern) is not done yet. |
| BUG-4 | Medium | v0.1.1 explicitly requires "Read-only Household name and base-currency display in Settings", but Settings never showed this information. | `internal/ui/settings_page.go` | **Fixed** — added a read-only Household card (`householdSummaryCard`) above the existing preference sections. |
| BUG-5 | Medium | v0.1.1 lists "Window-state restoration" as included, but the window always opened at a fixed `1100x720` size; nothing was persisted or restored. | `internal/app/app.go`, `internal/settings/settings.go` | **Fixed** — `Settings.WindowWidth/WindowHeight` are persisted through the existing settings store; `app.New()` restores the saved size and `Controller.PersistWindowSize` saves it via `window.SetCloseIntercept`. Cross-platform window *position* restore is not exposed by Fyne, so only size is restored. |
| BUG-6 | Medium | `sort_order` for new Members/Institutions/Groups was computed by reading all existing rows in the application layer and then inserting separately — a read-then-write race if two creates ran concurrently. | `internal/application/service.go`, `internal/infrastructure/sqlite/repository.go` (`CreateMember`/`CreateInstitution`/`CreateGroup`) | **Fixed** — `sort_order` is now computed with a `MAX(sort_order)+1` subquery evaluated inside the same write transaction as the `INSERT`; combined with the single DB connection and `_txlock=immediate`, this is race-free. Covered by `TestCreateMemberAssignsUniqueSortOrderUnderConcurrency` (run with `-race`). |
| BUG-7 | Low | `showAccountEditDialog` dereferenced `record.LatestValue.Amount` without a nil check before calling the service (the service itself checks, but the UI would panic first if it were ever nil). | `internal/ui/live_pages.go` | **Fixed** — added an early nil check that shows a validation message and returns instead of opening the dialog. |
| BUG-8 | Low | Dead code: `resultInstitution`/`resultGroup` (unused, identity functions) and `Money.SignedAmount()` (duplicate of `Money.Amount()`, easily confused with the real `Account.SignedAmount()`). | `internal/application/service.go`, `internal/domain/model.go` | **Fixed** — removed. |
| BUG-9 | Low | The Settings "primary display currency" preference only affects the tiny formatting preview sample; it has no effect on any real Account/Overview amount (which always render in the Household base currency, correctly). The label did not make this clear, which could mislead users into thinking it converts their net worth. | `internal/i18n` catalogs (`settings.numbers.currency`) | **Fixed** — relabeled to "Sample display currency (preview only)" / "示例展示货币（仅用于下方预览）" / "示例展示貨幣（僅用於下方預覽）". |
| BUG-10 | Low | The onboarding base-currency field was hardcoded to `"CNY"` regardless of the user's existing display-currency preference. | `internal/ui/live_pages.go` | **Fixed** — now pre-fills from `controller.preference.Currency`. |
| DOC-1 | Low | Legacy Rust/Tauri/React release documents could be mistaken for current Go plans even when each carried a warning. | Former `docs/releases/v0.1.1.md`, `v0.1.3*.md`, `v0.1.4*.md`, `v0.1.5*.md` | **Fixed** — v0.1.1 was replaced by a current-only Go contract, v0.1.2 was fully redesigned for Go/Fyne, and all uncorrected inherited documents were moved to `docs/legacy/rust-tauri-inherited-unreviewed/` with an archive notice and non-authoritative index. |

## Gaps against the v0.1.1 release contract

| ID | Finding | Status |
| --- | --- | --- |
| GAP-1 | Household name/base currency not shown anywhere in the running app | Fixed — same change as BUG-4 |
| GAP-2 | Window-state restoration not implemented | Fixed — same change as BUG-5 |
| GAP-3 | Stale, non-Go release docs risk being read as the current roadmap | Fixed — same relocation and active-index cleanup as DOC-1 |

## UI observations

The `internal/ui/dashboard.go` "demo" view (gradient hero cards, badges, a
trend chart, allocation bars) is never reachable from the real application —
`app.New()` always constructs a live `application.Service`, so
`Controller.mainArea()` always renders the plain `live_pages.go` views
instead. The polished visual language that exists in the codebase never
ships. **Not fixed in this pass** — redesigning every live page to that
visual language is a large, separately-scoped effort. The concrete, low-risk
UI fixes from this pass (ownership selector, Household summary card) are
folded into BUG-1/BUG-1b/BUG-4 above.

## Notes on BUG-2b and BUG-3 (previously deferred, now fixed)

Both were implemented following
[deferred-issues-design-2026-08-21.md](deferred-issues-design-2026-08-21.md),
Phase 1 of each section. Verified with `gofmt -l`, `go vet ./...`,
`go build ./cmd/nestworth`, and `go test ./... -race` — all clean.

**BUG-3**: `internal/ui/region.go` (opt-in memoized subtree, explicit
`Invalidate()`), wired into the Accounts create form via
`accountFormRegion()`; `Controller.Refresh()`/`RefreshContent()` split so
in-page filter/list changes no longer rebuild the sidebar and window menu.
Remaining work (Phase 2: same treatment for Members/Institutions/Groups,
Phase 3: optional `widget.List`-based rendering for large lists) is not
part of this pass and is still open — see the design doc's rollout plan.

**BUG-2b**: `internal/i18n/errors.go` adds a message-text → catalog-key
table and `Translator.TranslateError`; every UI call site that used to do
`feedback.SetText(err.Error())` now calls `TranslateError`. Phase 3 of the
design doc (extending the table to non-`domain.Error` messages beyond what
was needed here) is optional future work, not required by this pass.

## Regression tests added in this pass

- `internal/ui/ownership_test.go` — duplicate member names resolve by ID
  (BUG-1) and archived current owners stay editable (BUG-1b).
- `internal/infrastructure/sqlite/repository_test.go` — concurrent
  `CreateMember` calls never produce a duplicate `sort_order` (BUG-6), run
  with `-race`.
- `internal/settings/settings_test.go` — window size validation bounds and
  store round-trip (BUG-5).
- `internal/ui/shell_test.go` — `Controller.PersistWindowSize` writes to the
  settings store (BUG-5).

## Verification

`gofmt -l`, `go vet ./...`, `go build ./cmd/nestworth`, and `go test ./...`
(including the new tests above) all pass as of this pass.
