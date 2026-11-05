# MCP Wave 2 deferred items

This pass does not implement D or E3 from
[mcp-remaining-work.md](mcp-remaining-work.md).

## D — investment review default exposure

Deferred. The 16 `investment_review` management tools stay registered for
`ledger_write` as they are today. A2's measured tasks were net worth, a buy,
a reconciliation and a fund NAV import. That script did not call review
tools, and no model host was attached, so there is still no usage data for
hiding or re-permissioning them. Choosing a capability switch, a grouping-only
change, or a new write mode needs that product evidence. It also touches
permission modes, which this pass does not change.

## E3 — one PATCH rule for every update tool

Deferred until the E1 nullable-schema change has been in use. E1 is on this
branch: optional pointers and slices no longer publish `"type": ["null", ...]`
unions, while required pointers, `shortName`, and `update_instruments_batch`
`note` still accept null through `anyOf`. The current update tools still
differ (`update_account` `<field>Set`, `update_instrument` `replace`, and the
directory updates' omit-versus-value rules). Collapsing those into
"omit keeps, null clears" is a breaking contract. This pass does not redefine
them.
