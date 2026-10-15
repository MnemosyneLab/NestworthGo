package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ReadInvestmentReviewSnapshot reads positions only to select held instruments.
// Rate evidence is actual instrument quotes; no cost replay or ledger is read.
func (r *Repository) ReadInvestmentReviewSnapshot(ctx context.Context) (domain.InvestmentReviewSnapshot, error) {
	tx, err := r.database.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.InvestmentReviewSnapshot{}, err
	}
	defer tx.Rollback()
	if domain.IsBoundedRead(ctx) {
		for _, table := range []string{"accounts", "holdings", "instruments", "instrument_quotes"} {
			if err := checkReadTableBudget(ctx, tx, table, "1=1"); err != nil {
				return domain.InvestmentReviewSnapshot{}, err
			}
		}
	}
	household, err := scanHousehold(tx.QueryRowContext(ctx, `SELECT id, name, base_currency, created_at, updated_at FROM households WHERE singleton_key = 1`))
	if err != nil {
		return domain.InvestmentReviewSnapshot{}, err
	}
	portfolio := domain.PortfolioSnapshot{Household: household}
	if household != nil {
		portfolio.Origin, err = historyOriginQuery(ctx, tx, household.ID)
		if err != nil {
			return domain.InvestmentReviewSnapshot{}, err
		}
		portfolio.Accounts, err = listAccountRecords(ctx, tx, household.ID, domain.AccountFilter{IncludeArchived: true})
		if err != nil {
			return domain.InvestmentReviewSnapshot{}, err
		}
		portfolio.Holdings, err = listHoldingsQuery(ctx, tx, household.ID, true)
		if err != nil {
			return domain.InvestmentReviewSnapshot{}, err
		}
		portfolio.Instruments, err = listInstrumentsQuery(ctx, tx, household.ID, true)
		if err != nil {
			return domain.InvestmentReviewSnapshot{}, err
		}
		portfolio.InstrumentQuotes, err = listAllInstrumentQuotesQuery(ctx, tx, household.ID, time.Time{})
		if err != nil {
			return domain.InvestmentReviewSnapshot{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return domain.InvestmentReviewSnapshot{}, err
	}
	return domain.InvestmentReviewSnapshot{Portfolio: portfolio}, nil
}
