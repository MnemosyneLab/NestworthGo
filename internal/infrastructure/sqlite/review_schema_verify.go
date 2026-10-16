package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func verifyReviewSchema(ctx context.Context, query schemaQuery) error {
	raw, err := schemaFS.ReadFile("review_schema_v19.sql")
	if err != nil {
		return err
	}
	for _, part := range strings.Split(string(raw), "CREATE ")[1:] {
		end := strings.IndexByte(part, ';')
		if end < 0 {
			return fmt.Errorf("invalid investment review schema artifact")
		}
		statement := "CREATE " + part[:end]
		fields := strings.Fields(statement)
		if len(fields) < 3 {
			return fmt.Errorf("invalid investment review schema artifact")
		}
		nameAt := 2
		if fields[1] == "UNIQUE" {
			nameAt = 3
		}
		if len(fields) <= nameAt {
			return fmt.Errorf("invalid investment review schema artifact")
		}
		name := strings.Split(fields[nameAt], "(")[0]
		var actual string
		if err := query.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE name=?`, name).Scan(&actual); err != nil {
			return fmt.Errorf("missing investment review schema %s: %w", name, err)
		}
		canonical := func(value string) string {
			return strings.ReplaceAll(strings.ReplaceAll(strings.Join(strings.Fields(value), " "), " ,", ","), " )", ")")
		}
		if canonical(actual) != canonical(statement) {
			return fmt.Errorf("invalid investment review schema %s", name)
		}
	}
	rows, err := query.QueryContext(ctx, `SELECT raw_json,raw_sha256 FROM investment_review_legacy_state`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw, hash string
		if err := rows.Scan(&raw, &hash); err != nil {
			return err
		}
		digest := sha256.Sum256([]byte(raw))
		if hash != hex.EncodeToString(digest[:]) {
			return fmt.Errorf("investment review legacy source hash changed")
		}
	}
	return rows.Err()
}
