# Production App navigation focus check

Captured 2026-10-16 in Chromium at 390×844 from `frontend/preview/app-interaction-audit.html`. The fixture mounts the production `App` and `AppShell`. Real `MarketDataPage`, `InstrumentManagement`, and `InstrumentDetailPage` handle the interaction; TanStack Query is preloaded with synthetic values, so no ledger or native Wails service is used.

Keyboard sequence: Data maintenance → search `One` → Details → Data records → Previous period → Maintain data → Back to Instrument details → Back to Data maintenance.

The quote period was `2026-08-17 – 2026-09-16`. It appeared in the maintenance focus card and remained selected after returning to the instrument page. After the first Back, `document.activeElement` was `#instrument-detail-maintain` and matched `:focus-visible`. After the second Back, the search still contained `One`; focus returned to the original Details button and matched `:focus-visible`.

## Issue reproduced before the fix

Before removing the focus-dependent `MarketDataPage` key, the same real-App route returned with an empty search and `document.activeElement` at `BODY`. The focused editor navigation changed the key, remounting `MarketDataPage`; its `useState` search value was lost. The detail page also had no stable Maintain button ID, so App's existing element-or-ID restore path could not find its trigger after the detail page unmounted.

## Fix and expected behavior

- Keep the existing Market Data page instance mounted while its focus prop changes. This preserves search and tab state across Details → Maintain → Back.
- Give the Maintain and Related transactions buttons stable IDs. App can resolve the matching trigger after the instrument-detail page has unmounted and remounted.
- Make `main-content` programmatically focusable with `tabIndex={-1}` so it works as the existing fallback when a source trigger cannot be resolved.

`App.test.tsx` has a regression test for the two-Back search/focus round trip. The browser evidence above uses real App routes rather than the component-only preview navigation wrapper.
