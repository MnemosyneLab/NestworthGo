# Optional investment short names

SQLite schemas 21 and 22 retain the short-name field. Only those database
versions are supported; see [storage recovery](../architecture/data-and-ipc-contracts.md#database-support-and-storage-recovery).

## Display and identity contract

Every primary instrument label uses the same order:

1. Trimmed, nonempty `shortName`.
2. Trimmed symbol/code.
3. Trimmed full name.

Go and React share this rule, including Unicode whitespace, NEL and BOM. There
are no market or instrument-type exceptions. Full-name subtitles retain the
existing `InstrumentLabel` layout and classes. Selectors append identity text
(full name, symbol, market, currency and ISIN) and continue to use IDs as values.
Duplicate aliases are allowed, including across share classes and archived
instruments. Sorting and labels do not change stable row/navigation keys.

The optional field permits at most 120 Unicode code points after trimming.
An explicit empty or whitespace-only string clears it. An omitted field keeps
an existing alias, including an older complete-form request that predates this
field. New records default to an empty alias. The field does not change symbols,
provider bindings, ISINs, IDs, quantities, exact money values or transaction
identity. Existing provider bindings are retained when a form edits metadata
on an instrument using Manual or Agent quotes.

The alias is propagated through Wails/MCP requests, instrument DTOs, valuation
and missing-input components, grouped holdings, overview/activity labels,
change preview endpoints, historical hierarchy rows, gain summaries, data
health and sync progress. Current queries and analysis caches are invalidated
after metadata edits. Frozen historical records and financial facts are not
rewritten to substitute an alias for an identity.

Managed products have one persistent alias: `instruments.short_name` on the
contract's linked internal instrument. The product projection reads it from
that instrument. Contract-term edits update the linked instrument's full name
and alias in the same revision-checked transaction, including the guarded MCP
terms path. General instrument editing continues to reject managed instruments.
The optional alias in sealed contract JSON omits empty values, preserving older
receipts' encoding. Product details retain the symbol and legal name.

## JSON export compatibility

`com.nestworth.export` format version 2 is a deliberately versioned financial
projection, not the restorable application database. It does **not** export
short names. This change leaves that contract unchanged. Alias-preserving
backup/restore uses the SQLite backup package or the cloud database snapshot.
