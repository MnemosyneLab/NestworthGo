# Nestworth-go code smell review

26 August 2026. Read-only pass over the current Wails v3 tree (`0.2.0`). The frontend activity-sentence extraction reviewed here is already committed. Not a security audit. Tests and the native UI were not re-run.

## Bottom line

The architecture is fine. Financial truth lives in Go; the React app formats DTOs; `wailsapi` wraps errors and never opens SQLite. Typed IDs, `changeMu` on history writes, and “copy the provider under the lock then work unlocked” are the right instincts.

What hurts is concentration. Almost every financial use case hangs off one `application.Service` and one ~94-method `Repository`. The same onboarding check is pasted through `service.go` while a helper already exists next door. On the frontend, query invalidation is distributed across mutation hooks without an explicit dependency map. Saved Dark / zh-CN preferences are ignored at frontend boot, and the window-close path can overwrite newly saved settings with the snapshot loaded when the process started.

Do the small, certain cleanups first. Split the god object after those land.

## Do this first

**Fix the Settings lifecycle before cleanup work.** `App.tsx` never calls `useSettings()`. `SettingsPage` applies appearance and language only on Save/Reset. The header toggles mutate zustand / i18next and never call `SettingsService.Save`, so persisted appearance/language is invisible at frontend boot and header changes are session-only.

There is a second, more serious write path in `cmd/nestworth`: `preference` is loaded once during startup and captured by the window-closing callback. `persistWindowSize` changes only width/height on that stale copy and saves the entire struct. If Settings saved language, appearance, currency, or provider after startup, closing the window can restore the startup values while preserving only the new dimensions.

Use one lifecycle:

1. Load Settings once in the application shell after `Startup` succeeds and before rendering the normal workspace. Apply appearance and resolved language from that query result.
2. Make the header controls submit a complete Settings value through the same mutation as SettingsPage, with optimistic UI only if failure restores the last persisted value and reports an error. Removing the controls is acceptable only as an explicit product decision.
3. On window close, load the latest value from `settings.Store`, merge only the current width/height, validate, and save. Do not close over the startup snapshot. If the latest load fails, log and skip the write instead of replacing the file with stale defaults.
4. Keep FX-provider ordering unchanged: apply the provider before persisting the Settings value.

Acceptance: a saved Dark / zh-CN preference applies before the main shell becomes interactive; a failed header save does not pretend persistence succeeded; saving Settings, resizing, closing, and reopening preserves every non-window field; a window-size persistence failure does not corrupt or replace the previous Settings file.

**Preserve decimal strings during display formatting.** `formatAmount` calls `Number(amount)` before `Intl.NumberFormat`. That is not exact across the documented bounds: integers above `2^53 - 1`, quantities approaching `10^18`, and sufficiently precise fractional values can change before rendering. Its comment claiming exact conversion across those bounds is false. Separately, `Intl.NumberFormat(undefined, …)` follows the host locale while timestamps explicitly use `i18n.language`, so one screen can mix separators and currency conventions.

Keep the canonical decimal string intact. Use a decimal-string-aware formatter, or split sign/integer/fraction, localize grouping and separators without a binary-float conversion, and apply the supported currency's display rules deliberately. Pass the resolved i18next language explicitly for both money and quantity. Do not feed the formatted result back into calculations.

Acceptance: table tests cover values around `2^53`, an 18-digit quantity, long fractional quantities, negative values, zero, supported currency display, and en / zh-CN / zh-TW separators. The rendered digits must round-trip to the input canonical value after removing presentation-only grouping/currency tokens.

**Make invalidation dependency-driven, not universal.** Several writes can leave derived data stale, especially `["analytics", "accountGain", …]`. But a single `invalidateAfterPortfolioChange()` on every mutation would replace under-invalidation with unnecessary refetching:

