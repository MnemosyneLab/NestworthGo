// Package testutil contains isolated synthetic fixture helpers.
package testutil

import (
	"database/sql"
	"testing"
)

// Schema21 removes only schema22's additive closure facts from a synthetic
// current database. Never call this on an existing user database.
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
