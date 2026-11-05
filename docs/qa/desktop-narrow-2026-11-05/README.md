# Narrow desktop layout regression

The latest fetched main was `843c375d88c7b3b857514f54d46779aa7e376733`
(the merged #238). Its desktop UI still clips overview titles and financial
values and overlaps account names with amounts at narrow widths. This establishes
an issue on that main revision, **not** that #238 introduced it: no pre-#238
baseline was used to attribute causality.

This test covers the responsive **desktop** React UI (`AppShell`, `OverviewPage`,
`AccountsPage`) in Chromium device-size viewports. `src/mobile` / `mobile.html`
is a separate mobile frontend and is not changed or claimed as tested here.

## Reproduction and evidence

Run from the repository root with installed frontend dependencies:

```sh
PLAYWRIGHT_MODULE=/absolute/path/to/playwright/index.mjs \
CHROMIUM_PATH=/usr/bin/chromium \
QA_OUTPUT=/tmp/nestworth-desktop-narrow \
node docs/qa/desktop-narrow-2026-11-05/browser.mjs
```

The script creates and removes its own temporary frontend entry point. It uses
real page, shell, navigation, theme, localization and shared control components;
queries and unrelated child features are substituted. No native app, database,
real household, provider or credential is accessed. This is frontend layout
coverage, not full-stack or native acceptance.

The matrix is 320 / 390 / 1440 CSS px × en / zh-CN / zh-TW × light / dark ×
16 / 32px root font (100% / 200% text sizing), 36 combinations. Long unbroken
Latin and Chinese account names and institution names accompany 13-digit amounts,
including fractional digits and a negative overview liability. The script checks
text bounds against their containers and viewport, name/amount non-overlap,
complete amount/name/translated badge text and keyboard Enter account selection.
200% root sizing also enlarges rem spacing; this is not a claim about native OS
text zoom or physical devices. Vertical scrolling is expected for long values.

- Baseline: **30 / 36** combinations failed, zero page errors. See
  [before.json](evidence/before.json), [overview](evidence/before-overview-320.png)
  and [accounts](evidence/before-accounts-320.png).
- Fixed: **36 / 36** combinations pass, zero page errors. See
  [after.json](evidence/after.json), [320 overview](evidence/after-overview-320.png),
  [320 accounts](evidence/after-accounts-320.png),
  [390 accounts](evidence/after-accounts-390.png),
  [1440 overview](evidence/after-overview-1440.png), and
  [1440 accounts](evidence/after-accounts-1440.png).

The baseline was repeated in a detached worktree using the same synthetic
financial fixture after correcting its account type to the supported enum.
Final assertions additionally cover header buttons and exact text. Screenshots
were visually reviewed at 320/390 and 1440; the wide account row retains its icon,
name/badge and right-aligned amount arrangement. Values wrap rather than truncate.

## Checks and boundaries

Linux, Chromium 154.0.8037.57, Node 24.22.0, Go 1.27.3 and Wails v3.1.0.
Locked dependencies were installed with pnpm 11.25.0 (repository declaration is
12.14.0); no manifest or lockfile changes. Wails bindings were generated once with
`CGO_ENABLED=0 GOFLAGS=-tags=server` using the repository guard, then the equivalent
underlying frontend tools were run without redundant regeneration:

- `node node_modules/eslint/bin/eslint.js .`: PASS.
- `node node_modules/typescript/bin/tsc --noEmit`: PASS.
- `node node_modules/vite/bin/vite.js build --mode production`: PASS; existing
  large ECharts vendor chunk warning remains.
- Focused `PageChrome` / `MoneyText` Vitest checks: 4 / 4 PASS.
- Full Vitest suite: pending at report preparation; final result is in the PR.
- `git diff --check`: PASS.

Native macOS WebView, native packaging, full-stack ledger flows and OS-level
accessibility acceptance were not run. No financial calculations or API contracts
change. The changes are CSS layout and an opt-in `MoneyText wrap` presentation
prop; no new dependency, polling or measured performance claim.