| Mutation family | Invalidate | Do not invalidate without another dependency |
| --- | --- | --- |
| Account metadata, ownership, archive, current value | accounts, Overview, affected account-gain/current analytics | History activities/origin; bootstrap, which contains no accounts |
| Holding create/archive and recorded/fixed/undone changes | holdings, Overview, affected account gain; History only when an Activity changed | unrelated directory and Settings queries |
| Manual/provider instrument quote | quote, Overview, holdings-derived gain/account gain | raw holdings and History activities unless the write also records an Activity |
| Instrument identity/create/archive | instruments | Overview merely because the Instrument exists or is archived; valuation intentionally retains active Holdings of archived Instruments |
| Instrument quote-source/provider binding | instruments, quote selection, Overview, current gain | raw holdings and History activities |
| Required-FX refresh | Overview and current gain views that convert through FX | raw instrument quote and holdings lists |
| Refresh all | Overview, instrument quote, and current gain views | raw holdings and History origin/activities |
| Start History | History namespace | current accounts, holdings, Overview, or Analytics; the operation captures an immutable origin without changing the current portfolio |

Export query-key factories from one module, normalize filters before both key construction and the service call, and provide small invalidators per dependency group (`invalidateAccountReads`, `invalidateCurrentValuation`, `invalidateHistoryReads`) rather than one global invalidator. Treat an empty account filter as `{ includeArchived: false }`, so `useAccounts({})` and `useAccounts({ includeArchived: false })` share one cache entry. Where an operation has a concrete account ID, invalidate only that account-gain key; fall back to the analytics prefix when the affected accounts are not cheaply known.

Acceptance: mutation-hook tests seed every dependent cache with stale data and assert the exact keys invalidated; tests also assert known-unrelated keys stay fresh. Cover account create/update/archive, holding create/archive, record/fix/undo, manual quote, Refresh Required FX, Refresh All, and Start History.

**Use one lightweight household gate.** `portfolio.go` already has `onboardingRequired()`. `history_changes.go` uses it. `service.go` still inlines `Bootstrap` + `"complete onboarding first"` on every Create / Archive / Set\*. Most of those calls load members, institutions, and groups just to read `Household.ID`.

Add `requireHousehold(ctx) (domain.Household, error)` on the application service, backed only by `Repository.Household`. Keep `Bootstrap` for screens that actually need Household + active Members + Institutions + Groups. Replace identity-only gates without changing the existing `conflict` code or `"complete onboarding first"` message. Do not replace calls whose following validation intentionally uses the Bootstrap directory collections until the port has direct reference lookups.

Acceptance: no identity-only mutation performs directory list queries; onboarding-incomplete behavior keeps the same error code/message; existing ownership/institution/group validation still rejects archived or cross-Household references.

**Normalize expected errors, injected clocks, and async cancellation.** `apierror.Wrap` correctly hides any error that is not a `*domain.Error` as `{ code: "internal" }`; do not turn every infrastructure/programming failure into a user-visible domain message. The rule should be narrower: every expected, user-actionable failure leaving `application` must be a `*domain.Error`, while unexpected infrastructure errors remain opaque and cross the wire as generic internal failures.

Apply that rule to the known mismatches: invalid image data at pick time, the `startDate` parse in `RebuildHistoricalSnapshots`, and expected SQLite conflicts such as an existing Household. Keep raw database/file/provider details out of domain messages. Inject an `ImageNormalizer` rather than importing `infrastructure/media` from `application`.

Pass `s.clock()` through media/icon and instrument logo/quote-source update ports instead of stamping `time.Now()` in sqlite. Preserve the existing timestamp normalization. For cancellable refresh, make duplicate active request IDs cancel-and-replace atomically without changing the existing void Start-method wire shape. Store a per-registration generation/token; an older goroutine may clear or emit only when its token is still current. Suppress the superseded goroutine's completion so the request ID has one authoritative completion: the replacement's.

Acceptance: invalid images and invalid snapshot ranges return stable validation errors; unexpected repository errors still become generic internal wire errors; frozen-clock tests cover media/icon/instrument metadata; duplicate refresh-ID tests prove no uncancellable entry, cross-delete, or double authoritative completion.

