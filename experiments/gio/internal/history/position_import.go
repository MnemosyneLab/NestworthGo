package history

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type PositionImportInput struct {
	Account     domain.AccountID
	Instrument  domain.InstrumentID
	Currency    domain.CurrencyCode
	Quantity    string
	UnitCost    string
	Note        string
	Date        string
	Time        string
	EffectiveAt time.Time
}

func ReviewPositionImport(ctx context.Context, ledger InvestmentLedger, input PositionImportInput) (ReviewedInvestment, error) {
	var review ReviewedInvestment
	err := ledger.WithRead(ctx, func(ctx context.Context) error {
		preview, token, err := ledger.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
			sources, err := investmentSources(ctx, ledger, input.Account, domain.ActivityPositionTransfer)
			if err != nil {
				return nil, err
			}
			at := slices.IndexFunc(sources.Instruments, func(i domain.Instrument) bool { return i.ID == input.Instrument })
			if at < 0 {
				return nil, &domain.Error{Code: domain.ErrValidation, Field: "instrumentId"}
			}
			if sources.Instruments[at].QuoteCurrency != input.Currency {
				return nil, &domain.Error{Code: domain.ErrStalePreview, Field: "currency"}
			}
			request, err := positionImportRequest(input)
			if err != nil {
				return nil, err
			}
			return review.bind(sources, request)
		})
		review.Preview, review.token = preview, token
		return err
	})
	if err != nil {
		return ReviewedInvestment{}, err
	}
	return review, nil
}

func positionImportRequest(input PositionImportInput) (historyapi.ChangeCommandRequest, error) {
	var request historyapi.ChangeCommandRequest
	quantity, err := domain.ParseQuantity(strings.TrimSpace(input.Quantity))
	if err != nil || quantity.IsZero() {
		return request, &domain.Error{Code: domain.ErrValidation, Field: "quantity"}
	}
	cost, err := domain.ParseUnitPrice(strings.TrimSpace(input.UnitCost))
	if err != nil {
		return request, &domain.Error{Code: domain.ErrValidation, Field: "unitCost"}
	}
	if input.EffectiveAt.IsZero() && (input.Date == "" || input.Time == "") {
		return request, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveLocalDateTime"}
	}
	request = historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionImport, AccountID: input.Account.String(), InstrumentID: input.Instrument.String(),
		Quantity: quantity.Canonical(), UnitCost: cost.Canonical(), Currency: input.Currency.String(),
	}
	if note := strings.TrimSpace(input.Note); note != "" {
		request.Note = &note
	}
	if input.EffectiveAt.IsZero() {
		request.EffectiveLocalDate, request.EffectiveLocalTime = input.Date, input.Time
	} else {
		request.EffectiveAt = input.EffectiveAt.UTC().Format(time.RFC3339Nano)
	}
	return request, nil
}
