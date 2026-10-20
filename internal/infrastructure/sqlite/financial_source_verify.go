package sqlite

import (
	"context"
	"fmt"
	"strings"
)

func verifyFinancialSourceTracking(ctx context.Context, q schemaQuery) error {
	return verifyFinancialSourceTrackingState(ctx, q, false)
}

// verifiedTriggers may be true only after the exact complete certified writer
// contract was checked in this same SQLite transaction/snapshot. It avoids
// rereading every financial trigger body; no result survives the transaction.
func verifyFinancialSourceTrackingState(ctx context.Context, q schemaQuery, verifiedTriggers bool) error {
	var recursive int
	if err := q.QueryRowContext(ctx, `PRAGMA recursive_triggers`).Scan(&recursive); err != nil {
		return err
	}
	if recursive != 1 {
		return fmt.Errorf("financial source recursive triggers disabled")
	}
	raw, err := schemaFS.ReadFile("source_changes_schema_v20.sql")
	if err != nil {
		return err
	}
	for _, part := range strings.Split(string(raw), "CREATE TABLE ")[1:] {
		statement := "CREATE TABLE " + strings.SplitN(part, ";", 2)[0]
		name := strings.Fields(part)[0]
		if err := verifyFinancialSourceDefinition(ctx, q, name, statement); err != nil {
			return err
		}
	}
	if !verifiedTriggers {
		expected := map[string]string{}
		var schemaVersion int
		if err := q.QueryRowContext(ctx, "PRAGMA user_version").Scan(&schemaVersion); err != nil {
			return err
		}
		for name, statement := range financialTriggerStatements() {
			if schemaVersion < 22 && strings.HasPrefix(name, "financial_source_agent_instrument_closures_") {
				continue
			}
			expected[name] = statement
		}
		for name, statement := range expected {
			if err := verifyFinancialSourceDefinition(ctx, q, name, statement); err != nil {
				return err
			}
		}
		rows, err := q.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='trigger' AND substr(name,1,length('financial_source_'))='financial_source_'`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var n string
			if err = rows.Scan(&n); err != nil {
				rows.Close()
				return err
			}
			if _, ok := expected[n]; !ok {
				rows.Close()
				return fmt.Errorf("unknown financial source trigger %s", n)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}

	classified := map[string]bool{}
	for _, n := range financialSourceTables {
		classified[n] = true
	}
	for _, n := range financialSourceExcluded {
		classified[n] = true
	}
	// Unrelated extension tables were historically allowed and are not read
	// by the financial closure. Every application-owned schema addition must
	// instead be classified explicitly in this build's manifest.
	for n := range financialColumnManifest {
		if !classified[n] {
			return fmt.Errorf("unclassified application source table %s", n)
		}
	}
	var invalid int
	err = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM households h LEFT JOIN financial_source_revision r ON r.household_id=h.id WHERE r.household_id IS NULL OR r.tracking_version<>? OR length(r.tracking_epoch)<>32 OR r.tracking_epoch GLOB '*[^0-9a-f]*' OR typeof(r.revision)<>'integer' OR r.journal_floor<0 OR r.journal_floor>r.revision OR (SELECT COUNT(*) FROM (SELECT 1 FROM financial_source_changes c WHERE c.household_id=h.id LIMIT ?))<>r.revision-r.journal_floor OR COALESCE((SELECT MIN(c.revision) FROM financial_source_changes c WHERE c.household_id=h.id),r.revision+1)<>r.journal_floor+1 OR COALESCE((SELECT MAX(c.revision) FROM financial_source_changes c WHERE c.household_id=h.id),r.revision)<>r.revision`, financialTrackingVersion, financialJournalMaxRetained+1).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid != 0 {
		return fmt.Errorf("incomplete financial source revision/journal")
	}
	if err = q.QueryRowContext(ctx, `SELECT count(*) FROM financial_source_revision WHERE revision-journal_floor>?`, financialJournalMaxRetained).Scan(&invalid); err != nil {
		return err
	}
	if invalid != 0 {
		return fmt.Errorf("financial source journal exceeds retention bound")
	}
	return nil
}
func verifyFinancialSourceDefinition(ctx context.Context, q schemaQuery, name, want string) error {
	var got string
	if err := q.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE name=?`, name).Scan(&got); err != nil {
		return fmt.Errorf("missing financial source schema %s: %w", name, err)
	}
	canon := func(s string) string { return strings.TrimSuffix(strings.Join(strings.Fields(s), " "), ";") }
	if canon(got) != canon(want) {
		return fmt.Errorf("invalid financial source schema %s", name)
	}
	return nil
}
