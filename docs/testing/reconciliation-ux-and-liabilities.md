# Reconciliation UX and liability regression coverage

Use synthetic accounts and isolated ledgers only. This work does not establish
native GUI or browser visual acceptance from code, jsdom, compilation or CI alone.

| Area | Regression coverage |
| --- | --- |
| Account groups | Complete requires nonempty current confirmed items and completeness; unfinished first; manual keyboard toggles; deferred move after dialog closure; refetch/session state; focus continuation |
| Item groups | Pending/unresolved and completeness visible; skipped separate; confirmed collapsed; refetch/manual expansion; recheck resurfaces |
| Snapshot and preview | Exact tiny quantity, cash currency, product principal, scope context; distinct ledger/source; current action foreground; unrelated context collapsed; mismatch warnings visible |
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
go test ./internal/application -run 'Test(Reconciliation|Liability|AnalysisCanonical|AnalysisOrdinary|Overview)'
cd frontend
pnpm run typecheck
pnpm run lint
pnpm run test
pnpm run build
```

Tests are coverage definitions, not claims that every gate or every screen was
executed. Record exact commit/check results in the PR. Browser screenshots were
not available in the saved executor's supported browser-control tools; do not
substitute the user's private screenshots as test artifacts. Native macOS GUI
acceptance requires the documented isolated development identity and database.
