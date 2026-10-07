package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// These are admission budgets, not truncation limits. Check each query inside
// the read transaction before allocating rows or decoding persisted JSON.
const contextInputRows = 100000
const contextInputBytes = 32 << 20

type contextBudgetQuery struct {
	tx          *sql.Tx
	rows, bytes int
	err         error
}

func (q *contextBudgetQuery) check(ctx context.Context, statement string, args ...any) error {
	if q.err != nil {
		return q.err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	probe, err := q.tx.QueryContext(ctx, "SELECT * FROM ("+statement+") LIMIT 0", args...)
	if err != nil {
		return err
	}
	columns, err := probe.Columns()
	probe.Close()
	if err != nil {
		return err
	}
	lengths := make([]string, len(columns))
	for i, col := range columns {
		lengths[i] = `COALESCE(length(CAST("` + strings.ReplaceAll(col, `"`, `""`) + `" AS BLOB)),0)`
	}
	bounded := fmt.Sprintf("SELECT * FROM (%s) LIMIT %d", statement, q.rows+1)
	var count, size int
	err = q.tx.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM("+strings.Join(lengths, "+")+"),0) FROM ("+bounded+")", args...).Scan(&count, &size)
	if err != nil {
		return err
	}
	// Include per-row decoding/object overhead in the byte admission budget.
	size += count * 256
	if count > q.rows || size > q.bytes {
		q.err = &domain.Error{Code: domain.ErrorCode("too_large"), Message: "financial context inputs exceed the bounded read budget"}
		return q.err
	}
	q.rows -= count
	q.bytes -= size
	return nil
}
func (q *contextBudgetQuery) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	if err := q.check(ctx, statement, args...); err != nil {
		return nil, err
	}
	return q.tx.QueryContext(ctx, statement, args...)
}
func (q *contextBudgetQuery) QueryRowContext(ctx context.Context, statement string, args ...any) *sql.Row {
	if err := q.check(ctx, statement, args...); err != nil {
		q.err = err
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		return q.tx.QueryRowContext(cancelled, statement, args...)
	}
	return q.tx.QueryRowContext(ctx, statement, args...)
}

func (r *Repository) ReadFinancialContextInputs(ctx context.Context, historical bool, ids []domain.AccountID, now time.Time) (domain.FinancialContextInputs, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.FinancialContextInputs{}, err
	}
	defer tx.Rollback()
	q := &contextBudgetQuery{tx: tx, rows: contextInputRows, bytes: contextInputBytes}
	portfolio, err := readPortfolioSnapshotQuery(ctx, q, domain.AccountFilter{IncludeArchived: true})
	if q.err != nil {
		return domain.FinancialContextInputs{}, q.err
	}
	if err != nil {
		return domain.FinancialContextInputs{}, err
	}
	if portfolio.Household == nil {
		return domain.FinancialContextInputs{}, &domain.Error{Code: domain.ErrorCode("household_required"), Message: "household is required"}
	}
	for _, id := range ids {
		found := false
		for _, a := range portfolio.Accounts {
			if a.Account.ID == id {
				found = true
				break
			}
		}
		if !found {
			return domain.FinancialContextInputs{}, &domain.Error{Code: domain.ErrNotFound, Message: "selected account was not found"}
		}
	}
	result := domain.FinancialContextInputs{Portfolio: portfolio}
	if historical {
		if portfolio.Origin == nil {
			return result, &domain.Error{Code: domain.ErrHistoryNotStarted, Message: "history has not started"}
		}
		// Full dependencies are deliberately retained, including transfer endpoints.
		batch, loadErr := loadHistoricalSnapshotBatchQuery(ctx, q, portfolio.Household.ID, now)
		if q.err != nil {
			return result, q.err
		}
		if loadErr != nil {
			return result, loadErr
		}
		result.History = &batch
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}