**Delete dead surface while it is still cheap.** `CreateAccountWithActivity` is on the port and implemented in sqlite, never called. `Repository.DB()` has no callers. Package `application.PreviewChange` only forwards to `domain.PreviewChange`. `safePortfolioError` is `return err` at ~20 sites. Frontend: `SampleForm`, unreachable `ComingSoonPage`, `implementedPageIds` duplicating `NAV_ITEMS`, unused hooks (`useAppendAccountValue`, `useSetAccountIcon`, `useArchiveHolding`, `useAccountGain`, `useSetInstitutionIcon`, …), `components/ui/dialog.tsx`. Gitignore `frontend/.bindings-tmp*` and delete the leftover tree — `/frontend/bindings/` is ignored, the temp dir is not.

Then split `Service` / `Repository` by bounded context: directory, accounts, portfolio/quotes, history/change, snapshots. `ValuationService` and `GainService` already exist; stop calling `NewX(s.repository, s.clock)` on every IPC.

## Where the mass is

These files are not bad because they are long. They are long because new work has nowhere else to go.

| File | Lines |
| --- | ---: |
| `internal/domain/change.go` | 1356 |
| `internal/infrastructure/sqlite/portfolio_repository.go` | 1291 |
| `internal/application/service.go` | 1259 |
| `internal/infrastructure/sqlite/repository.go` | 1110 |
| `internal/wailsapi/wire/wire.go` | 1089 |
| `frontend/src/i18n/additions.ts` | 968 |
| `internal/application/portfolio.go` | 958 |
| `internal/application/gain_service.go` | 814 |
| `internal/application/historical_snapshot.go` | 775 |

`Repository` in `service.go:17-112` runs from `Household` through `ListDailyValuationSnapshots`. `Service` methods continue across `service.go`, `portfolio.go`, `history.go`, `history_changes.go`, `change_service.go`, `refresh.go`, and `historical_snapshot.go`. Every financial Wails adapter takes `*application.Service`; `wailsapi/app` is independent and Settings also owns a Store dependency. Current application-service tests usually open a real database because implementing a focused fake against the broad Repository port is disproportionately expensive, not because faking it is technically impossible.

The riskiest blob inside that mass is `historicalPortfolioSnapshot` (~350 lines). It replays accounts, holdings, cash, quotes, and FX in one procedure, and the batch cache is `ctx.Value(historicalSnapshotBatchKey{})`. Five `latest*Observation` helpers copy the same cutoff rule. Extract an explicit `HistoricalReplay` with an injected batch. This path has existing history regression tests; use them.

`domain/change.go` mixes UUID wrappers with the planner. `PreviewChange(state, command any)` is a type switch whose `default` is a runtime error. A marker `ChangeCommand` interface would restrict the accepted set but would not make Go type switches exhaustive: adding a new implementation would still compile without adding a case. Choose deliberately between a behavior-bearing interface/visitor, which moves dispatch onto each command, and keeping the central switch with a registry/table test that enumerates every supported command and wire mapping. Split IDs out of the planner when you touch the file.

`wire.go` is a mapping dump. The comment still talks about per-service `dto.go` files that mostly do not exist. Keep time/money primitives in `wire`; move Overview / Activity mapping next to the service that owns them. `portfolio` importing `account.AccountFilterRequest` is the same scatter.

## Copy-paste that will drift

Directory CRUD in `service.go` is Member / Institution / Group three times (Bootstrap, factory, persist, list-all-then-scan on update). `ArchiveInstitution` nests the onboarding check; `ArchiveGroup` does not. Wails `directory.go` repeats parse → app → wrap → `wire.From*` for each entity. Bindings need separate methods; application does not need three copies.

Account create in sqlite is three transaction shapes (`CreateAccount`, `CreateAccountWithActivity`, `CreateAccountWithHistory`) with the same validate/insert/ownership/initial checks. Fold into one helper with optional observation/commit once the unused variant is gone.

`UpdateMember` / `UpdateAccount` list every row and scan. `ValuationService.Account` loads a full portfolio snapshot. Fine for one small household; it encodes “there is no Get-by-ID” into the application API. Add `Member(id)` / `AccountRecord(id)` when the port splits.

`listMembersQuery` / `listInstitutionsQuery` / `listGroupsQuery` are the same `includeArchived` + `ORDER BY` + scan loop.

