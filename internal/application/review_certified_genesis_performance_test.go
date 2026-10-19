//go:build perf

package application

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Only unpublished synthetic facts/configuration are copied into a newly
// initialized database. Reopening a VACUUM copy cannot carry genesis trust.
// Install metrics at creation so instrumentation never replaces that connection.
func cloneReviewGenesisPerfFixture(t *testing.T, source reviewFullPipelineFixture) reviewFullPipelineFixture {
	t.Helper()
	var published int
	if err := source.db.SQL.QueryRow(`SELECT (SELECT count(*) FROM review_temporal_checkpoints)+(SELECT count(*) FROM investment_review_episodes)+(SELECT count(*) FROM investment_review_episode_events)+(SELECT count(*) FROM investment_review_legacy_state)+(SELECT count(*) FROM investment_review_legacy_quarantine)+(SELECT count(*) FROM investment_review_mutation_receipts WHERE json_type(result_json,'$.episodeId') IS NOT NULL)`).Scan(&published); err != nil || published != 0 {
		t.Fatal("genesis fixture source contains review history", published, err)
	}
	path := filepath.Join(t.TempDir(), "synthetic-genesis.db")
	db, err := sqlite.OpenReviewPerformanceDatabase(path, reviewPerfDriverName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.SQL.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.SQL.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	rows, err := source.db.SQL.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	skip := map[string]bool{"review_temporal_checkpoints": true, "investment_review_daily_inputs": true, "investment_review_daily_results": true, "investment_review_position_epochs": true, "investment_review_evaluation_state": true, "investment_review_episodes": true, "investment_review_episode_events": true}
	copyTable := func(name string) {
		quoted := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
		data, err := source.db.SQL.Query(`SELECT * FROM ` + quoted)
		if err != nil {
			t.Fatal(err)
		}
		defer data.Close()
		cols, err := data.Columns()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(t.Context(), `DELETE FROM `+quoted); err != nil {
			t.Fatal(err)
		}
		args := make([]any, len(cols))
		into := make([]any, len(cols))
		marks := make([]string, len(cols))
		for i := range args {
			into[i] = &args[i]
			marks[i] = "?"
		}
		insert, err := tx.PrepareContext(t.Context(), `INSERT INTO `+quoted+` VALUES(`+strings.Join(marks, ",")+`)`)
		if err != nil {
			t.Fatal(err)
		}
		defer insert.Close()
		for data.Next() {
			if err := data.Scan(into...); err != nil {
				t.Fatal(err)
			}
			if _, err := insert.ExecContext(t.Context(), args...); err != nil {
				t.Fatal(fmt.Errorf("seed %s: %w", name, err))
			}
		}
		if err := data.Err(); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range tables {
		if !skip[table] && !strings.HasPrefix(table, "financial_source_") {
			copyTable(table)
		}
	}
	// Known derived state markers may have been invalidated by seed triggers.
	// Restore the source fixture's markers after all authoritative rows exist.
	copyTable("history_snapshot_state")
	copyTable("investment_review_control")
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	check, err := db.SQL.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if check.Next() {
		check.Close()
		t.Fatal("synthetic genesis has invalid foreign keys")
	}
	if err := check.Err(); err != nil {
		t.Fatal(err)
	}
	check.Close()
	now := *source.now
	return reviewFullPipelineFixture{db: db, dbPath: path, now: &now, householdID: source.householdID, start: source.start, instruments: source.instruments}
}
