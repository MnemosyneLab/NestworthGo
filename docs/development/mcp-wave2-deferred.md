# MCP Wave 2 deferred items

D and E3 from [mcp-remaining-work.md](mcp-remaining-work.md) are done on
this branch.

## D — investment review default exposure

Done. Product choice is option 1: a settings switch, default off.

The nine investment-review reads stay in every MCP mode. The seven management
tools (`create_investment_review_rule`, `update_investment_review_rule`,
`archive_investment_review_rule`, `acknowledge_investment_review_alert`,
`set_investment_review_suppression`, `clear_investment_review_suppression`,
`refresh_investment_reviews`) are absent from discovery until
`investmentReviewManagement` is enabled, and they still require
`ledger_write`. There is no new permission mode. The desktop review screen
does not use the switch. How to turn it on is in
[mcp.md](mcp.md#investment-review-management).

## E3 — one PATCH rule for every update tool

Done. E1 is on this branch: optional pointers and slices no longer publish
`"type": ["null", ...]` unions. Patch tools now share one rule. Omit a field
to keep it. JSON null clears a clearable field. An empty string is rejected,
so it neither clears nor keeps. `update_account` no longer publishes
`<field>Set`. `update_instrument` no longer publishes `replace`.
`update_instruments_batch` clears `shortName` with null, not with an empty
string. Directory name, icon and `archived` still cannot be cleared. Review
rules, liquidity saves and product terms stay full replacements;
`terms.shortName` still treats omitted and null as keep. The inventory and
the migration are in [mcp-update-conventions.md](mcp-update-conventions.md).