On the frontend, `DirectoryEntityList` is the CRUD pattern Accounts and Investments have not adopted. `RecordChangeForm` clones `AccountSelect` / `MoneyFields` per kind and builds the command from `Record<string, string>` plus `kind as ChangeCommandRequest["kind"]`. `activityToInitialCommand` fills `reason` and `note`; the form has no those fields, so Fix can resubmit hidden values. Give it typed state (or RHF + Zod per kind), a field config instead of cloned JSX, and either show or drop reason/note.

Image attach is copied (`attachAccountLogo` vs `attachImage`) and the error UX already diverges. Native `<select className="h-9 rounded-md …">` appears ~25 times; there is no shared `Select`. Instrument archive has no confirm dialog; accounts, directory, and history undo do. Holdings flattening is implemented twice (`InvestmentsPage` vs `useAllHoldingsFlat`). Activity name maps are rebuilt in both History and Overview.

## Frontend state that lies

Settings draft sync is a render-phase `setState` keyed by response object identity (`settings.data !== syncedFrom`) plus `JSON.stringify` for dirty. Render-phase adjustment is supported by React, but this identity rule can replace a dirty draft after a background refetch returns an equivalent new object. Initialize/reset the form only on first load and explicit Save/Reset success; preserve a dirty draft across unrelated refetches, and compute dirty state through typed fields or the form library rather than serialized object identity.

`additions.ts` (968 lines) exists so “generated” locale JSON will not clobber frontend-only keys. The JSON is already hand-maintained product copy. `history.kind` lives in both; `deepMerge` lets additions win. SampleForm strings and leftover `history.activityAccount` / `activityValue` keys after the sentence rewrite are still in the overlay. Fold into `locales/*.json` or stop pretending the JSON is generated.

Activity sentences are the right extraction — Overview and History share one committed builder. Timeline does not wait for account/instrument queries, so the first paint can say “an account”. Incomplete data falls back to `displayEnum` kind codes, which the History page comment says not to show. Overview still labels missing instrument prices with `unknownAccount`.

The locale mismatch and binary-float precision loss in `formatAmount` are confirmed defects, not just consistency smells; the first-work section above defines the required formatting boundary and coverage.

Pages unmount on navigate (`activePageId === "overview" && …`). Investments tab, Analytics range, History sheet, and Market Data last result reset. Mapping `NavItem.id` to a component removes the repeated conditionals but does not preserve state: rendering only the selected map entry still unmounts the others. Decide per state whether reset-on-leave is desirable. For state that must survive, keep the page mounted and hidden, lift the state into the shell/store, or encode it in routing; do not imply that a component map alone fixes the lifecycle.

## Edges: errors, clock, layers, tenancy

`apierror.Wrap` turns anything that is not `*domain.Error` into `{ code: "internal" }`. That is the correct secrecy boundary for unexpected infrastructure/programming failures. The inconsistency is in expected failures: `CreateMediaAsset` maps a bad image to validation, while `NormalizeImage` returns `media.ErrInvalidImage` unmapped, so pick-time failure looks internal; sqlite still has `errors.New("household already exists")` next to a conflict error; `RebuildHistoricalSnapshots` wraps the `endDate` parse but returns raw `time.Parse` for `startDate`. `provider_time.go` errors describe malformed upstream data and may remain opaque when they are not actionable by the user. Map expected, user-actionable failures to `*domain.Error`; preserve opaque internal failures for `apierror.Wrap` to redact.

`application/service.go` imports `infrastructure/media`. Domain does not import infrastructure; this is the one inversion. Inject an `ImageNormalizer`.

Financial writes take `s.clock()`. Media/icon `updated_at` (`setMediaReference`, `setIconReference`, some instrument logo/quote-source updates) stamp `time.Now()`. Frozen-clock tests will not see those rows. Pass `now` through, like `Set*Archive`.

`PreviewChange` re-derives Household from Bootstrap. `DailySnapshotState` and `CompleteDailySnapshotRange` take a client `householdId`. One household today hides it. Use `resolveHouseholdID()` everywhere.

