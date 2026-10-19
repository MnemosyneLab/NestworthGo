package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// This is a compiled schema contract, not a ledger-data cache. A matching
// schema cookie alone cannot authenticate arbitrary triggers present at Open.
var certifiedTriggerDefinitions = sync.OnceValues(func() (map[string]string, error) {
	definitions := make(map[string]string, len(financialTriggerStatements())+16)
	for name, sql := range financialTriggerStatements() {
		definitions[name] = sql
	}
	raw, err := schemaFS.ReadFile("reconciliation_schema.sql")
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(string(raw), "CREATE TRIGGER ")[1:] {
		end := strings.Index(part, "\nEND;")
		if end < 0 {
			return nil, fmt.Errorf("invalid certified trigger schema artifact")
		}
		statement := "CREATE TRIGGER " + part[:end] + "\nEND"
		fields := strings.Fields(part)
		if len(fields) == 0 {
			return nil, fmt.Errorf("missing certified trigger schema name")
		}
		definitions[fields[0]] = statement
	}
	for name, sql := range definitions {
		definitions[name] = canonicalCertifiedTrigger(sql)
	}
	return definitions, nil
})

func canonicalCertifiedTrigger(sql string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.Join(strings.Fields(strings.TrimSuffix(strings.TrimSpace(sql), ";")), " "), " ,", ","), " )", ")")
}

// Reject every unknown permanent trigger, including ones on otherwise
// unrelated tables that can write protected rows. Also verify all known bodies;
// an expected name is not evidence of trusted writer behavior.
func verifyCertifiedTriggers(ctx context.Context, q schemaQuery) (bool, error) {
	expected, err := certifiedTriggerDefinitions()
	if err != nil {
		return false, err
	}
	rows, err := q.QueryContext(ctx, `SELECT name,sql FROM sqlite_master WHERE type='trigger' ORDER BY name`)
	if err != nil {
		return false, err
	}
	seen := 0
	valid := true
	for rows.Next() {
		var name, sql string
		if err = rows.Scan(&name, &sql); err != nil {
			break
		}
		want, ok := expected[name]
		if !ok || canonicalCertifiedTrigger(sql) != want {
			valid = false
		}
		seen++
	}
	if err = errors.Join(err, rows.Err(), rows.Close()); err != nil {
		return false, err
	}
	return valid && seen == len(expected), nil
}
