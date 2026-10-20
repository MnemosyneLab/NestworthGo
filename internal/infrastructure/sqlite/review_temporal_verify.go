package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func verifyReviewTemporalSchema(ctx context.Context, q schemaQuery) error {
	raw, err := schemaFS.ReadFile("review_temporal_schema_v21.sql")
	if err != nil {
		return err
	}
	definition := strings.SplitN(string(raw), ";", 2)[0]
	if err := verifyFinancialSourceDefinition(ctx, q, "review_temporal_checkpoints", definition); err != nil {
		return err
	}
	rows, err := q.QueryContext(ctx, `SELECT household_id,evaluated_at,next_transition_at,certificate_json FROM review_temporal_checkpoints`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, evaluated, next, raw string
		if err := rows.Scan(&id, &evaluated, &next, &raw); err != nil {
			return err
		}
		var c domain.ReviewTemporalCertificate
		if json.Unmarshal([]byte(raw), &c) != nil || c.EvaluatedAt.IsZero() || !c.NextTransitionAt.After(c.EvaluatedAt) || c.ClockEpoch == "" || c.PolicyVersion == "" || c.DependencyScope == "" || evaluated != c.EvaluatedAt.UTC().Format(time.RFC3339Nano) || next != c.NextTransitionAt.UTC().Format(time.RFC3339Nano) || c.Source != nil && (!c.Source.Valid() || c.Source.Token.HouseholdID.String() != id) {
			return fmt.Errorf("invalid persisted review temporal checkpoint")
		}
	}
	return rows.Err()
}

func installReviewTemporalSchema(ctx context.Context, tx *sql.Tx) error {
	raw, err := schemaFS.ReadFile("review_temporal_schema_v21.sql")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, string(raw))
	return err
}
