package history

import (
	"context"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type CustodyHolding struct {
	Holding    domain.Holding
	Account    domain.Account
	Instrument domain.Instrument
}

type CustodySources struct {
	Origin   domain.HistoryOrigin
	Location *time.Location
	Accounts []domain.Account
	Holdings []CustodyHolding
}

func LoadCustodySources(ctx context.Context, ledger InvestmentLedger) (out CustodySources, err error) {
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
		if len(ids) < 2 {
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
				if ordinary && h.ArchivedAt == nil && !h.Quantity.IsZero() {
					out.Holdings = append(out.Holdings, CustodyHolding{Holding: h, Account: account, Instrument: instrument})
				}
			}
		}
		return nil
	})
	return out, err
}

type CustodyTransferInput struct {
	From        domain.HoldingID
	To          domain.AccountID
	Quantity    string
	Note        string
	Date        string
	Time        string
	EffectiveAt time.Time
}

func ReviewCustodyTransfer(ctx context.Context, ledger InvestmentLedger, input CustodyTransferInput) (ReviewedInvestment, error) {
	var review ReviewedInvestment
	err := ledger.WithRead(ctx, func(ctx context.Context) error {
		preview, token, err := ledger.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
			origin, location, err := cashMovementOrigin(ctx, ledger)
			if err != nil {
				return nil, err
			}
			request := historyapi.ChangeCommandRequest{
				Kind: historyapi.ChangePositionTransfer, FromHoldingID: input.From.String(),
				ToAccountID: input.To.String(), Quantity: strings.TrimSpace(input.Quantity),
				EffectiveLocalDate: input.Date, EffectiveLocalTime: input.Time,
			}
			if !input.EffectiveAt.IsZero() {
				request.EffectiveLocalDate, request.EffectiveLocalTime = "", ""
				request.EffectiveAt = input.EffectiveAt.UTC().Format(time.RFC3339Nano)
			}
			if note := strings.TrimSpace(input.Note); note != "" {
				request.Note = &note
			}
			return review.bind(origin, location, request)
		})
		review.Preview, review.token = preview, token
		return err
	})
	if err != nil {
		return ReviewedInvestment{}, err
	}
	return review, nil
}
