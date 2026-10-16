# Post-change interaction Web preview (synthetic)

These screenshots were captured on 2026-10-16 from the actual React components through `frontend/preview/interaction-audit.html` in Chromium at 1440×960. The fixture seeds TanStack Query in memory with a synthetic household, account, instrument, and holdings. It never connects to a ledger or native Wails service. These are Web previews, not native macOS acceptance screenshots.

- [Investment holdings](holdings.png): grouped investment assets and the dated existing-holding entry point.
- [Existing position import](position-import.png): the shared dated command form with account, instrument, quantity, cost, local date and time, and preview.
- [Instrument detail, household](instrument-household.png): shared detail with household holding totals.
- [Instrument detail, archived account](instrument-archived-account.png): account-scoped historical holding amounts from the account query.

Run the frontend Vite dev server and visit `/preview/interaction-audit.html?view=holdings`, `?view=detail`, or `?view=archived` to inspect the fixture interactively. The hard-coded fixture IDs and names are synthetic.
