// Package testutil contains isolated synthetic fixture helpers.
package testutil

import (
	"database/sql"
	"testing"
)

// Schema21 removes only schema22's additive closure facts from a synthetic
// current database. Never call this on an existing user database.
// Provenance: cb93bed4c24d5d09789e7d63e6288db23b3c826d was schema21.
// Its base/reconciliation/review/source/temporal DDL is unchanged in schema22;
// 8a6bdbc adds only the closure table/index and its three tracking triggers.
// Removing those additions is the exact structural inverse, without old upgrades.
func Schema21(t testing.TB, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`DROP TRIGGER financial_source_agent_instrument_closures_insert;
 DROP TRIGGER financial_source_agent_instrument_closures_update;
 DROP TRIGGER financial_source_agent_instrument_closures_delete;
 DROP TABLE agent_instrument_closures;
 PRAGMA user_version=21;`); err != nil {
		t.Fatal(err)
	}
}
