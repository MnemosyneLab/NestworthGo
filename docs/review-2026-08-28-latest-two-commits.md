# Code Review: Latest Two Commits

Reviewed commits:

- `8a88d3e` — Implement Account-container interaction design (#7)
- `4e45733` — Require an explicitly checked Account owner before confirm or save (#9)

Review method: full diff read of both commits, cross-checked against `docs/architecture/account-container-interaction-design.md`, plus targeted `go build` / `go test` / `pnpm test` verification and manual re-reading of the flagged code paths.

---

## Summary

| Commit | Bugs | Design deviations | Test gaps | Nitpicks |
| --- | --- | --- | --- | --- |
| `8a88d3e` (#7) | 1 | 0 | 2 | 2 |
| `4e45733` (#9) | 0 | 0 | 3 | 2 |

Overall both commits are solid: money math consistently uses `decimal`/string types (no floats for financial values in Go), the nil-vs-explicit-empty ownership semantics in #9 are correctly threaded end-to-end, and i18n keys stay in sync across `en` / `zh-CN` / `zh-TW`. The one real bug found is a stale-state navigation issue in #7 that is user-visible but not data-destructive.

---

## Commit `8a88d3e` — Implement Account-container interaction design (#7)

### 🐛 Bug: stale `selectedAccountId` when navigating away and back to "Accounts"

**File:** `frontend/src/App.tsx:83-89`

```tsx
const handleNavigate = (pageId: string) => {
  if (pageId === "accounts" && activePageId === "accounts") {
    setSelectedAccountId(null);
  }
  markVisited(pageId);
  setActivePageId(pageId);
};
```

`selectedAccountId` is only cleared when the user clicks "Accounts" while **already on** the Accounts page. Reproduction:

1. From `PortfolioPage`, click an account row → `openAccount(id)` sets `selectedAccountId` and switches to the "accounts" page, showing `AccountDetail`.
2. Navigate to e.g. "Overview" via the sidebar.
3. Click the top-level **"Accounts"** nav item.

At step 3, `handleNavigate("accounts")` runs with `activePageId === "overview"`, so the guard condition is `false` and the stale `selectedAccountId` is never cleared. The user lands back on the previously-opened `AccountDetail` instead of the accounts list they just clicked into — a confusing, silent UX bug (was introduced fresh in this commit; verified via `git show 8a88d3e -- frontend/src/App.tsx`, this whole navigation function is new).

Not covered by `App.test.tsx` (only Portfolio/Instruments nav additions were tested; there's no "open account elsewhere → navigate away → click Accounts nav" scenario).

**Suggested fix:** clear `selectedAccountId` whenever navigating *to* "accounts" from anywhere except by opening a specific account, e.g. drop the `&& activePageId === "accounts"` condition (any click on the top-level "Accounts" nav item should reset to the list view), or track "entered via account-open" vs "entered via nav-click" explicitly.

### ✅ Design-doc conformance

No deviations found from the referenced spec sections (0, A–G). Specifically verified:
- `buildMoneyChange` in `internal/domain/change.go` correctly allows any supported currency for holdings (composite) accounts while Simple stays locked.
- `OverviewResult.ByAccountType` (`internal/application/service.go`) is limited to `IncludeInNetWorth && !IsLiability`, matching §10.
- History-gate "return to original action" flow in `AccountActionSheets.tsx` and `StartHistoryForm.onStarted → setStartedInSheet` match the commit's described fix.
- Incomplete valuation is shown as "incomplete", never coerced to zero, in `AccountDetail.tsx`.
- All specific commit-message claims (file/line references) were spot-checked against the diff and hold true.

### 🧪 Test gaps

1. **No frontend test for a currency-mismatch error on a Simple account's deposit/withdraw path.** The Go layer has solid coverage (`TestCompositeCashAcceptsForeignCurrencyBeforeAndAfterHistory`, `TestPreviewMoneyAddedAllowsForeignCurrencyOnHoldingsAndRejectsOnSimple`), but no UI test asserts that picking a foreign currency for a Simple account's Deposit/Withdraw surfaces the backend's inline error while preserving user input (design §14 requirement). This is exactly the distinction slice 0 introduced, so a UI regression (e.g. accidentally locking/unlocking the currency selector) wouldn't be caught.
2. **No regression test for the navigation bug above.** `App.test.tsx` was touched in this commit; a test for "open account from Portfolio → navigate elsewhere → click Accounts nav → expect the list, not stale detail" would have caught it.

### 🎨 Nitpicks / code smells

1. **Float-ish percentage-to-bps conversion in the UI layer.** `frontend/src/features/accounts/accountCatalog.ts:136`:
   ```ts
   shareBps: Math.round(Number(percentages[index]) * 100),
   ```
   Not a money value per se (it's a ratio, and the Go backend independently enforces `sum(ShareBPS) == 10000`), but a user entering e.g. `33.33 / 33.33 / 33.34` can pass client validation and still get a generic rejection from the backend because of rounding. There's no live "must sum to 100%" hint in the wizard/settings form to guide the user before submit.
2. **`AccountsPage.tsx` list row falls back to "Complete" when valuation is still loading.** When `valuation` is `undefined` (not yet loaded), `valuation?.complete === false ? partial : complete` resolves to "Complete" rather than a neutral/loading state — a small misleading-status nitpick, not a data-correctness bug (the displayed amount itself correctly falls back to "No value").

---

## Commit `4e45733` — Require an explicitly checked Account owner before confirm or save (#9)

This commit is a correct, well-scoped fix. The full nil-vs-explicit-empty chain was traced end-to-end (JSON → `UpdateAccountRequest.Ownership` → `toApplicationInput` → `AccountInput.Ownership` → `UpdateAccount`'s preserve check → `resolveOwnership`) and confirmed correct: an **omitted** `ownership` field stays `nil` throughout (guarded in `internal/wailsapi/account/account.go` by `if r.Ownership != nil`), while an **explicit** `"ownership": []` becomes a non-nil, zero-length slice that is correctly rejected rather than mistaken for "leave unchanged."

This also fixes a real pre-existing bug: before this commit, `UpdateAccount` used `len(input.Ownership) == 0` (not a nil check), so `UpdateAccount({ownership: []})` was silently treated as "preserve existing ownership" instead of being rejected. Confirmed by diffing against the parent commit.

Verified: `go build ./...`, `go test ./internal/application/... ./internal/wailsapi/account/...` all pass (including the 6 new Go tests); frontend Vitest for `accountCatalog`/`AccountsPage` is 166/166 green, matching the PR's own reported numbers. The four touched doc sections (§7.4, §9.7, §16.1, §18) are mutually consistent, consistent with §14, and consistent with `en.json`/`zh-CN.json`/`zh-TW.json` and the implementation — no stale "leave blank to split evenly / default to household" language remains anywhere.

Frontend bypass concerns were also checked and are handled correctly:
- `AccountCreateWizard` is a plain `role="form"` `<div>` (no native `<form>`), so there's no Enter-key submit bypass of the disabled Continue button.
- `AccountForm` is a real `<form>`, but its `zod` schema (`ownerIds: z.array(z.string()).min(1)`) blocks `handleSubmit` from calling `submit()` with zero owners regardless of the Save button's `disabled` attribute — defense in depth.

No correctness bugs were found. Findings are limited to test-coverage gaps and minor nitpicks.

### 🧪 Test gaps

1. **No test verifies that omitting `ownership` on update preserves the *exact* existing multi-owner/custom split.** `internal/application/service_test.go` / `internal/wailsapi/account/account_test.go` only cover reject-on-create-with-zero-owners and reject-on-update-with-*explicit-empty*. The "omit → preserve" branch (`service.go` ~L662-664) is only indirectly exercised by the pre-existing `TestUpdateAccountSetFlagsPattern`, which uses a single 100%-owner account and never asserts the resulting `Ownership` value.
   *Suggested fix:* create a 70/30 two-owner account, update only the name (leave `Ownership`/`OwnerIDs` nil), assert the returned ownership is still 70/30.
2. **No test for the remainder/rounding split with a non-evenly-divisible number of checked owners**, in the exact function this commit added tests for. `frontend/src/features/accounts/accountCatalog.test.ts` only covers 0/1/2 owners; the remainder-distribution branch in `ownershipShares` (`accountCatalog.ts` ~L139-144, `index < remainder ? base + 1 : base`) is untested for e.g. 3 owners (10000/3 → 3334/3333/3333).
   *Suggested fix:* add a 3-owner test case asserting exact per-member `shareBps`.
3. **No JSON-unmarshal-boundary test** proving that an actually-omitted `"ownership"` key vs. an explicit `"ownership": []` decode to `nil` vs. non-nil-empty on `UpdateAccountRequest` — the real Wails-IPC path this whole feature depends on. All new/changed tests construct `UpdateAccountRequest{...}` as Go struct literals directly, bypassing JSON deserialization and relying on unverified (if standard) `encoding/json` slice semantics.
   *Suggested fix:* add `json.Unmarshal([]byte(`{"name":"x"}`), &req)` → expect `req.Ownership == nil`, and one with `{"ownership":[]}` → expect non-nil empty, mirroring the existing `TestAccountRecordDTORoundTripsAsJSON` pattern.

### 🎨 Nitpicks

1. **`accounts.ownershipSharePlaceholder` i18n key name is misleading.** The string is rendered as the label of the "use custom percentages" checkbox (`AccountCreateWizard.tsx` ~L424, `AccountForm.tsx` ~L345), not as an `<input placeholder>` (the percentage input itself uses a static `placeholder="%"`). The new English copy — "Optionally enter a percentage for each selected owner." — reads more like input placeholder text than a checkbox label, which could confuse future maintainers even though the Chinese copy reads fine as a label. Purely cosmetic.
2. **Pre-existing float usage sits right next to code this commit touches** (not introduced by this commit, flagged only because the PR checklist explicitly claims "Financial values do not use binary floating point"): `accountCatalog.ts`'s `ownershipShares` custom-percentage branch does `Math.round(Number(percentages[index]) * 100)` — floating-point arithmetic compensated by rounding, inconsistent with the Go side's `decimal`-based `PercentToBasisPoints`. Same underlying pattern as nitpick #1 in the #7 section above; worth a unified follow-up cleanup across both commits.

---

## Suggested follow-ups (priority order)

1. Fix the `App.tsx` `handleNavigate` stale-selection bug (#7) — small, user-visible, easy fix.
2. Add a regression test for that navigation scenario.
3. Add the three missing Go/TS test cases from #9 (omit-preserves-split, 3-way remainder split, JSON-unmarshal nil-vs-empty boundary).
4. Consider a shared decimal-safe percentage→basis-points helper on the frontend (mirroring `domain.PercentToBasisPoints`) to remove the two instances of `Number(...) * 100` float math, and add a live "percentages must sum to 100%" hint in the wizard/settings form.
5. Fix the "Complete" fallback for not-yet-loaded valuations in `AccountsPage.tsx` to show a neutral/loading state instead.
