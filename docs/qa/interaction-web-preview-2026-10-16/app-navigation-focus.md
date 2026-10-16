# Production App navigation focus check

Captured 2026-10-16 in Chromium at 390×844 from `frontend/preview/app-interaction-audit.html`. The fixture mounts the production `App` and `AppShell`. Real `MarketDataPage`, `InstrumentManagement`, and `InstrumentDetailPage` handle the interaction; TanStack Query is preloaded with synthetic values, so no ledger or native Wails service is used.

Keyboard sequence: Data maintenance → search `One` → Details → Data records → Previous period → Maintain data → Back to Instrument details → Back to Data maintenance.

The quote period was `2026-08-17 – 2026-09-16`. It appeared in the maintenance focus card and remained selected after returning to the instrument page. After the first Back, `document.activeElement` was `#instrument-detail-maintain` and matched `:focus-visible`. After the second Back, the search still contained `One`; focus returned to the original Details button and matched `:focus-visible`.

After closing that focused editor, the real App also reopened the same `Fund One` editor on a new Maintain request, then opened `Fund Two` after the search changed. This confirms editor dismissal is scoped to a navigation request rather than the mounted maintenance component. The App regression test covers both same-instrument and different-instrument requests.

For FX routing, the real App started on Instruments, navigated through Data Health's synthetic CNY/USD “Add manual data” action, and returned with FX rates selected. The screenshot is [app-fx-focus-narrow.png](app-fx-focus-narrow.png). This verifies the navigation target changes the active tab while preserving user-owned tab state for ordinary returns.

## Nested Maintain → Details → Maintain Back regression

Reproduction sequence: Details → Maintain (R1) → close editor → open Details from the focused maintenance list → Maintain (R2) → close editor → Back to Details → Back to R1 maintenance. Before the fix, returning to the detail route already exposed one editor dialog and its Name field from the hidden, still-mounted maintenance page; the modal portal could intercept Back.

Editor and manual-price overrides are now stored per navigation request ID, so closing R2 cannot overwrite R1's dismissed state. `App` also passes the current route activity to `MarketDataPage`, which suppresses an editor while that page is hidden. Replaying the full sequence in the real App produced zero dialogs on the detail route and on restored R1 maintenance, while preserving search `One`. See [app-nested-back-narrow.png](app-nested-back-narrow.png). The App regression test covers the nested Back, closed-state restoration, and a later same-instrument Maintain request.

## Issue reproduced before the fix

Before removing the focus-dependent `MarketDataPage` key, the same real-App route returned with an empty search and `document.activeElement` at `BODY`. The focused editor navigation changed the key, remounting `MarketDataPage`; its `useState` search value was lost. The detail page also had no stable Maintain button ID, so App's existing element-or-ID restore path could not find its trigger after the detail page unmounted.

## Fix and expected behavior

- Keep the existing Market Data page instance mounted while its focus prop changes. This preserves search and tab state across Details → Maintain → Back.
- Give the Maintain and Related transactions buttons stable IDs. App can resolve the matching trigger after the instrument-detail page has unmounted and remounted.
- Make `main-content` programmatically focusable with `tabIndex={-1}` so it works as the existing fallback when a source trigger cannot be resolved.

`App.test.tsx` has a regression test for the two-Back search/focus round trip. The browser evidence above uses real App routes rather than the component-only preview navigation wrapper.
