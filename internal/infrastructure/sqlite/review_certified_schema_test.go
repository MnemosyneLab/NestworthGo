package sqlite

import (
	"strings"
	"testing"
)

func TestReviewCertifiedSchemaRequiresCompleteCompiledContract(t *testing.T) {
	for _, change := range []string{"none", "index", "table", "view", "sqlite-prefix", "predicate", "table-fk"} {
		t.Run(change, func(t *testing.T) {
			db, err := Open(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			statements := map[string][]string{
				"index":         {`CREATE INDEX synthetic_extra ON review_temporal_checkpoints(evaluated_at)`},
				"table":         {`CREATE TABLE synthetic_extra(id TEXT PRIMARY KEY)`},
				"view":          {`CREATE VIEW synthetic_extra AS SELECT * FROM review_temporal_checkpoints`},
				"sqlite-prefix": {`CREATE TABLE sqliteXextra(id TEXT PRIMARY KEY)`},
				"predicate":     {`DROP INDEX ux_investment_review_open_episode`, `CREATE UNIQUE INDEX ux_investment_review_open_episode ON investment_review_episodes(rule_id,instrument_id,position_epoch_id) WHERE resolved_at IS NOT NULL`},
			}
			if change == "table-fk" {
				var definition string
				if err := db.SQL.QueryRow(`SELECT sql FROM sqlite_master WHERE name='review_temporal_checkpoints'`).Scan(&definition); err != nil {
					t.Fatal(err)
				}
				end := strings.LastIndex(definition, ")")
				definition = definition[:end] + ", FOREIGN KEY(evaluated_at) REFERENCES households(id) ON UPDATE CASCADE DEFERRABLE INITIALLY DEFERRED" + definition[end:]
				statements[change] = []string{`DROP TABLE review_temporal_checkpoints`, definition}
			}
			for _, statement := range statements[change] {
				if _, err := db.SQL.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			ok, err := verifyCertifiedSchema(t.Context(), db.SQL)
			if err != nil || ok != (change == "none") {
				t.Fatal("incorrect compiled schema verdict", ok, err)
			}
		})
	}
}
