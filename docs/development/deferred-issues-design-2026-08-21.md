# Design: Resolving the Deferred Findings from 2026-08-21

> **Implementation status (updated after review):** Phase 1 of both
> designs below has been implemented and verified (`gofmt`, `go vet`,
> `go build`, `go test ./... -race` all clean). See
> `internal/i18n/errors.go` + `internal/i18n/errors_test.go` for BUG-2b and
> `internal/ui/region.go` + `internal/ui/region_test.go` +
> `TestAccountsCreateFormSurvivesContentRefresh` for BUG-3. The status is
> also reflected in
> [`code-review-2026-08-21.md`](code-review-2026-08-21.md). Phase 2/3 items
> called out in each section below are still open follow-up work, not part
> of this pass.

This document proposes concrete designs for the two items left as
`Deferred` in [`code-review-2026-08-21.md`](code-review-2026-08-21.md):

- **BUG-2b** — hardcoded-English `domain.Error` messages reach the UI
  untranslated.
- **BUG-3** — `Controller.Refresh()` rebuilds the entire window on every
  state change.

Both designs are written to be implemented incrementally, in their own
scoped PRs, without a big-bang rewrite. Nothing here is implemented yet;
this is the plan to review before starting the work.

---

## 1. BUG-2b — Localized error messages

### 1.1 Problem, restated

`domain.Error` (and a handful of `errors.New`/`fmt.Errorf` calls in
`internal/application` and `internal/infrastructure/sqlite`) carry a
hardcoded English `Message` string. The UI shows it verbatim:

```163:163:internal/ui/settings_page.go
		status.SetText(t.T("common.invalid") + ": " + controller.validationError)
```

```292:292:internal/ui/live_pages.go
				feedback.SetText(err.Error())
```

There are roughly **60 distinct error constructions** across
`internal/domain/model.go`, `internal/application/service.go`, and
`internal/infrastructure/sqlite/repository.go`, and **12 UI call sites**
that render `err.Error()` directly (`internal/ui/live_pages.go`:11,
`internal/ui/settings_page.go`:1). All of them are English-only today, even
when the UI language is `zh-CN`/`zh-TW`.

Per the project's "always use English in code" convention, the Go source
in `internal/domain` must keep its literal error text in English — we
cannot template `%s` placeholders in Chinese inside `model.go`. The fix has
to live in the presentation layer.

### 1.2 Goals

- Every error message the user can see is rendered in the active UI
  language when a translation exists.
- Zero changes required to `internal/domain`, `internal/application`, or
  `internal/infrastructure/sqlite` source text — those packages keep
  emitting stable, English `Error()` strings for logs/debugging/tests.
- A missing translation degrades to the English message, never to a blank
  string, a template artifact like `%!s(MISSING)`, or a panic.
- Adding a new domain error and forgetting to localize it should be
  **visible** (a failing test), not a silent gap discovered by a user.

### 1.3 Non-goals

- Replacing `domain.Error.Message` with an enum/code for *every* error
  (see 1.5 "Rejected alternatives" for why).
- Localizing errors coming from the Fyne/OS layer (file dialogs, etc.) —
  out of scope, not observed as hardcoded English in the review.
- A general-purpose i18n message-formatting library (e.g. ICU
  pluralization). Current error messages are all single, fixed sentences.

### 1.4 Chosen approach: a message-text-keyed translation table in `internal/i18n`

Add a lookup table that maps the *exact* English string produced by an
error today to a catalog key, plus a small `TranslateError` helper that the
UI calls instead of `err.Error()`. No other package changes.

