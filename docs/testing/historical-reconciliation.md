# Historical reconciliation validation

Synthetic data only. Baseline: `94626d6fccf992c6a98a889c31a4b5fe3879e487`.
Design: [endpoint bridge](../development/historical-reconciliation.md).

## Accounting evidence

The new application tests establish:

- exact identity: total = sum(category contributions) + residual;
- corrected income at original economic time, normalized subsequent balance
  observations, later undo and reversed/same-day comparisons;
- neutral internal transfers and debt principal, fees and debt interest;
- USD income at later FX, stable holding price and opening FX without overlap:
  income 80 + price 32 + FX 120 = movement 232 CNY, with exact negation on reverse;
- an isolated SQLite CNY/USD transfer: movement 20 CNY, supported 0, residual
  20, with foreign exposure and imbalance reasons; reversed residual −20;
- liability FX signs, missing price/rate evidence, inclusion/state changes,
  zero-net uncertainty, quantity round trips and a 0.000000000001 residual;
- inverse FX division at existing precision and a 23-hour DST interval with
  start-exclusive/end-inclusive selection;
- input work-budget refusal, pre-cancellation, and immutable input serialization.

Existing overview/series tests continue to cover read-only SQL, shared captures,
missing provider coverage/backfill, civil-day discontinuities and cancellation.
The reconciliation uses the same batch, adding no queries or financial writes.

## UI evidence

Five new jsdom tests cover all three locales, exact tiny/negative amounts,
unknown versus zero, retained uncertainty, keyboard Enter/Space expansion using
the shared Button and four rapid date selections followed by stale replies.
The UI uses the existing capture hook, global tabs/date pickers and theme tokens.
No new chart or external provider is introduced.

## Gates and platform limits

Local Go toolchain: 1.26.8 installed under `/tmp`; the environment's original
`go` is a board-game executable. Dependencies and caches are isolated in `/tmp`.
Locked frontend dependencies were installed without changing the lockfile.

The canonical `go test ./...` ran all buildable packages successfully; only
`cmd/nestworth` could not build due to absent GTK4/WebKit6/libsoup development
headers. The documented native dependency installer cannot run because `sudo`
is absent. Wails' official generator succeeded with `GOFLAGS=-tags=server` and
`CGO_ENABLED=0`; its check verified identical generated output. Targeted reconciliation race tests passed. Server-mode Go
build/vet, frontend build/type checking, lint, the user-skill checker and all
53 Python tooling tests passed. Bindings and bundles are generator outputs and
are not included in the feature commit. CI on the final PR head remains the
canonical desktop Go build/full race and frontend gate; inspect the PR checks
for their exact SHA and final results.

Native macOS GUI, VoiceOver, window focus and visual acceptance were **not
observed** in this cloud Linux session. jsdom interaction tests are synthetic
and are not native or browser visual verification. No packaging, release,
merge, real financial data or external agent contact was performed.
