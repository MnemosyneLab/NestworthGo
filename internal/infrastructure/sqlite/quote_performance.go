package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ReadQuotePerformanceEvidence reads latest daily evidence, nearest baselines,
// and intervening local facts in one transaction, independently of chart range.
func (r *Repository) ReadQuotePerformanceEvidence(ctx context.Context, i domain.Instrument, today string) ([]domain.InstrumentQuote, []string, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	where := `q.instrument_id=? AND q.source_kind=? AND COALESCE(q.binding_revision,0)=? AND q.currency=? AND q.observation_kind='close' AND q.effective_date<>'' AND q.effective_date<=? AND NOT EXISTS(SELECT 1 FROM agent_quote_records w WHERE w.target_type='instrument' AND w.target_quote_id=q.id)`
	args := []any{i.ID.String(), string(i.QuoteSource), i.ProviderBindingRevision, i.QuoteCurrency.String(), today}
	var latest sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT MAX(q.effective_date) FROM instrument_quotes q WHERE "+where, args...).Scan(&latest); err != nil {
		return nil, nil, err
	}
	if !latest.Valid {
		return []domain.InstrumentQuote{}, nil, nil
	}
	end, err := time.Parse(time.DateOnly, latest.String)
	if err != nil {
		return nil, nil, err
	}
	earliest := latest.String
	for _, days := range []int{7, 30} {
		var base sql.NullString
		target := end.AddDate(0, 0, -days).Format(time.DateOnly)
		params := append(append([]any{}, args...), target)
		if err := tx.QueryRowContext(ctx, "SELECT MAX(q.effective_date) FROM instrument_quotes q WHERE "+where+" AND q.effective_date<=?", params...).Scan(&base); err != nil {
			return nil, nil, err
		}
		if base.Valid && base.String < earliest {
			earliest = base.String
		}
	}
	where += " AND q.effective_date>=?"
	args = append(args, earliest)
	if err := checkReadTableBudget(ctx, tx, "instrument_quotes q", where, args...); err != nil {
		return nil, nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT "+marketInstrumentQuoteColumns+" FROM instrument_quotes q WHERE "+where+" ORDER BY q.effective_date,q.revision,q.created_at,q.id LIMIT 20001", args...)
	if err != nil {
		return nil, nil, err
	}
	quotes, err := scanInstrumentQuotes(rows)
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if len(quotes) > 20000 {
		return nil, nil, &domain.Error{Code: "too_large", Message: "local price interval exceeds read budget"}
	}
	events, err := tx.QueryContext(ctx, `SELECT DISTINCT a.effective_local_date FROM activities a JOIN activity_effects e ON e.activity_id=a.id LEFT JOIN activity_dividend_details d ON d.activity_id=a.id WHERE (e.instrument_id=? OR d.instrument_id=?) AND a.effective_local_date>=? AND a.effective_local_date<=? AND ((a.kind='position_transfer' AND (a.reason='reconciliation' OR e.classification='remeasurement')) OR (a.kind='cash_dividend' AND ?)) AND NOT EXISTS(SELECT 1 FROM activities r WHERE r.reverses_activity_id=a.id)`, i.ID.String(), i.ID.String(), earliest, latest.String, i.Type != domain.InstrumentStock && i.Type != domain.InstrumentETF)
	if err != nil {
		return nil, nil, err
	}
	unsafe := []string{}
	for events.Next() {
		var d string
		if err := events.Scan(&d); err != nil {
			events.Close()
			return nil, nil, err
		}
		unsafe = append(unsafe, d)
	}
	err = events.Err()
	events.Close()
	if err != nil {
		return nil, nil, err
	}
	return quotes, unsafe, tx.Commit()
}
