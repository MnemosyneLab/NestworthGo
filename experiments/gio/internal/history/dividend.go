package history

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type DividendInput struct {
	Account     domain.AccountID
	Instrument  domain.InstrumentID
	Holding     domain.HoldingID
	Amount      string
	Date        string
	Time        string
	Note        string
	Currency    domain.CurrencyCode
	EffectiveAt time.Time
	Link        *domain.ReconciliationLink
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
		currency := input.Currency
		if currency == "" && input.Link == nil {
			currency = sources.Account.DefaultCurrency
		}
		currency, err = domain.ParseSupportedCurrency(currency.String())
		if err != nil {
			return nil, err
		}
		amount, err := domain.ParseMoney(strings.TrimSpace(input.Amount), currency)
		if err != nil || amount.IsZero() {
			return nil, &domain.Error{Code: domain.ErrValidation, Field: "amount"}
		}
		if input.EffectiveAt.IsZero() && (strings.TrimSpace(input.Date) == "" || strings.TrimSpace(input.Time) == "") {
			return nil, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveLocalDateTime"}
		}
		request := historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeCashDividend, HoldingID: input.Holding.String(), ReconciliationLink: input.Link,
			Amount: amount.CanonicalAmount(), Currency: currency.String(),
		}
		if input.EffectiveAt.IsZero() {
			request.EffectiveLocalDate, request.EffectiveLocalTime = input.Date, input.Time
		} else {
			request.EffectiveAt = input.EffectiveAt.UTC().Format(time.RFC3339Nano)
		}
		if note := strings.TrimSpace(input.Note); note != "" {
			request.Note = &note
		}
		return review.bind(sources.Origin, sources.Location, request)
	})
	if err != nil {
		return ReviewedInvestment{}, err
	}
	review.Preview, review.token = preview, token
	if err := review.validateLink(ctx, ledger); err != nil {
		return ReviewedInvestment{}, err
	}
	return review, nil
}
