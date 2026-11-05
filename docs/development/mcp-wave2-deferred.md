# MCP Wave 2 deferred items

E3 from [mcp-remaining-work.md](mcp-remaining-work.md) stays deferred.
D is implemented on this branch.

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

Deferred until the E1 nullable-schema change has been in use. E1 is on this
branch: optional pointers and slices no longer publish `"type": ["null", ...]`
unions, while required pointers, `shortName`, and `update_instruments_batch`
`note` still accept null through `anyOf`. The current update tools still
differ (`update_account` `<field>Set`, `update_instrument` `replace`, and the
directory updates' omit-versus-value rules). Collapsing those into
"omit keeps, null clears" is a breaking contract. This pass does not redefine
them.
