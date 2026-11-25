package history

import (
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

// RecordedCustody identifies the two immutable legs of an ordinary transfer
// independently of the current ownership, quantity and value of each position.
type RecordedCustody struct {
	From, To domain.ActivityEffect
}

func CustodyAmendment(activity domain.Activity) (RecordedCustody, bool) {
	if activity.Kind != domain.ActivityPositionTransfer || activity.Reason != domain.ReasonOther ||
		activity.ReversesActivityID != nil || activity.ProductContext != nil || len(activity.Effects) != 2 {
		return RecordedCustody{}, false
	}
	var transfer RecordedCustody
	for _, leg := range activity.Effects {
		if leg.Target != domain.EffectTargetHoldingQuantity || leg.HoldingID == nil || leg.InstrumentID == nil || leg.Quantity == nil {
			return RecordedCustody{}, false
		}
		switch leg.Role {
		case domain.EffectRoleTransferFrom:
			transfer.From = leg
		case domain.EffectRoleTransferTo:
			transfer.To = leg
		}
	}
	return transfer, transfer.From.HoldingID != nil && transfer.To.HoldingID != nil
}

func custodyReplacement(input AmendmentInput) historyapi.ChangeCommandRequest {
	return historyapi.ChangeCommandRequest{
		Kind: historyapi.ChangePositionTransfer, FromHoldingID: input.From.String(), ToHoldingID: input.To.String(),
		Quantity: strings.TrimSpace(input.Amount), Note: &input.Note,
	}
}
