package history

import (
	"context"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type CustodyTransferInput struct {
	From        domain.HoldingID
	To          domain.AccountID
	ToHolding   domain.HoldingID
	Quantity    string
	Note        string
	Date        string
	Time        string
	EffectiveAt time.Time
	Link        *domain.ReconciliationLink
}

func ReviewCustodyTransfer(ctx context.Context, ledger InvestmentLedger, input CustodyTransferInput) (ReviewedInvestment, error) {
	request := historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionTransfer, ReconciliationLink: input.Link, FromHoldingID: input.From.String(),
		ToAccountID: input.To.String(), ToHoldingID: input.ToHolding.String(), Quantity: strings.TrimSpace(input.Quantity),
	}
	return reviewPositionCommand(ctx, ledger, request, input.Note, input.Date, input.Time, input.EffectiveAt)
}
