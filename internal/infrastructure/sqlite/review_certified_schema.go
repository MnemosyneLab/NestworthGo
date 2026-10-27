package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// The writer contract includes internal FK actions and constraints, not just
// named user triggers. Derive the complete table/index allowlist from compiled
// fresh-schema artifacts, never from the database being authenticated. Exact
// SQL also covers deferred actions, generated columns, CHECKs and predicates.
// Unknown tables/views can introduce incoming cascading dependencies, so they
// conservatively disable authority too. This is immutable schema metadata.
var certifiedSchemaDefinitions = sync.OnceValues(func() (map[string]string, error) {
	expected := map[string]string{}
	for _, file := range []string{"schema.sql", "review_schema.sql", "source_changes_schema.sql", "review_temporal_schema_v21.sql", "agent_closures_schema_v22.sql", "review_priority_schema_v23.sql"} {
		raw, err := schemaFS.ReadFile(file)
		if err != nil {
			return nil, err
		}
		for _, part := range strings.Split(string(raw), "CREATE ")[1:] {
			fields := strings.Fields(part)
			nameAt := 1
			if len(fields) > 0 && fields[0] == "UNIQUE" {
				nameAt = 2
			}
			if len(fields) <= nameAt || (fields[0] != "TABLE" && fields[nameAt-1] != "INDEX") {
				continue
			}
			end := strings.IndexByte(part, ';')
			if end < 0 {
				return nil, fmt.Errorf("invalid certified schema artifact %s", file)
			}
			name := strings.Split(fields[nameAt], "(")[0]
			if _, duplicate := expected[name]; duplicate {
				return nil, fmt.Errorf("duplicate certified schema object %s", name)
			}
			expected[name] = strings.TrimSpace("CREATE " + part[:end])
		}
	}
	return expected, nil
})

func verifyCertifiedSchema(ctx context.Context, q schemaQuery) (bool, error) {
	expected, err := certifiedSchemaDefinitions()
	if err != nil {
		return false, err
	}
	rows, err := q.QueryContext(ctx, `SELECT name,sql FROM sqlite_master WHERE type IN ('table','index','view') AND sql IS NOT NULL ORDER BY name`)
	if err != nil {
		return false, err
	}
	seen, valid := 0, true
	for rows.Next() {
		var name, definition string
		if err = rows.Scan(&name, &definition); err != nil {
			break
		}
		want, ok := expected[name]
		if !ok || strings.TrimSpace(definition) != want {
			valid = false
		}
		seen++
	}
	if err = errors.Join(err, rows.Err(), rows.Close()); err != nil {
		return false, err
	}
	return valid && seen == len(expected), nil
}
