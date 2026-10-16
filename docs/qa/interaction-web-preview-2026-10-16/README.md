# Post-change interaction Web preview (synthetic)

These screenshots were captured on 2026-10-16 from the actual React components through `frontend/preview/interaction-audit.html` in Chromium at 1440×960. The fixture seeds TanStack Query in memory with a synthetic household, account, instrument, and holdings. It never connects to a ledger or native Wails service. These are Web previews, not native macOS acceptance screenshots.

- [Investment holdings](holdings.png): grouped investment assets and the dated existing-holding entry point.
- [Existing position import](position-import.png): the shared dated command form with account, instrument, quantity, cost, local date and time, and preview.
- [Instrument detail, household](instrument-household.png): shared detail with household holding totals.
- [Instrument detail, archived account](instrument-archived-account.png): account-scoped historical holding amounts from the account query.
- [Narrow maintenance flow](interaction-maintenance-narrow.png): actual archived instrument detail opens Market Data with the instrument name and selected date period, then searches instruments, switches to FX rates and back, and checks keyboard focus at 390×844.
- [Narrow archived return](interaction-archived-return-narrow.png): the actual detail component returns to the archive account scope after maintenance at 390×844.

Run the frontend Vite dev server and visit `/preview/interaction-audit.html?view=holdings`, `?view=detail`, `?view=archived`, or `?view=market` to inspect the fixture interactively. Add `&from=2026-09-01&to=2026-09-30` to the archived view to replay the dated maintenance path. The hard-coded fixture IDs and names are synthetic. The browser flow uses real React page components with cached in-memory fixtures; it does not exercise native Wails or macOS focus restoration.
