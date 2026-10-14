package analytics

import (
	"context"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type HoldingAmountsDTO struct {
	Quantity           string                `json:"quantity"`
	TotalCost          *wire.MoneyView       `json:"totalCost,omitempty"`
	CurrentValue       *wire.MoneyView       `json:"currentValue,omitempty"`
	UnrealizedGain     *wire.SignedMoneyView `json:"unrealizedGain,omitempty"`
	CostMissingReason  string                `json:"costMissingReason,omitempty"`
	ValueMissingReason string                `json:"valueMissingReason,omitempty"`
	GainMissingReason  string                `json:"gainMissingReason,omitempty"`
}
type InstrumentHoldingMemberDTO struct {
	HoldingID   string            `json:"holdingId"`
	AccountID   string            `json:"accountId"`
	AccountName string            `json:"accountName"`
	Amounts     HoldingAmountsDTO `json:"amounts"`
}
type InstrumentHoldingsDTO struct {
	ShortName     string                       `json:"shortName,omitempty"`
	InstrumentID  string                       `json:"instrumentId"`
	Name          string                       `json:"name"`
	Symbol        string                       `json:"symbol"`
	QuoteCurrency string                       `json:"quoteCurrency"`
	QuantityUnit  string                       `json:"quantityUnit"`
	Archived      bool                         `json:"archived"`
	Amounts       HoldingAmountsDTO            `json:"amounts"`
	Holdings      []InstrumentHoldingMemberDTO `json:"holdings"`
}

func holdingAmountsDTO(a domain.HoldingAmounts) HoldingAmountsDTO {
	return HoldingAmountsDTO{Quantity: a.Quantity, TotalCost: wire.FromMoneyView(a.TotalCost), CurrentValue: wire.FromMoneyView(a.CurrentValue), UnrealizedGain: wire.FromSignedMoneyView(a.UnrealizedGain), CostMissingReason: a.CostMissingReason, ValueMissingReason: a.ValueMissingReason, GainMissingReason: a.GainMissingReason}
}
func (s *Service) InstrumentHoldings(ctx context.Context) ([]InstrumentHoldingsDTO, error) {
	groups, err := s.app.InstrumentHoldings(ctx)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return instrumentHoldingsDTO(groups), nil
}

func instrumentHoldingsDTO(groups []domain.InstrumentHoldingsView) []InstrumentHoldingsDTO {
	result := make([]InstrumentHoldingsDTO, 0, len(groups))
	for _, g := range groups {
		members := make([]InstrumentHoldingMemberDTO, 0, len(g.Holdings))
		for _, m := range g.Holdings {
			members = append(members, InstrumentHoldingMemberDTO{HoldingID: m.HoldingID.String(), AccountID: m.AccountID.String(), AccountName: m.AccountName, Amounts: holdingAmountsDTO(m.Amounts)})
		}
		result = append(result, InstrumentHoldingsDTO{InstrumentID: g.InstrumentID.String(), Name: g.Name, ShortName: g.ShortName, Symbol: g.Symbol, QuoteCurrency: g.QuoteCurrency.String(), QuantityUnit: g.QuantityUnit, Archived: g.Archived, Amounts: holdingAmountsDTO(g.Amounts), Holdings: members})
	}
	return result
}

// AccountHoldingDetails explicitly includes an archived account without changing
// the Portfolio default scope. Cost replay remains one capture for all members.
func (s *Service) AccountHoldingDetails(ctx context.Context, accountID string) ([]InstrumentHoldingsDTO, error) {
	id, err := domain.ParseAccountID(accountID)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	groups, err := s.app.AccountHoldingDetails(ctx, id)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return instrumentHoldingsDTO(groups), nil
}
