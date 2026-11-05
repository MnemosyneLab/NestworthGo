# MCP update conventions (audit)

Write tools that change existing state do not share one rule for what an omitted
field means. This is the audited inventory. `TestWriteToolUpdateConventionsAreClassifiedAndMatchSchemas`
(`internal/mcpserver/update_conventions_test.go`) classifies every `update_*`,
`set_*` and versioned replacement preview and checks each against the schema
shape its convention requires, so a new tool cannot silently introduce another
convention: it must be added to the table and to this document.

| Convention | Tools | Omitted field | How to clear |
| --- | --- | --- | --- |
| `patch_at_least_one` | `update_member`, `update_institution`, `update_group` | kept; at least one of `name`, `iconKey`, `archived` is required. Supplied name and icon apply in one write. `archived: true` archives and `archived: false` restores | omit `archived` to keep archive state. JSON null is rejected because it decodes the same as omission. A name and an icon are never empty |
| `patch_set_flags` | `update_account` | kept | send `<field>Set:true` and omit the value for institutionId, groupId, note, iconKey, openedOn, closedOn. JSON null decodes to the same unset pointer as omission, so the schema rejects it; the Set flag is what clears. An empty value **without** `Set` keeps the stored value. Ownership is a complete replacement when supplied |
| `patch_replace_flag` | `update_instrument` | kept when `replace=false`; with `replace=true` the call replaces the whole form state | omitted pointer fields are kept. JSON null is rejected because it decodes the same as omission, except `shortName`, which still accepts null. An empty string clears where the field allows it |
| `patch_versioned_batch` | `update_instruments_batch` | kept per row; every row needs `expectedMetadataVersion`; `changes` needs at least one field | empty `shortName` clears; `note:null` clears |
| `revisioned_replacement` | `update_investment_review_rule` | not allowed: complete rule with `expectedRevision` | not applicable |
| `versioned_full_replacement` | `preview_save_liquidity_policy`, `preview_product_terms` | not allowed: every policy and terms field is required, with `expectedRevision`. Omission never keeps a stored value | `null` means unknown/cleared where the schema allows it |
| `selection` | `set_fx_source`, `set_investment_review_suppression` | state selectors, not field updates | their own clear actions |

## Findings from the audit

- `update_account` previously said "empty string clears optional fields". The
  application keeps the stored value unless the matching `Set` flag is true, so an
  agent following the old text would silently not clear a note. The tool
  description and the skill now state the `Set` rule, and
  `TestUpdateAccountClearRequiresSetFlag` exercises set, omit, empty-without-Set and
  empty-with-Set against a real account.
- `preview_product_terms` is a full replacement with one audited exception:
  `terms.shortName` is optional (omitted or null keeps, empty string clears). The
  exception is listed in the test and stated in the tool description and skill.
- Three styles remain for "keep": omit (patch tools), a `replace` switch
  (`update_instrument`) and explicit per-field `Set` flags (`update_account`).
  Standardizing them (for example omitted = keep and `null` = clear everywhere) is a
  breaking schema change and is deliberately not part of this audit; it needs its
  own decision and a contract-snapshot review.
