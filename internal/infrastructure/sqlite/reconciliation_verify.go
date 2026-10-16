package sqlite

import (
	"context"
	"fmt"
	"strings"
)

func verifyReconciliationSchema(ctx context.Context, query schemaQuery, version int) error {
	file := "reconciliation_schema.sql"
	if version == 17 {
		file = "reconciliation_schema_v17.sql"
	}
	raw, err := schemaFS.ReadFile(file)
	if err != nil {
		return err
	}
	// Compare the actual DDL, including constraints, indexes and revision triggers.
	// This additive artifact is shared by new databases and the v16 upgrade.
	for _, part := range strings.Split(string(raw), "CREATE ")[1:] {
		lines := strings.Split(part, "\n")
		var statement string
		for _, line := range lines {
			statement += line + "\n"
			if strings.HasPrefix(part, "TRIGGER ") {
				if strings.TrimSpace(line) == "END;" {
					break
				}
			} else if strings.Contains(line, ";") {
				break
			}
		}
		fields := strings.Fields(statement)
		if len(fields) < 2 {
			return fmt.Errorf("invalid reconciliation DDL")
		}
		name := strings.Split(fields[1], "(")[0]
		var stored string
		if err := query.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE name=?", name).Scan(&stored); err != nil {
			return fmt.Errorf("missing reconciliation schema %s: %w", name, err)
		}
		normalize := func(s string) string {
			return strings.ReplaceAll(strings.ReplaceAll(strings.Join(strings.Fields(strings.TrimSuffix(strings.TrimSpace(s), ";")), " "), " ,", ","), " )", ")")
		}
		if normalize(stored) != normalize("CREATE "+statement) {
			return fmt.Errorf("invalid reconciliation schema %s: stored %s expected %s", name, normalize(stored), normalize("CREATE "+statement))
		}
	}
	var invalid int
	if err := query.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM reconciliation_accounts WHERE json_extract(evidence,'$.accountId') IS NOT account_id OR json_extract(evidence,'$.financialRevision') < 1)
 +(SELECT count(*) FROM reconciliation_items WHERE json_extract(evidence,'$.id') IS NOT id OR json_extract(evidence,'$.accountId') IS NOT account_id OR json_extract(evidence,'$.outcome') NOT IN ('pending','confirmed','skipped','unresolved'))
 +(SELECT count(*) FROM reconciliation_sessions WHERE (status='completed') != (completed_at IS NOT NULL))
 +(SELECT count(*) FROM reconciliation_receipts WHERE json_extract(result,'$.householdId') IS NOT household_id)`).Scan(&invalid); err != nil {
		return err
	}
	if invalid != 0 {
		return fmt.Errorf("reconciliation evidence is inconsistent")
	}
	return nil
}
