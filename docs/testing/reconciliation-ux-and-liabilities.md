# Reconciliation UX and liability regression coverage

Use synthetic accounts and isolated ledgers only. This work does not establish
native GUI or browser visual acceptance from code, jsdom, compilation or CI alone.

| Area | Regression coverage |
| --- | --- |
| Account groups | Complete requires nonempty current confirmed items and completeness; unfinished first; manual keyboard toggles; deferred move after dialog closure; refetch/session state; focus continuation |
| Item groups | Pending/unresolved and completeness visible; skipped separate; confirmed collapsed; refetch/manual expansion; recheck resurfaces |
| Snapshot and preview | Exact tiny quantity, cash currency, product principal, scope context; distinct ledger/source; current action foreground; unrelated context collapsed; mismatch warnings visible |
| Item-scoped refresh | Two holdings/two currencies; buy/sell/fees/dividend/cost/FX/transfer/reversal endpoints; net-zero and backdated events; unchanged confirmations/attempts; new inventory reopens completeness; stale whole-session plans still refused |
| Whole account | Explicit completeness, optional reference/date controls, one preview/commit; existing evidence preserved; per-item attempts; idempotent receipt; stale refusal |
| Supplement | Fixed account and exact holding; standalone selectors retained; event switch resets; transfer direction/counterparty; exact dividend association; immutable mutation identity |
| Liabilities | Nonnegative persisted magnitudes; purchase/refund/repayment/correction semantics; insufficient debt/overpayment refusal; Spending rather than Income attribution |
| Account totals | Signed list/detail and normalized institution net/sort; all-liability/negative net; incomplete price/FX suppress subtotal; caller's archived/filter scope |
| Related analysis | Canonical property/other-liability filter and signed values; non-investment property excluded from investment cash universe; known ordinary undo cancels semantic buckets |
| Historical presentation | Debt negatives in account cells, trend, category comparison, panorama and overview; unsigned API magnitudes retained; no double inversion of net/asset-only categories |

Key commands (run with the documented toolchain and native dependencies):

```sh
go test ./internal/domain ./internal/wailsapi/catalog ./internal/wailsapi/portfolio
# Affected application cases; full native CI remains a separate gate.
go test ./internal/application -run 'Test(Reconciliation|Liability|AnalysisCanonical|AnalysisOrdinary|AnalysisIncludeCash|Overview)'
cd frontend
pnpm run typecheck
pnpm run lint
pnpm run test
pnpm run build
```

Tests are coverage definitions, not claims that every gate or every screen was
executed. Record exact commit/check results in the PR. Browser evidence must distinguish synthetic shared-component fixtures from full
application/IPC workflows. Do not substitute private screenshots for test
artifacts. Native macOS GUI acceptance requires the documented isolated
development identity and database.

## Requirement verification map

“Implemented” describes the code contract. Test names identify executable evidence;
execution results and the exact tested head belong in the PR validation record.

| Requirement | Implementation and regression evidence | Remaining acceptance boundary |
| --- | --- | --- |
| Account sections and truthful completion | `accountIsVerified`; `ReconciliationAccountGroups.test.tsx` covers nonempty completeness, skipped/unresolved, deferred collapse, manual reopening, session reset and focus continuation | Native scroll/focus acceptance |
| Status groups and resurfacing | `ReconciliationItemGroups.test.tsx` covers pending/unresolved first, separate skipped, collapsed confirmations, retained manual state and affected-only resurfacing | Native rendering |
| Authoritative checkpoint and precision | `LedgerCheckpoint.test.tsx`; real page CheckForm uses frozen ledger values, units, source dates, product context and exact audit formatting; no provider refresh | Full desktop presentation |
| Focused preview and recovery | `ReconciliationPage.test.tsx` covers collapsed prior context, visible mismatch warnings, expiry/conflict return, nested Escape and same-plan unknown-outcome retry | Process/window-close behavior |
| Explicit whole-account confirmation | `TestReconciliationAccountConfirmation*` covers explicit completeness, atomic attempts, retained exceptions, idempotency and stale refusal; no inactivity-based confirmation | Optional save-and-next is not implemented |
| Supplement identity and operation scope | `ReconciliationRecordFlow.test.tsx`, concrete-holding/link backend tests and AccountsPage import regression retain exact identities, fixed primary account and counterparty-only transfers | Full desktop IPC interaction |
| Card transaction semantics | Domain and application liability purchase/refund/repayment tests retain positive magnitudes, spending/refund classification and overpayment refusal; corrections and fees stay distinct | Native labels in three locales |
| Signed liability balances | Archived metadata, loading/error, attempts/linked results, trend summary/chart/table and history direction regressions; raw frozen evidence remains unchanged | Full native historical-screen sweep |
| Net grouping and incomplete totals | `accountGroups.test.ts` covers signed institution totals/order, all-liability, zero, currency, missing/incomplete and caller-filtered scopes; asset-only breakdowns retain their contract | Native list/detail presentation |
| Canonical simple-account classification | `TestAnalysisCanonicalSimpleAccountTaxonomyAndLiabilitySigns` and `TestAnalysisIncludeCashDoesNotDiluteETFReturnWithProperty` keep property/debt outside investment cash | No live-provider evidence required |
| Ordinary undo attribution | `TestAnalysisOrdinaryUndoCancelsOriginalSemanticBucket` and `TestAnalysisOrdinaryTradeUndoUsesInverseSide` cover income/expense/interest, buy/sell, cash, fees and immutable source facts | Missing original history keeps conservative existing semantics |
| Pagination, cancellation and repeated actions | Page/App regressions cover clamping, session reset, truthful All filter, nested navigation, cancelled/late previews and immutable retry identity | Native keyboard/window lifecycle |
| Item-scoped financial invalidation | `TestReconciliationRefreshOnlyChangedEndpoints` covers two holdings/two cash currencies, both transfer endpoints, net-zero buy+undo, new membership, metadata/quotes, persistence and unchanged attempts; mixed product test preserves other confirmations | Unknown legacy evidence uses explicit conservative fallback |
