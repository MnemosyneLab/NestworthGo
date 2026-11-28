package history

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type InvestmentReplacementSources struct {
	Accounts                []domain.Account
	Holdings                []PositionHolding
	OriginalTradeRestricted bool
}

// LoadInvestmentReplacementSources returns the active choices for a historical
// replacement. Trades can retain their own archived position for a restricted
// correction. A different archived holding is never a replacement choice.
func LoadInvestmentReplacementSources(ctx context.Context, ledger PositionSourceReader, original domain.Activity) (out InvestmentReplacementSources, err error) {
	investment, ok := InvestmentAmendment(original)
	if !ok {
		return out, &domain.Error{Code: domain.ErrInvalidChange, Field: "activityId"}
	}
	err = ledger.WithRead(ctx, func(ctx context.Context) error {
		rows, err := ledger.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
		if err != nil {
			return err
		}
		var positionAccounts []domain.Account
		var ids []domain.AccountID
		for _, row := range rows {
			account := row.Account
			if account.ArchivedAt == nil {
				out.Accounts = append(out.Accounts, account)
			}
			if !TradeAccount(account) && account.ID != investment.Account {
				continue
			}
			positionAccounts = append(positionAccounts, account)
			ids = append(ids, account.ID)
		}
		holdings, err := ledger.HoldingsByAccounts(ctx, ids)
		if err != nil {
			return err
		}
		instruments, err := ledger.ListInstruments(ctx, true)
		if err != nil {
			return err
		}
		ownership, err := ledger.InstrumentProductOwnership(ctx)
		if err != nil {
			return err
		}
		ordinary := make(map[domain.InstrumentID]domain.Instrument, len(instruments))
		for _, instrument := range instruments {
			if ownership[instrument.ID.String()] == "" {
				ordinary[instrument.ID] = instrument
			}
		}
		foundOriginal := false
		for _, account := range positionAccounts {
			for _, holding := range holdings[account.ID] {
				instrument, ok := ordinary[holding.InstrumentID]
				if !ok || holding.AccountID != account.ID {
					continue
				}
				position := PositionHolding{Holding: holding, Account: account, Instrument: instrument}
				if account.ID == investment.Account && holding.ID == investment.holding && instrument.ID == investment.Instrument {
					foundOriginal = true
					out.OriginalTradeRestricted = investment.restrictedTrade(position)
					if out.OriginalTradeRestricted {
						out.Holdings = append(out.Holdings, position)
						continue
					}
				}
				if TradeAccount(account) && holding.ArchivedAt == nil && instrument.ArchivedAt == nil {
					out.Holdings = append(out.Holdings, position)
				}
			}
		}
		if !foundOriginal {
			return &domain.Error{Code: domain.ErrInvalidChange, Field: "activityId"}
		}
		return nil
	})
	return out, err
}
