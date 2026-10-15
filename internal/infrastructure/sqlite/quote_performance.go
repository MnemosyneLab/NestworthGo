package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ReadQuotePerformanceEvidence reads latest daily evidence, nearest baselines,
// and intervening instrument quotes in one transaction, independently of chart range.
func (r *Repository) ReadQuotePerformanceEvidence(ctx context.Context, i domain.Instrument, today string) ([]domain.InstrumentQuote, error) {
	return r.ReadQuotePerformanceEvidenceWindows(ctx, i, today, domain.QuotePerformanceWindows)
}

// ReadQuotePerformanceEvidenceWindows is ReadQuotePerformanceEvidence for an
// explicit set of lookback windows. A window of zero reads the whole history.
func (r *Repository) ReadQuotePerformanceEvidenceWindows(ctx context.Context, i domain.Instrument, today string, windows []int) ([]domain.InstrumentQuote, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	where := `q.instrument_id=? AND q.source_kind IN ('agent','manual','provider') AND COALESCE(q.binding_revision,0)=? AND q.currency=? AND q.observation_kind='close' AND q.effective_date<>'' AND q.effective_date<=? AND NOT EXISTS(SELECT 1 FROM agent_quote_records w WHERE w.target_type='instrument' AND w.target_quote_id=q.id)`
	args := []any{i.ID.String(), i.ProviderBindingRevision, i.QuoteCurrency.String(), today}
	var latest sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT MAX(q.effective_date) FROM instrument_quotes q WHERE "+where, args...).Scan(&latest); err != nil {
		return nil, err
	}
	if !latest.Valid {
		return []domain.InstrumentQuote{}, tx.Commit()
	}
	end, err := time.Parse(time.DateOnly, latest.String)
	if err != nil {
		return nil, err
	}
	earliest := latest.String
	for _, days := range windows {
		var base sql.NullString
		if days <= 0 {
			if err := tx.QueryRowContext(ctx, "SELECT MIN(q.effective_date) FROM instrument_quotes q WHERE "+where, args...).Scan(&base); err != nil {
				return nil, err
			}
		} else {
			target := end.AddDate(0, 0, -days).Format(time.DateOnly)
			params := append(append([]any{}, args...), target)
			if err := tx.QueryRowContext(ctx, "SELECT MAX(q.effective_date) FROM instrument_quotes q WHERE "+where+" AND q.effective_date<=?", params...).Scan(&base); err != nil {
				return nil, err
			}
		}
		if base.Valid && base.String < earliest {
			earliest = base.String
		}
	}
	where += " AND q.effective_date>=?"
	args = append(args, earliest)
	if err := checkReadTableBudget(ctx, tx, "instrument_quotes q", where, args...); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT "+marketInstrumentQuoteColumns+" FROM instrument_quotes q WHERE "+where+" ORDER BY q.effective_date,q.revision,q.created_at,q.id LIMIT 20001", args...)
	if err != nil {
		return nil, err
	}
	quotes, err := scanInstrumentQuotes(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(quotes) > 20000 {
		return nil, &domain.Error{Code: "too_large", Message: "local price interval exceeds read budget"}
	}
	return quotes, tx.Commit()
}
