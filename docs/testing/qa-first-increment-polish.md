# First increment polish QA

Synthetic fixtures only. This increment builds on merged PR 52 (`d687e23`)
and the existing upcoming-actions projection. It changes no schema, backup,
sync, release artifact or user ledger.

## Acceptance behavior

- Repeat allows only ordinary one-account `cash_in` reasons income, interest,
  contribution, gift and other, and `cash_out` reasons expense, fee, tax,
  interest and other. It copies editable defaults without copying identities or
  the original time. The domain retains its existing reason classifications.
  Dividend detail, holding attribution, products, transfers, trading,
  reconciliation, correction and reversal remain excluded. A reversal hidden
  by History filters or pagination still blocks preview and confirmation.
  Completed confirmations replay the same receipt. An uncertain retained
  confirmation has a distinct retry label; a fresh preview is a new operation.
- Upcoming main totals satisfy `total = attention + scheduled`. Past blocked
  items count as attention, alongside receipts to verify and reservation
  issues. Eligible items and data checks stay separate. One managed lifecycle
  has one item across maturity, eligibility and receipt dates. Unknown native
  amounts remain unknown, and this view computes no cross-currency total.
  Merged checks retain both terms/rules and reservation navigation targets.
- Receipt navigation rereads the Overview source and product permission/state,
  then opens a receipt draft only when due and permitted. It does not post.
  A locked product leaves actual gross proceeds blank; only a term deposit
  proposes its principal as returned principal. Manual actions, edits,
  cancellation and closing consume a pending receipt intent so a late Product
  response cannot overwrite or reopen the user's draft.
  Settlement removes the item, undo restores it, and interest receipt alone
  does not settle it. One-shot request IDs prevent replays on Back; canceled or
  superseded async navigation cannot open a stale target.
- History sentences, details and account cash history show the exact recorded
  decimal alongside currency rounding when subminor digits are nonzero. This
  includes the signed canonical value for negative changes and zero-scale
  JPY/KRW. Reconcile/Add cash focus
  returns to the trigger, or the account Back control when the trigger is gone.
  Repeat drafts ignore late preview responses after editing or closing.

## Automated and manual boundaries

Domain and API tests cover reason exclusions, hidden reversals, stale gates,
replay, calendar boundaries, deterministic priority, blocked past items,
merged dual targets, reservation and lifecycle projection. UI tests cover
navigation requests, late async results, all three locales, exact decimals,
focus, draft retention and narrow-layout wrapping classes. Server-profile Go
tests and jsdom exercise code and DOM behavior; neither proves native Wails
IPC, WebKit focus or actual small-viewport rendering. The parent will arrange
independent Linux GUI acceptance after review fixes. No release action is in
scope.

## Candidate checks

Run from the feature branch in the saved Cloud workspace with the repository's
server build tag and `CGO_ENABLED=0` where appropriate:

- `go test -p 1 ./...` — passed across all packages.
- `pnpm run test --maxWorkers=1` — passed, 90 files / 859 tests. An earlier
  complete run exposed one stale navigation fixture (852 passed, one failed);
  the fixture was corrected and the complete rerun passed.
- `pnpm run lint`, `pnpm run typecheck`, `pnpm run build`, and
  `pnpm run check:bindings` — passed. An initial lint run found a hook
  dependency error; it was fixed before the passing rerun.
- `go vet ./...`, server `go build ./cmd/nestworth`, and `git diff --check` —
  passed.
- `go test -race -short -p 1 -parallel 4 ./internal/domain
  ./internal/application ./internal/wailsapi/history
  ./internal/wailsapi/liquidity -run 'Repeat|Upcoming|CashBalance' -count=1`
  — passed in all four packages.

Native Wails/GTK/WebKit GUI acceptance was not run in this Cloud workspace;
the parent is arranging Linux GUI acceptance separately.

## Scope deliberately deferred

Transfer, dividend, repayment, trade and FX Repeat; ordinary-source fixed
receipt expansion; renewal UI; early-route reminders; balance/manual-value
flow expansion; templates; automatic execution; bill/payroll/coupon
schedules; OS notifications; snooze/read state; schema/backup/sync changes.
