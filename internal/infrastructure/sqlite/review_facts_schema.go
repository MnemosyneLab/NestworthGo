package sqlite

import (
	"context"
	"database/sql"
	"strings"
)

func installReviewFactSchema(ctx context.Context, tx *sql.Tx) error {
	raw, err := schemaFS.ReadFile("review_facts_schema_v23.sql")
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, string(raw)); err != nil {
		return err
	}
	return nil
}
func verifyReviewFactSchema(ctx context.Context, q schemaQuery) error {
	raw, err := schemaFS.ReadFile("review_facts_schema_v23.sql")
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
