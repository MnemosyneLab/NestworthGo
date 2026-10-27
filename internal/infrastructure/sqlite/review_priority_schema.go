package sqlite

import (
	"context"
	"database/sql"
	"strings"
)

// Legacy rows need no rewrite: the read adapter maps warning to P1, notice to P2.
func installReviewPrioritySchema(ctx context.Context, tx *sql.Tx) error {
	raw, err := schemaFS.ReadFile("review_priority_schema_v23.sql")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, string(raw))
	return err
}

func verifyReviewPrioritySchema(ctx context.Context, q schemaQuery) error {
	raw, err := schemaFS.ReadFile("review_priority_schema_v23.sql")
	if err != nil {
		return err
	}
	statement := strings.SplitN(string(raw), ";", 2)[0]
	return verifyFinancialSourceDefinition(ctx, q, "investment_review_rule_priorities", statement)
}

const reviewPrioritySQL = `COALESCE((SELECT priority FROM investment_review_rule_priorities p WHERE p.rule_id=r.id),CASE r.severity WHEN 'warning' THEN 'p1' ELSE 'p2' END)`