```go
// internal/i18n/errors.go
package i18n

import (
	"errors"
	"fmt"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// errorMessageKeys maps the exact English message text produced by
// internal/domain, internal/application, and internal/infrastructure/sqlite
// error constructors to a catalog key. The domain/application/infra layers
// stay English-only source per project convention; this is the one place
// that needs a new entry whenever a new user-facing error message appears.
// Keep entries alongside the message they were copied from, e.g. with a
// `// model.go:382` comment, so drift is easy to spot in review.
var errorMessageKeys = map[string]string{
	"must not be empty":                                   "error.notEmpty",
	"at least one owner is required":                      "error.ownership.atLeastOneOwner",
	"each share must be between 1 and 10000 basis points":  "error.ownership.shareRange",
	"an owner may appear only once":                        "error.ownership.duplicateOwner",
	"shares must total exactly 10000 basis points":         "error.ownership.sharesMustTotal100",
	"percentage must be between 0 and 100 with at most two decimals": "error.ownership.percentageFormat",
	"complete onboarding first":                            "error.onboardingRequired",
	"household must retain at least one active member":     "error.lastActiveMember",
	// ... one line per distinct message; ~60 total, added incrementally
	// (see rollout plan) rather than all at once.
}

// fieldLabelKeys maps a domain.Error.Field value to an existing catalog key
// so the same string used for a form label ("Owner", "所有人", ...) prefixes
// the translated message, instead of the raw English field name leaking
// through untranslated.
var fieldLabelKeys = map[string]string{
	"ownership":       "accounts.owner",
	"amount":          "accounts.amount",
	"trackingMode":    "accounts.trackingMode",
	"defaultCurrency": "settings.household.baseCurrency",
	"institutionId":   "nav.institutions",
	"groupId":         "nav.groups",
	"effectiveAt":     "accounts.effectiveDate",
	// ...
}

// TranslateError renders err for the Translator's active language.
//
//   - If err's exact text is in errorMessageKeys, the translated text is
//     returned (with the field label translated and prefixed, for
//     *domain.Error values that carry a Field).
//   - Otherwise err.Error() is returned unchanged (English fallback), so a
//     translation gap is a readable-but-untranslated message rather than a
//     blank string or a crash.
func (t *Translator) TranslateError(err error) string {
	if err == nil {
		return ""
	}
	key, ok := errorMessageKeys[baseMessage(err)]
	if !ok {
		return err.Error()
	}
	message := t.T(key)
	var domainErr *domain.Error
	if errors.As(err, &domainErr) && domainErr.Field != "" {
		field := domainErr.Field
		if labelKey, ok := fieldLabelKeys[field]; ok {
			field = t.T(labelKey)
		}
		return fmt.Sprintf("%s: %s", field, message)
	}
	return message
}

// baseMessage strips the "field: " prefix *Error.Error() adds so the table
// above only ever needs the underlying Message text, not every field
// combination that could prefix it.
func baseMessage(err error) string {
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		return domainErr.Message
	}
	return err.Error()
}
```

Then every UI call site changes from:

```go
feedback.SetText(err.Error())
```

to:

```go
feedback.SetText(c.translator.TranslateError(err))
```

This is a mechanical, low-risk, purely-additive change: 12 call sites in
`internal/ui`, one new file in `internal/i18n`, zero changes anywhere else.

### 1.5 Rejected alternatives

**Add a stable `Key`/enum field to `domain.Error` itself.** This is the
"proper" long-term shape (a real code you can `switch` on instead of
string-matching English text), but it means touching all ~60 error
construction call sites across three packages in one pass, and keeping the
key in sync with the message forever. Given the current error messages are
already string constants (not built from runtime data — no interpolated
values to lose), matching on the literal string carries the same
information with a fraction of the diff. If the domain layer later grows
error messages with runtime-interpolated data (e.g. `"%d exceeds limit"`),
revisit this — the message-text table stops working once messages are not
fixed strings, and at that point an explicit `Key` field on `domain.Error`
is the right upgrade.

**Localize inside `internal/domain` with a language parameter.** Rejected
outright: it would put UI-language concerns inside the domain layer and
violates the project rule that code (including domain messages) is
English-only; translation is a presentation concern.

**Machine translation / runtime translation service.** Overkill for ~60
fixed, short, already-known sentences and adds a runtime dependency for no
benefit over a static table.

### 1.6 Testing strategy

1. **Completeness test** (`internal/i18n/errors_test.go`): for every key in
   `errorMessageKeys`, assert `T(key)` returns a non-empty, non-fallback
   string in all three languages (English, zh-CN, zh-TW). This catches a
   key added to the map without matching catalog entries.
2. **Fixture tests**: for a representative sample of real domain/service
   failures (e.g. `domain.ParseOwnership(nil)`, `service.CreateMember(ctx,
   "")` after onboarding, `service.CreateAccount` with a mismatched
   currency), assert `translator.TranslateError(err)` — with the
   translator set to `zh-CN` — differs from `err.Error()` and equals the
   expected Chinese sentence. This is the regression guard against message
   text drifting out of sync with the table (if `model.go` changes a
   message string, the matching fixture test starts returning the English
   fallback and fails).
3. Existing UI tests (`shell_test.go`, `ownership_test.go`) keep working
   unchanged since `TranslateError` is additive.

### 1.7 Rollout plan

Localizing all ~60 messages in one PR is a lot of reviewable surface for a
`Low` severity issue. Suggested phased rollout, each phase shippable and
independently valuable:

1. **Phase 1** — land `TranslateError`/`errorMessageKeys`/`fieldLabelKeys`
   with entries for the messages a user is most likely to actually hit
   today: onboarding (`"at least one member is required"`, `"complete
   onboarding first"`), Account creation/edit (ownership messages, `"must
   use YYYY-MM-DD"`, currency mismatch messages), and Member/Institution/
   Group archive conflicts (`"household must retain at least one active
   member"`). Wire up the 12 UI call sites to use it.
