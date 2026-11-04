package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Restrict authoritative current-input queries, including their preflights,
// before row decoding. Current quote grouping and all-history invalidators are
// deliberately left intact. FX and household metadata remain shared inputs.
type accountReadScope struct {
	queryer
	ids []string
}

func (q accountReadScope) scope(statement string, args []any) (string, []any) {
	marks, values := sqlInArgs(q.ids)
	if len(q.ids) == 0 {
		marks = "NULL"
	}
	instrumentScope := " IN (SELECT instrument_id FROM holdings WHERE account_id IN (" + marks + "))"
	switch {
	case strings.Contains(statement, "a.household_id = ?"):
		statement = strings.Replace(statement, "a.household_id = ?", "a.household_id = ? AND a.id IN ("+marks+")", 1)
	case strings.Contains(statement, "i.household_id = ?"):
		statement = strings.Replace(statement, "i.household_id = ?", "i.household_id = ? AND i.id"+instrumentScope, 1)
	case strings.Contains(statement, "i.household_id=?"):
		statement = strings.Replace(statement, "i.household_id=?", "i.household_id=? AND i.id"+instrumentScope, 1)
	case strings.Contains(statement, "FROM agent_instrument_closures") && strings.Contains(statement, "FROM instruments WHERE household_id=?"):
		statement = strings.Replace(statement, "FROM instruments WHERE household_id=?", "FROM instruments WHERE household_id=? AND id"+instrumentScope, 1)
	default:
		return statement, args
	}
	return statement, append(args, values...)
}
func (q accountReadScope) QueryRowContext(ctx context.Context, statement string, args ...any) *sql.Row {
	statement, args = q.scope(statement, args)
	return q.queryer.QueryRowContext(ctx, statement, args...)
}
func (q accountReadScope) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	statement, args = q.scope(statement, args)
	// Current quote selectors already preflight the complete selected projection.
	if strings.Contains(statement, "WITH candidates") || strings.Contains(statement, "FROM agent_instrument_closures") {
		return q.queryer.QueryContext(ctx, statement, args...)
	}
	size := "1024"
	if strings.Contains(statement, "FROM instruments i") {
		size = "1024+length(CAST(name AS BLOB))+COALESCE(length(CAST(note AS BLOB)),0)"
	}
	if strings.Contains(statement, "FROM holdings h") {
		size = "1024+COALESCE(length(CAST(note AS BLOB)),0)"
	}
	return checkedReadQuery(ctx, q.queryer, statement, size, args...)
}
func scopedAccountInputs(query queryer, accounts []domain.AccountRecord) queryer {
	ids := make([]string, len(accounts))
	for i, a := range accounts {
		ids[i] = a.Account.ID.String()
	}
	return accountReadScope{queryer: query, ids: ids}
}
