package sqlite

import (
	"context"
	"database/sql"
	"strings"
)

func installAgentClosureSchema(ctx context.Context, tx *sql.Tx) error {
	raw, err := schemaFS.ReadFile("agent_closures_schema_v22.sql")
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, string(raw)); err != nil {
		return err
	}
	for name, statement := range financialTriggerStatements() {
		if strings.HasPrefix(name, "financial_source_agent_instrument_closures_") {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	return nil
}
func verifyAgentClosureSchema(ctx context.Context, q schemaQuery) error {
	raw, err := schemaFS.ReadFile("agent_closures_schema_v22.sql")
	if err != nil {
		return err
	}
	for _, part := range strings.Split(string(raw), "CREATE ")[1:] {
		statement := "CREATE " + strings.SplitN(part, ";", 2)[0]
		name := strings.Split(strings.Fields(part)[1], "(")[0]
		if err := verifyFinancialSourceDefinition(ctx, q, name, statement); err != nil {
			return err
		}
	}
	return nil
}
