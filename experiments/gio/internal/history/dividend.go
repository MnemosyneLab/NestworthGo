package history

import (
	"context"
	"slices"
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type DividendInput struct {
	Account    domain.AccountID
	Instrument domain.InstrumentID
	Holding    domain.HoldingID
	Amount     string
	Date       string
	Time       string
}

func ReviewDividend(ctx context.Context, ledger InvestmentLedger, input DividendInput) (ReviewedInvestment, error) {
	var review ReviewedInvestment
	preview, token, err := ledger.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
		sources, err := investmentSources(ctx, ledger, input.Account, domain.ActivityCashDividend)
		if err != nil {
			return nil, err
		}
		if input.Holding == "" || sources.Holdings[input.Instrument].ID != input.Holding ||
			!slices.ContainsFunc(sources.Instruments, func(i domain.Instrument) bool { return i.ID == input.Instrument }) {
			return nil, &domain.Error{Code: domain.ErrValidation, Field: "holdingId"}
		}
		currency := sources.Account.DefaultCurrency
		amount, err := domain.ParseMoney(strings.TrimSpace(input.Amount), currency)
		if err != nil || amount.IsZero() {
			return nil, &domain.Error{Code: domain.ErrValidation, Field: "amount"}
		}
		if strings.TrimSpace(input.Date) == "" || strings.TrimSpace(input.Time) == "" {
			return nil, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveLocalDateTime"}
		}
		return review.bind(sources, historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeCashDividend, HoldingID: input.Holding.String(),
			Amount: amount.CanonicalAmount(), Currency: currency.String(),
			EffectiveLocalDate: input.Date, EffectiveLocalTime: input.Time,
		})
	})
	if err != nil {
		return ReviewedInvestment{}, err
	}
	review.Preview, review.token = preview, token
	return review, nil
}
