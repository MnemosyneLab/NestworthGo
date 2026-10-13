package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ReadInvestmentReviewSnapshot captures current positions, corrected dividends,
// adjustments, and actual (not cached valuation) quote facts in one transaction.
func (r *Repository) ReadInvestmentReviewSnapshot(ctx context.Context) (domain.InvestmentReviewSnapshot, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.InvestmentReviewSnapshot{}, err
	}
	defer tx.Rollback()
	if domain.IsBoundedRead(ctx) {
		for _, table := range []string{"accounts", "holdings", "instruments", "instrument_quotes", "fx_quotes", "activities", "activity_effects", "activity_trade_details", "activity_dividend_details", "history_origin_components"} {
			if err := checkReadTableBudget(ctx, tx, table, "1=1"); err != nil {
				return domain.InvestmentReviewSnapshot{}, err
			}
		}
	}
	gain, err := readGainSnapshotQuery(ctx, tx)
	if err != nil {
		return domain.InvestmentReviewSnapshot{}, err
	}
	if gain.Portfolio.Household != nil {
		gain.Portfolio.InstrumentQuotes, err = listAllInstrumentQuotesQuery(ctx, tx, gain.Portfolio.Household.ID, time.Time{})
		if err != nil {
			return domain.InvestmentReviewSnapshot{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return domain.InvestmentReviewSnapshot{}, err
	}
	return domain.InvestmentReviewSnapshot{Gain: gain}, nil
}