`registerCancel` overwrites `cancels[requestID]` without cancelling the previous goroutine. Worse, the older goroutine can finish and `clearCancel(requestID)`, deleting the newer entry. Duplicate `StartRefreshAll` IDs can therefore leave a refresh uncancellable and emit competing completions. Cancel-and-replace atomically, track a registration generation/token, and allow only the current generation to clear the entry or emit completion.

`CommitChange` is `RecordChange`. Keep a wire alias if the UI needs the name; do not keep two application methods.

`PreviewFixChange` holds `changeMu` even though it does not write. Keep that protection for now: the preview reads the Activity, reversal status, effects, and current state in multiple steps, and must not race a History write into a mixed view. Revisit only with an equivalent snapshot transaction or a proven `RWMutex` design. `cmd/nestworth` logs a Bootstrap failure and continues; a failed DB open correctly registers only `AppService`. `ChangeCommandKind` (`money_added`) and `ActivityKind` (`cash_in`) are translated in backend `ToCommand` and in frontend `activityToInitialCommand`; a bidirectional table test should lock both mappings.

Account input is three ownership encodings (shares, owner IDs, ID + percentages) plus `*Set` flags, duplicated on `application.AccountInput` and `domain.AccountInput`. One patch type is enough once the form always sends shares.

Application tests copy `sqlite.Open` + `CompleteOnboarding` in almost every function. `wailstest.NewService` and `newRefactorTestService` already exist. Frontend tests construct a bare `QueryClient()` (TanStack default retry 3) while the app uses `retry: 1`, and they assert English literals.

## Leave these alone

Domain has no infrastructure import. `ActivityID` cannot be passed as `AccountID`. Preview is not write authorization; `RecordChange` reloads under `changeMu`. Settings live outside the financial DB; FX provider is applied before persist. `ChangeCommandRequest` as one discriminator struct is a Wails v3 binding constraint; household ID for commands is injected server-side. Refresh results do not include provider URLs. Failed DB open exposes only `AppService`.

Frontend: `callService` + `errorCode` namespace, no `any` / `@ts-ignore` in `src`. Go remains the calculation authority (Overview golden fixture, holdings gain from `AccountGain`). `usePreviewFixChange` (not `PreviewChange`) for Fix is documented and regression-tested. `DirectoryEntityList` and `activitySentence` are the abstractions the other pages should catch up to. There are no `TODO`/`FIXME` markers in production code. Schema verify and history/gain integrity tests are heavy on purpose.

## Suggested order of work

1. Fix Settings close-time stale overwrite, boot hydration, and header persistence as one lifecycle change. Verify save → resize → close → reopen, including a failed save/load path.
2. Replace `formatAmount`'s `Number` conversion with locale-explicit decimal-string formatting. Lock precision at the `2^53` and `10^18` boundaries before changing call sites.
3. Introduce normalized query-key factories and dependency-specific invalidators. Add exact invalidation tests before migrating mutation hooks, so Start History and identity-only Instrument writes do not acquire unrelated refetches.
4. Add `requireHousehold` with the same conflict code/message, then migrate identity-only gates. Keep Bootstrap where directory collections are used for reference validation.
5. Normalize expected image/date/conflict errors, inject the image normalizer and clock, and define duplicate refresh-ID semantics with race-focused tests.
6. Type `RecordChangeForm` while preserving preview/confirm and visible-or-explicitly-dropped reason/note. Remove confirmed dead surface and ignore `.bindings-tmp*` in the same low-risk cleanup phase.
7. Split `Repository` / `Service` by bounded context, retaining thin Wails adapters and explicit cross-context orchestration where one operation is genuinely atomic.
8. Extract `historicalPortfolioSnapshot` into an explicit replay component with an injected batch; keep the history regression, gain-integrity, snapshot, and concurrency suites green.

The first five items are independently shippable. Do not combine them with the Service/Repository split: smaller diffs keep user-visible fixes reviewable and preserve a clean regression baseline for the structural work.

Related UX/copy review from the same day: [nestworth-go-frontend-review-2026-08-26.md](./nestworth-go-frontend-review-2026-08-26.md). Architecture contracts: [system-overview.md](../architecture/system-overview.md), [data-and-ipc-contracts.md](../architecture/data-and-ipc-contracts.md).
