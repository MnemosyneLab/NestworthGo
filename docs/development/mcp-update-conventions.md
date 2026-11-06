# MCP update conventions

Write tools that change existing state follow one of three rules. Patch tools
share one: omit a field to keep the stored value, and JSON null clears a field
that is allowed to be empty. An empty string is rejected on those tools. It
does not clear, and it does not keep. Full replacements still require every
field. Selectors are not field patches.

`TestWriteToolUpdateConventionsAreClassifiedAndMatchSchemas`
(`internal/mcpserver/update_conventions_test.go`) classifies every `update_*`,
`set_*` and versioned replacement preview and checks the schema shape, so a
new tool cannot reintroduce a `<field>Set` flag or a `replace` switch.

| Convention | Tools | Omitted field | How to clear |
| --- | --- | --- | --- |
| `patch` | `update_member`, `update_institution`, `update_group`, `update_account`, `update_instrument`, `update_instruments_batch` | kept | JSON null on a clearable field. Null and an empty string are rejected when the field cannot be cleared |
| `revisioned_replacement` | `update_investment_review_rule` | not allowed: the complete rule and `expectedRevision` are required | not applicable |
| `versioned_full_replacement` | `preview_save_liquidity_policy`, `preview_product_terms` | not allowed: every policy and terms field is required, with `expectedRevision`. Omission never keeps a stored value | `null` means unknown or cleared where the schema allows it |
| `selection` | `set_fx_source`, `set_investment_review_suppression` | state selectors, not field updates | their own clear actions |

## Patch fields

Directory name, icon and `archived` cannot be cleared. At least one of those
fields is required. `archived: true` archives and `archived: false` restores.
A household keeps at least one active member, and the only active owner of a
live account cannot be archived. A rejected archive does not apply the other
fields in that call. Name, icon, archive state and the update timestamp commit
atomically; a SQL failure or cancellation before commit keeps every stored field.

`update_account` clears `institutionId`, `groupId`, `note`, `iconKey`,
`openedOn` and `closedOn` with null. A note has no distinct empty stored
value, so null is the only clear; an empty string is invalid. Null and an
empty string are rejected for name, account type, balance-sheet role,
tracking mode, currency, inclusion flags, ownership and `initialAmount`.
Those classification and currency fields stay immutable when a different
value is supplied. Ownership, when supplied, replaces the whole allocation
and cannot be empty. `initialAmount` does not rewrite balance history.
Removing `<field>Set` is a breaking change: `{"note":"","noteSet":true}` is
rejected. Send `{"note":null}`.

`update_instrument` clears `shortName`, `symbol`, `marketCode`,
`countryCode`, `isin`, `note`, `providerKey` and `providerSymbol` with null.
Null on `iconKey` resets it to the instrument type's default icon. Null is
rejected for `name`, `type`, `quoteCurrency`, `metalTemplate`,
`quantityUnit`, `quoteSource` and `sortOrder`. `sortOrder: 0` is a stored
order, not an omission. The `replace` flag is removed. A caller that used
`replace: true` to drop omitted optional fields now sends null for each field
to clear and omits the fields to keep. An empty `shortName` no longer
clears; send null.

`update_instruments_batch` uses the same rule for `name`, `shortName` and
`note`. Null clears `shortName` and `note`. An empty string is rejected,
including a blank note that previously cleared. Null `shortName` now clears;
it used to be rejected. Null and an empty name are rejected. Every row still
needs `expectedMetadataVersion`, and `changes` still needs at least one
field. Managed or archived targets and a stale version reject the whole
batch.

## Full replacements

`preview_product_terms` remains a full replacement with one audited exception:
`terms.shortName` is optional. Omitted or null keeps the stored alias and an
empty string clears it. That exception is not the patch rule. The field is
optional inside an otherwise required terms object, and null there still
means keep. The tool description states it.

Liquidity policy save is a full replacement. Null means unknown or cleared
where that schema allows it. Omission does not keep a stored policy field.
