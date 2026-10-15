package analytics

import (
	"context"
	"errors"

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
	Type          string                       `json:"type"`
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
	// Match HistoricalOverview: an abandoned Wails read completes quietly.
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil, nil
	}
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
		result = append(result, InstrumentHoldingsDTO{InstrumentID: g.InstrumentID.String(), Type: string(g.Type), Name: g.Name, ShortName: g.ShortName, Symbol: g.Symbol, QuoteCurrency: g.QuoteCurrency.String(), QuantityUnit: g.QuantityUnit, Archived: g.Archived, Amounts: holdingAmountsDTO(g.Amounts), Holdings: members})
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
	// Match HistoricalOverview: an abandoned Wails read completes quietly.
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil, nil
	}
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return instrumentHoldingsDTO(groups), nil
}

// HoldingSubtotalDTO sums one native currency inside a holding group. Quantity
// is intentionally absent because units of different instruments do not add.
type HoldingSubtotalDTO struct {
	Currency           string                `json:"currency"`
	InstrumentCount    int                   `json:"instrumentCount"`
	TotalCost          *wire.MoneyView       `json:"totalCost,omitempty"`
	CurrentValue       *wire.MoneyView       `json:"currentValue,omitempty"`
	UnrealizedGain     *wire.SignedMoneyView `json:"unrealizedGain,omitempty"`
	CostMissingReason  string                `json:"costMissingReason,omitempty"`
	ValueMissingReason string                `json:"valueMissingReason,omitempty"`
	GainMissingReason  string                `json:"gainMissingReason,omitempty"`
}

// InstrumentHoldingGroupDTO is one instrument-type section of the holdings list.
type InstrumentHoldingGroupDTO struct {
	Key         string                  `json:"key"`
	Instruments []InstrumentHoldingsDTO `json:"instruments"`
	Subtotals   []HoldingSubtotalDTO    `json:"subtotals"`
}

func instrumentHoldingGroupsDTO(groups []domain.InstrumentHoldingGroupView) []InstrumentHoldingGroupDTO {
	result := make([]InstrumentHoldingGroupDTO, 0, len(groups))
	for _, group := range groups {
		subtotals := make([]HoldingSubtotalDTO, 0, len(group.Subtotals))
		for _, subtotal := range group.Subtotals {
			a := holdingAmountsDTO(subtotal.Amounts)
			subtotals = append(subtotals, HoldingSubtotalDTO{Currency: subtotal.Currency.String(), InstrumentCount: subtotal.InstrumentCount, TotalCost: a.TotalCost, CurrentValue: a.CurrentValue, UnrealizedGain: a.UnrealizedGain, CostMissingReason: a.CostMissingReason, ValueMissingReason: a.ValueMissingReason, GainMissingReason: a.GainMissingReason})
		}
		result = append(result, InstrumentHoldingGroupDTO{Key: string(group.Key), Instruments: instrumentHoldingsDTO(group.Instruments), Subtotals: subtotals})
	}
	return result
}

// InstrumentHoldingGroups returns Portfolio holdings grouped by instrument type
// with per-currency subtotals.
func (s *Service) InstrumentHoldingGroups(ctx context.Context) ([]InstrumentHoldingGroupDTO, error) {
	groups, err := s.app.InstrumentHoldingGroups(ctx)
	// Match HistoricalOverview: an abandoned Wails read completes quietly.
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil, nil
	}
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return instrumentHoldingGroupsDTO(groups), nil
}

// AccountHoldingGroups is InstrumentHoldingGroups scoped to one account,
// including an archived account.
func (s *Service) AccountHoldingGroups(ctx context.Context, accountID string) ([]InstrumentHoldingGroupDTO, error) {
	id, err := domain.ParseAccountID(accountID)
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	groups, err := s.app.AccountHoldingGroups(ctx, id)
	// Match HistoricalOverview: an abandoned Wails read completes quietly.
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil, nil
	}
	if err != nil {
		return nil, apierror.Wrap(err)
	}
	return instrumentHoldingGroupsDTO(groups), nil
}
