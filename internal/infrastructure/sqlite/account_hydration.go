package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Leave room for the household parameter below SQLite's legacy 999-variable
// limit. Each chunk is fully consumed and closed before opening the next one.
const accountHydrationChunkSize = 500

// An omitted scope retains the household query as an internal baseline oracle.
// An explicitly empty scope performs no reads. Callers pass IDs from the account
// selection, and every hydration query independently retains the household fence.
func loadAccountHydration[T any](ctx context.Context, query queryer, householdID domain.HouseholdID, scope [][]domain.AccountID, statement string, scan func(*sql.Rows, error) (map[domain.AccountID]T, error)) (map[domain.AccountID]T, error) {
	if len(scope) == 0 {
		return scan(query.QueryContext(ctx, statement, householdID.String()))
	}
	result := make(map[domain.AccountID]T)
	ids := scope[0]
	for start := 0; start < len(ids); start += accountHydrationChunkSize {
		end := min(start+accountHydrationChunkSize, len(ids))
		args := make([]any, 1, end-start+1)
		args[0] = householdID.String()
		for _, id := range ids[start:end] {
			args = append(args, id.String())
		}
		// Insert before the ownership ORDER BY; latest-value ordering stays inside
		// its indexed correlated lookup, unchanged.
		predicate := " AND a.id IN (" + strings.TrimSuffix(strings.Repeat("?,", end-start), ",") + ")"
		selected := strings.Replace(statement, " WHERE a.household_id = ?", " WHERE a.household_id = ?"+predicate, 1)
		chunk, err := scan(query.QueryContext(ctx, selected, args...))
		if err != nil {
			return nil, err
		}
		for id, value := range chunk {
			result[id] = value
		}
	}
	return result, nil
}
