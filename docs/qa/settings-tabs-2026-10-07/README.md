# Settings tabs cloud verification

Initial implementation base: `f23a4fb` (main). This is a separate settings redesign branch. PR34 was
still open at `f94a706` when the base was checked; its version/Wails/bindings
changes are not included. The earlier design-review screenshots were from
`0618f42` on PR34, whose `frontend/src` matched this main baseline, not from
this implementation.

## Implementation choices

- Six tabs reuse the Return Analysis Tabs components: Appearance, Market data,
  AI · MCP, Data, Diagnostics, About. Labels do not shrink or wrap; the tab
  strip scrolls horizontally and focused tabs scroll into view.
- Panels stay mounted. A single permanent preference form and in-memory draft
  serve every tab, including Save from About. Provider secrets, MCP permission
  changes, and R2 settings retain their independent actions; no nested forms.
- Hidden panels use Base UI's hidden/inert semantics. Agent status/operations,
  continuous-backup status and retention status disable their queries and
  five-second polling while hidden. Mutation observers, inputs, selected
  recovery point and retention days remain mounted, preserving busy/results.
- SecretField display state is recreated on exit/re-entry; the replacement
  values remain in component memory and start masked. MCP plaintext connection
  configuration is collapsed on exit; the unapplied permission mode remains.
  No secret draft is added to a persistent store.
- Tabs are locked through local/cloud restore inspection, confirmation and
  installation, retention preview/confirmation/cleanup, and MCP actions.
  Inspection locks immediately before submitting so a deferred success cannot
  open a Portal from a hidden tab. Ordinary backup/configuration operations
  can continue across switches with their state retained.
- Restore cancellation clears frontend preview/acknowledgment and returns
  focus to the initiating button. No backend preview-cancel API exists:
  `application.RestorePreviewTTL` is 15 minutes; retained staging files are
  expired/evicted by the existing backend. Cancellation does not install data.
- Restore all preference defaults is a page-level secondary action. It calls
  Reset directly after confirmation, discards preference drafts across tabs,
  and displays Reset's returned authoritative snapshot without another Save.
  Reset covers appearance, accent, language, timezone, week start and date/time/number formats,
  display currency, window dimensions, FX routing, quote-cache TTL and logging.
  Existing provider keys, household/ledger data, MCP and R2 are preserved.
- The footer is a normal-flow sibling of Tabs. It grows with text/errors and
  is reached by scrolling at low height; it never overlays panel content.
  Form grids and MCP rows respond to available container width, including
  after the sidebar is expanded.

## Automated evidence

- Full frontend suite: 85 files / 743 tests passed with `--maxWorkers=2`.
- Latest settings components and keyboard-only suite: 7 files / 60 tests passed,
  including an additional cross-tab recovery-point/retention-days regression.
- Lint and TypeScript checks passed; production frontend build passed.
- Aggregate `wails3 task check` passed the generated-bindings comparison,
  gofmt, Go tests, vet, Go build, frontend lint/typecheck/build. Its first
  frontend run had an obsolete settings-navigation test (updated here) and
  three timing failures in unchanged instrument tests. The instrument file
  independently passed all 21 tests; the full suite passed with two workers.
- First Go run hit the existing cold-analysis performance budget during
  concurrent compilation. The aggregate's subsequent Go run passed.
- Portable skill checker and 19 installer tests passed.

Local Wails binding generation and Go aggregate checks used the pinned
beta.26 CLI, `GOFLAGS='-mod=readonly -tags=server'`, and `CGO_ENABLED=0`.
The cloud image lacks GTK4/WebKit6 headers and sudo. A CGO/race attempt reports
those missing headers; native CI is the authoritative native/race gate.
Generated binding differences already present on main were excluded from this
PR, leaving PR34's bindings/version work separate.

## Browser evidence and limits

`geometry.json` records 108 layouts: three languages × three viewport sizes
(1400×900, 800×420, 640×600) × both sidebar states × six panels. It checks one
visible panel, nonwrapping/nonsqueezed labels, no main horizontal overflow or
outlying visible controls, static footer placement after the panel, and 18
keyboard End-selection/scroll cases. A synthetic long save failure produced a
402px footer; the save action remained reachable by scrolling to the bottom.

Screenshots render the actual SettingsPage, AppShell, theme and CSS in cloud
Chromium, using a temporary mock-services harness and synthetic values. No
real database, provider keys, MCP server or R2 service was used. Narrow-window
`*-top.png` images show the tabs/top of the panel; matching unsuffixed images
show the footer. The long-error images show both the start and bottom of the
expanded footer. A 640px expanded-sidebar MCP selector overflow was found and
fixed during this matrix pass; this is not a claim of an earlier 800px
navigation overflow.

These are frontend browser checks, not Wails/macOS GUI acceptance. Remaining
parent-coordinated checks: isolated Mac file picker, native Enter/focus behavior,
local and cloud restoration dialogs on delayed success/error/cancellation,
low-height scrolling, and sidebar changes with native WebKit. Keep the PR in
draft for strict review; do not merge, tag, release or use a real ledger/R2.

## Follow-up retention navigation review

After PR34 merged, main `84715934864d9f5e07477ee184db9fbb6bab6204` was merged
into this branch without rewriting its history. Its Wails beta.28/version
changes now come from main, rather than from edits to PR34's branch. The
screenshots above still describe the original implementation and mock data.

Review reproduced a navigation deadlock: a delayed or cached retention status
with `running: true` locked every inactive tab even when Data was hidden and
its polling had stopped. Retention now reports two independent states:
conflicting Data controls stay disabled during background cleanup, while the
page navigation lock follows only frontend requests and confirmation dialogs.
Users can return to Data, resume polling and use Stop cleanup. Restore
inspection/confirmation/restart protection remains unchanged.

If background cleanup starts while a retention confirmation is open, Cancel
can close it when no frontend request is pending. Focus returns to the
initiating control when enabled, or the retention heading if background work
has disabled that control.

Three new integration regressions cover delayed hidden-panel status, cached
running status on reopening Settings, and confirmation cancellation/focus
during background cleanup. The first two failed before the fix. The relevant
settings/retention/continuous-backup/keyboard suite passed 49 tests in 4 files,
including the existing deferred local and cloud restore safety checks.

Follow-up validation with pinned Wails beta.28: binding verification, gofmt,
Go tests/vet/build, frontend lint/typecheck/build and `git diff --check` passed
using the same local server/CGO=0 setup. The aggregate default four-worker
frontend run passed 746/747 tests; its only failure was the unchanged
ProductPickers date-selection test exceeding its five-second budget. A full
two-worker rerun passed all 747 tests in 85 files. Native/race CI is linked
from PR35; isolated Mac GUI acceptance remains separate.
