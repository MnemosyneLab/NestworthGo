package history

import (
	"context"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type PositionHolding struct {
	Holding    domain.Holding
	Account    domain.Account
	Instrument domain.Instrument
}

type PositionSources struct {
	Origin   domain.HistoryOrigin
	Location *time.Location
	Accounts []domain.Account
	Holdings []PositionHolding
}

func LoadPositionSources(ctx context.Context, ledger InvestmentLedger) (out PositionSources, err error) {
	err = ledger.WithRead(ctx, func(ctx context.Context) error {
		origin, zone, err := cashMovementOrigin(ctx, ledger)
		if err != nil {
			return err
		}
		out.Origin, out.Location = origin, zone
		rows, err := ledger.ListAccounts(ctx, domain.AccountFilter{})
		if err != nil {
			return err
		}
		var ids []domain.AccountID
		for _, row := range rows {
			if TradeAccount(row.Account) {
				out.Accounts = append(out.Accounts, row.Account)
				ids = append(ids, row.Account.ID)
			}
		}
		if len(ids) == 0 {
			return nil
		}
		holdings, err := ledger.HoldingsByAccounts(ctx, ids)
		if err != nil {
			return err
		}
		instruments, err := ledger.ListInstruments(ctx, false)
		if err != nil {
			return err
		}
		ownership, err := ledger.InstrumentProductOwnership(ctx)
		if err != nil {
			return err
		}
		byID := make(map[domain.InstrumentID]domain.Instrument, len(instruments))
		for _, instrument := range instruments {
			if instrument.ArchivedAt == nil && ownership[instrument.ID.String()] == "" {
				byID[instrument.ID] = instrument
			}
		}
		for _, account := range out.Accounts {
			for _, h := range holdings[account.ID] {
				instrument, ordinary := byID[h.InstrumentID]
				if ordinary && h.ArchivedAt == nil {
					out.Holdings = append(out.Holdings, PositionHolding{Holding: h, Account: account, Instrument: instrument})
				}
			}
		}
		return nil
	})
	return out, err
}
