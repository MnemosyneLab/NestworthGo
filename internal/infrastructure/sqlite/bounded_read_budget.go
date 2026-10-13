package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Preflight counts and UTF-8 payload bytes in SQL before expanding rows. The
// table and predicate are internal constants, never client SQL.
func checkReadTableBudget(ctx context.Context, q queryer, table, predicate string, args ...any) error {
	var count, size int
	expression := "256"
	switch strings.Fields(table)[0] {
	case "instrument_quotes":
		expression = "512+length(CAST(source_key AS BLOB))+COALESCE(length(CAST(conversion_json AS BLOB)),0)"
	case "fx_quotes":
		expression = "512+length(CAST(source_key AS BLOB))"
	case "agent_quote_records":
		expression = "512+length(CAST(source_title AS BLOB))+length(CAST(source_url AS BLOB))"
	case "instruments":
		expression = "256+length(CAST(name AS BLOB))+length(CAST(short_name AS BLOB))+COALESCE(length(CAST(note AS BLOB)),0)"
	case "liquidity_policies":
		expression = "512+COALESCE(length(CAST(note AS BLOB)),0)"
	case "liquidity_reservations":
		expression = "512+length(CAST(label AS BLOB))"
	case "product_contracts":
		expression = "1024+length(CAST(name AS BLOB))+COALESCE(length(CAST(note AS BLOB)),0)"
	}
	statement := fmt.Sprintf("SELECT COUNT(*),COALESCE(SUM(%s),0) FROM %s WHERE %s", expression, table, predicate)
	if err := q.QueryRowContext(ctx, statement, args...).Scan(&count, &size); err != nil {
		return err
	}
	if count > 20000 || size > 4<<20 {
		return &domain.Error{Code: "too_large", Message: "capture exceeds SQL read budget; narrow targets or filters"}
	}
	return domain.ReserveReadBudget(ctx, fmt.Sprint(table, "/", predicate, "/", args), count, size)
}

func checkedReadQuery(ctx context.Context, q queryer, statement, bytesExpression string, args ...any) (*sql.Rows, error) {
	if domain.IsBoundedRead(ctx) {
		var rows, size int
		if err := q.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM("+bytesExpression+"),0) FROM ("+statement+")", args...).Scan(&rows, &size); err != nil {
			return nil, err
		}
		if err := domain.ReserveReadBudget(ctx, fmt.Sprint(statement, args), rows, size); err != nil {
			return nil, err
		}
	}
	return q.QueryContext(ctx, statement, args...)
}
