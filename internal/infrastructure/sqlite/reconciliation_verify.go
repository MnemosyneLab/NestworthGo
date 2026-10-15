package sqlite

import (
	"context"
	"fmt"
	"strings"
)

func verifyReconciliationSchema(ctx context.Context, query schemaQuery) error {
	raw, err := schemaFS.ReadFile("reconciliation_schema.sql")
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
			return strings.Join(strings.Fields(strings.TrimSuffix(strings.TrimSpace(s), ";")), " ")
		}
		if normalize(stored) != normalize("CREATE "+statement) {
			return fmt.Errorf("invalid reconciliation schema %s", name)
		}
	}
	return nil
}
