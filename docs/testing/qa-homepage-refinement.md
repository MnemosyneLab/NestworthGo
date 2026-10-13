# Homepage refinement acceptance — 2026-10-13

Baseline: `36fa9035267bc48dde0e087cd4e5f99bf3610600` (main at checkout).
Only presentation and synthetic UI tests changed. Financial calculations,
read models, backend services, dropdown labels and generated bindings were not
modified. The private reference image stayed outside the repository; no image
or financial screenshot was added to this change.

## Browser evidence

Headless Chromium rendered the actual OverviewPage, AppShell, shared controls,
fonts and theme CSS with synthetic query responses. Existing trend and
composition charts were stubbed out for this focused check; this is not a
native desktop or chart acceptance run. Main and the changed page used the
same fixtures and styles.

First-row measurements matched main in both light and dark themes (rounded to
0.01 CSS pixels). Numeric styles were unchanged:

| Viewport width | Net-worth card width × height | Liquidity card width × height | Net-worth font | Assets/liabilities font | Liquidity fonts |
| --- | --- | --- | --- | --- | --- |
| 1440 | 552.5 × 234 | 552.5 × 234 | 48px | 18px | 24px |
| 800 | 657 × 234 | 657 × 233 | 48px | 18px | 24px |
| 480 | 353 × 306 | 353 × 337 | 36px | 18px | 24px |

The lower row placed Needs attention and Next 30 days side by side at wide
widths and stacked them at narrow widths. The top Historical overview shortcut
was hidden at 960px and wider; it and the accessible sidebar rail destination
remained available in narrow windows.

The 24 state/size cases covered empty actions, no accounts, opt-in, enabled
without pending alerts, real health/review/scheduled items, partial valuation
and liquidity, loading, errors, long labels with Traditional Chinese, ledger
next steps, paused reminders and insufficient/stale NAV data. There were no
browser runtime exceptions or horizontal overflows. Blocking, warning and
information health labels retained their source severity; loss reminders
retained their destructive status. Distinct report read times/timezones stayed
visible. No zero-count View all action appeared. Browser Enter activation
opened the health target and the below-fold History destination.

## Automated checks and limits

Frontend regressions cover all three supported locales, source timestamp
formatting, monitoring states, actual review conditions, severity, retry,
keyboard navigation, partial amounts and the preserved financial/action-row
separation. Run lint, typecheck, tests and build with the pinned bindings guard.

This Linux container lacks GTK4, WebKitGTK 6, libsoup and gio development
headers. Canonical native compilation was attempted and failed on those
missing dependencies. The headless configuration used
`GOFLAGS='-mod=readonly -tags=server'` and `CGO_ENABLED=0` for binding generation
and Go checks. Passing that configuration is not proof of a native GTK or macOS
GUI build. No native app was launched or installed, no real database was used,
and macOS packaging/accessibility acceptance remains a separate manual gate.

Completed automated evidence:

- Frontend lint, TypeScript typecheck and production build passed.
- Full frontend suite passed: 1173 tests across 108 files with
  `pnpm exec vitest run --maxWorkers=1`. The first concurrent run hit unrelated
  interaction deadlines; the full bounded rerun passed after fixture updates.
- Generated binding freshness passed; bindings and bundles remain ignored.
- Headless `go test -p 1 ./...`, `go vet ./...` and `go build ./cmd/nestworth`
  passed with the configuration above.
- `gofmt -l cmd internal` was empty and `git diff --check` passed.
- User-skill validation and all 53 Python repository checks passed (including
  synthetic installer tests; these did not install native dependencies).
- Local race detection, native GUI accessibility and macOS packaging were not
  run. They remain CI/manual gates.

## Review follow-up: shared Available Funds entry

The upcoming card defaults to its standard presentation, retaining Available
Funds' investment-review query, summary and setup/navigation entry. Overview
explicitly opts into the homepage presentation and renders one compact review
block in Needs attention. Standard callers also retain their original heading,
window text and attention preview.

Follow-up validation passed: 92 targeted tests across Available Funds,
Available Funds navigation, Overview, upcoming actions and investment review;
frontend lint, typecheck and build; and the same synthetic Chromium matrix and
first-row geometry comparison. New Available Funds regressions cover empty
upcoming results with paused, not-enrolled, enabled and alerting review states
in all three locales, including keyboard access to review settings. The 1173-test
full-suite result above belongs to the initial implementation; this follow-up
reran the affected suites.
