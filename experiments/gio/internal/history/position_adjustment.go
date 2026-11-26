package history

import (
	"context"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type QuantityAdjustmentInput struct {
	Holding     domain.HoldingID
	Added       bool
	Quantity    string
	UnitCost    string
	Note        string
	Date        string
	Time        string
	EffectiveAt time.Time
	Link        *domain.ReconciliationLink
}

func ReviewQuantityAdjustment(ctx context.Context, ledger InvestmentLedger, input QuantityAdjustmentInput) (ReviewedInvestment, error) {
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionAdjustment, HoldingID: input.Holding.String(),
		Added: input.Added, Quantity: strings.TrimSpace(input.Quantity),
		ReconciliationLink: input.Link,
	}
	if input.Added {
		request.UnitCost = strings.TrimSpace(input.UnitCost)
	}
	return reviewPositionCommand(ctx, ledger, request, input.Note, input.Date, input.Time, input.EffectiveAt)
}

func reviewPositionCommand(ctx context.Context, ledger InvestmentLedger, request historyapi.ChangeCommandRequest, note, date, clock string, at time.Time) (ReviewedInvestment, error) {
	if at.IsZero() {
		request.EffectiveLocalDate, request.EffectiveLocalTime = date, clock
	} else {
		request.EffectiveAt = at.UTC().Format(time.RFC3339Nano)
	}
	if value := strings.TrimSpace(note); value != "" {
		request.Note = &value
	}
	var review ReviewedInvestment
	err := ledger.WithRead(ctx, func(ctx context.Context) error {
		preview, token, err := ledger.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
			origin, location, err := cashMovementOrigin(ctx, ledger)
			if err != nil {
				return nil, err
			}
			return review.bind(origin, location, request)
		})
		review.Preview, review.token = preview, token
		if err != nil {
			return err
		}
		return review.validateLink(ctx, ledger)
	})
	if err != nil {
		return ReviewedInvestment{}, err
	}
	return review, nil
}