2. **Phase 2** — fill in the remaining validation messages (name length,
   currency code format, category/tracking-mode combinations) as a
   mechanical follow-up; low risk, easy to review in a table diff.
3. **Phase 3 (optional)** — extend `errorMessageKeys` to cover non-`domain.Error`
   messages that are also hardcoded English today, e.g.
   `format.ValidateTimezone`'s error text used in `settings_page.go`. The
   `baseMessage`/table design already supports this (it keys on `err.Error()`
   for anything that isn't a `*domain.Error`), so this is purely additive.

### 1.8 Effort and risk

- **Effort:** Phase 1 is a half-day (new file + ~15 table entries + 12
  one-line call-site edits + tests). Phase 2 is mechanical, a couple of
  hours. Low risk throughout — additive, no existing behavior changes when
  a message isn't yet in the table.
- **Risk:** the only failure mode is "forgot to add an entry," which
  degrades to the current English text — never worse than today.

---

## 2. BUG-3 — `Controller.Refresh()` rebuilds the whole window

### 2.1 Problem, restated, with a concrete failure case

```160:165:internal/ui/shell.go
func (c *Controller) Refresh() {
	if c.window != nil {
		c.window.SetMainMenu(NewMainMenu(c.application, c.window, c.icon, c.translator))
		c.window.SetContent(c.Content())
	}
}
```

`Content()` calls `c.sidebar()` and `c.mainArea()` fresh every time, and
every live page (`NewAccountsPage`, `NewLiveOverview`, `NewSettingsPage`,
…) is a pure function that builds a brand-new widget tree from
`Controller` state. This isn't just a flicker/perf concern — it causes a
real, reproducible data-loss bug today:

> Open **Accounts**, start typing a name into the "Create account" form at
> the bottom of the page, then click any filter dropdown above it (e.g.
> change the category filter). `Controller.Refresh()` runs,
> `NewAccountsPage` is called again, and it calls `accountForm(c)`
> unconditionally — a brand-new, empty form. **The half-typed account is
> silently discarded.**

This happens because there is no notion of "this subtree's state should
survive a refresh unless something specific to it changed."

### 2.2 Goals

- Stop discarding in-progress form input when an unrelated part of the
  page (or the window) refreshes.
- Reduce unnecessary rebuild work (sidebar + main menu + entire page tree)
  for changes that only affect part of the page.
- Ship incrementally: each step should be independently mergeable and
  should not require rewriting every page in one PR.
- Never make things worse: a page that isn't migrated keeps exactly
  today's behavior (full rebuild), so risk is opt-in and localized to the
  pages that adopt the new pattern.

### 2.3 Non-goals (for now)

- A full binding-based/virtual-DOM-style reactive UI rewrite of every page.
  That is the natural end state (see Phase 3) but is a large, risky,
  all-at-once change that this design deliberately avoids requiring.
- Solving it for the demo `dashboard.go` view, which is unreachable from
  the running app (see the review doc's UI observations) and not worth
  investing in until/unless it ships.

### 2.4 Chosen approach: an explicit, opt-in "region" cache

Introduce a small primitive that memoizes a subtree until something
explicitly invalidates it. This lets us keep the existing "pure function
returns a tree" style everywhere (no risky rewrite) while opting specific,
high-value subtrees (like the account creation form) out of unconditional
rebuilding.

```go
// internal/ui/region.go

// region memoizes the result of build() until Invalidate is called. Nested
// inside a page's returned tree, it lets one subtree keep its widget
// identity (and therefore focus, cursor position, and typed-but-unsaved
// text) across a Controller.Refresh() triggered by something unrelated,
// while everything outside the region still rebuilds normally.
//
// This is intentionally minimal: no diffing, no data binding, just an
// explicit cache with an explicit invalidation call at the handful of
// places that actually need to reset the region (e.g. after a successful
// create, or when navigating away from the page).
type region struct {
	build  func() fyne.CanvasObject
	cached fyne.CanvasObject
}

func newRegion(build func() fyne.CanvasObject) *region {
	return &region{build: build}
}

// Invalidate discards the cached tree so the next Object() call rebuilds
// it. Call this only when the region's own state should reset (e.g. the
// form was just submitted), not on every unrelated Controller.Refresh().
func (r *region) Invalidate() { r.cached = nil }

func (r *region) Object() fyne.CanvasObject {
	if r.cached == nil {
		r.cached = r.build()
	}
	return r.cached
}
```

`Controller` owns one `*region` per page that needs this, created lazily
and keyed by page so navigating away and back gets a fresh region (no
stale cross-page state):

```go
// in Controller
accountFormRegions map[Page]*region // lazily populated

func (c *Controller) accountFormRegion() *region {
	if c.accountFormRegions == nil {
		c.accountFormRegions = map[Page]*region{}
	}
	r, ok := c.accountFormRegions[PageAccounts]
	if !ok {
		r = newRegion(func() fyne.CanvasObject { return accountForm(c) })
		c.accountFormRegions[PageAccounts] = r
	}
	return r
}
```

`NewAccountsPage` changes from calling `accountForm(c)` directly to
`c.accountFormRegion().Object()`. The only place that calls `Invalidate()`
is the successful-create callback (where the form is already being reset)
and `navigate()` when leaving `PageAccounts`. A filter dropdown changing,
or an unrelated account being archived, no longer touches the form at all
— its widgets, and whatever the user typed into them, survive.

### 2.5 Reducing chrome rebuilds (sidebar + main menu)

Independent of regions, split `Refresh()` into two tiers so a data-only
change doesn't tear down and rebuild the sidebar and window menu, which
never depend on anything other than the current page, language, and theme:

```go
// Refresh rebuilds the sidebar, main menu, and page content. Call this
// when the page, language, theme, or backend connectivity changed.
func (c *Controller) Refresh() {
	if c.window == nil {
		return
	}
	c.window.SetMainMenu(NewMainMenu(c.application, c.window, c.icon, c.translator))
	c.window.SetContent(c.Content())
}

// RefreshContent rebuilds only the main content area, leaving the sidebar
// and window menu untouched. Use this for in-page data changes (filters,
// list reloads after a mutation) where navigation state, language, and
// theme did not change.
func (c *Controller) RefreshContent() {
	if c.window == nil {
		return
	}
	c.window.SetContent(container.NewBorder(nil, nil, c.sidebar(), nil, c.mainArea()))
}
```

This still calls `window.SetContent`, so it is a smaller, not a zero,
change on its own — the real win is combining it with regions (2.4) for
the subtrees that must not lose state, and reserving full `Refresh()` for
navigation/theme/language changes as originally intended. A future
iteration can replace the `window.SetContent` call in `RefreshContent`
with mutating a persistent root container's `Objects` slice in place
(avoiding even the top-level content swap), once enough pages use regions
that it is safe to do so without stale nested state.

### 2.6 Rejected alternatives

**Full Fyne `binding.*` rewrite of every page now.** This is the
architecturally "right" end state and is listed as Phase 3 below, but
doing it in one pass means rewriting `NewAccountsPage`, `NewLiveOverview`,
`NewMembersPage`, `NewInstitutionsPage`, `NewGroupsPage`,
`NewSettingsPage`, and every dialog builder at once — a large, high-risk
change for a `Medium` severity issue, and it blocks on nothing else in
this plan. The region approach gets the concrete bug (data loss) and most
of the perf win fixed now, and doesn't foreclose migrating individual
pages to bindings later.

**Diff the old and new widget trees and patch in place (virtual-DOM
style).** Fyne widgets don't expose the kind of structural equality/key
metadta this needs, so this would mean building and maintaining a bespoke
reconciler — much more machinery than the actual problem (a few subtrees
with state worth preserving) justifies.

### 2.7 Rollout plan

1. **Phase 1** — add `region`, split `Refresh()`/`RefreshContent()`, and
   apply a region to the one confirmed data-loss case: the account
   creation form in `NewAccountsPage`. Ship with a regression test (2.8).
2. **Phase 2** — audit the other live pages for the same "form embedded in
   a list page that refreshes on unrelated filter/list changes" shape
   (Members/Institutions/Groups pages follow the same
   list-plus-inline-create pattern) and apply the same region treatment
   where the same bug exists. Switch filter/list-only refreshes to
   `RefreshContent()` instead of `Refresh()`.
3. **Phase 3 (optional, larger)** — for pages where regions plus
   `RefreshContent()` still aren't enough (e.g. the Accounts list itself
   growing large enough that even list rebuilds are visibly slow), migrate
   that specific list to a Fyne-native incremental widget (e.g.
   `widget.List` bound to a data source) instead of a `container.VBox` of
   freshly-built rows. This is scoped per-page and only taken on if a
   concrete perf problem shows up (v0.1.1's data volumes are small; this is
   explicitly not needed yet).

### 2.8 Testing strategy

1. **Regression test for the data-loss bug**: build the Accounts page,
   type into the create-account name entry, trigger the same code path a
   filter change uses (`c.RefreshContent()` or an equivalent
   `c.accountCategory = ...; c.Refresh()`), rebuild the page, and assert
   the *same* `*widget.Entry` pointer (obtained via the region) still has
   the typed text — using `fyne.io/fyne/v2/test` the way `shell_test.go`
   already does.
2. **Region unit tests** (`internal/ui/region_test.go`): `Object()` returns
   the same pointer on repeated calls; returns a new pointer only after
   `Invalidate()`; `build` is not called until the first `Object()` call
   (laziness).
3. Existing `TestLiveAccountsPageBuildsWithoutRecursiveRefresh` and other
   shell tests continue to guard against infinite refresh loops as regions
   are introduced.

### 2.9 Effort and risk

- **Effort:** Phase 1 (the `region` primitive, the `Refresh`/
  `RefreshContent` split, and applying it to the Accounts create form) is
  roughly a day including tests. Phase 2 is similar-sized, page by page.
  Phase 3 is only taken on if/when needed.
- **Risk:** low and localized — a page not migrated behaves exactly as
  today. The main thing to get right is remembering to call
  `Invalidate()` at the right points (after successful submit, on
  navigating away); the region unit tests and the data-loss regression
  test are meant to catch a missed invalidation quickly.

---

## Summary

| Item | Approach | Where it lives | Phase 1 effort | Risk |
| --- | --- | --- | --- | --- |
| BUG-2b | Message-text → catalog-key lookup table + `TranslateError` helper | New `internal/i18n/errors.go`; 12 call-site edits in `internal/ui` | ~half day | Low, additive, graceful English fallback |
| BUG-3 | Opt-in `region` cache + `Refresh`/`RefreshContent` split, applied first to the Accounts create form | New `internal/ui/region.go`; targeted edits in `shell.go` and `live_pages.go` | ~1 day | Low, localized to migrated pages |

Neither design requires touching `internal/domain` or rewriting the
existing "pure function returns a widget tree" page style; both are meant
to be landed as their own small PRs, in the phases above, and both leave a
clearly-marked path (Phase 3 in each section) to a more thorough long-term
fix if the team later decides it's worth the larger investment.
