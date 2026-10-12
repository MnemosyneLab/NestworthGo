package wire

import "github.com/waltwang/nestworth-go/internal/domain"

type AccountCompositionItemDTO struct {
	Key             string     `json:"key"`
	TypeKey         string     `json:"typeKey"`
	Label           string     `json:"label"`
	DetailRefs      []string   `json:"detailRefs"`
	BaseAmount      *MoneyView `json:"baseAmount,omitempty"`
	BaseAmountExact string     `json:"baseAmountExact,omitempty"`
	ShareBPS        int        `json:"shareBps"`
	NativeAmount    string     `json:"nativeAmount,omitempty"`
	NativeCurrency  string     `json:"nativeCurrency,omitempty"`
	ValuedCount     int        `json:"valuedCount"`
	MissingCount    int        `json:"missingCount"`
}

type AccountCompositionDTO struct {
	Currency     string                      `json:"currency"`
	TotalExact   string                      `json:"totalExact,omitempty"`
	Complete     bool                        `json:"complete"`
	Status       string                      `json:"status"`
	ValuedCount  int                         `json:"valuedCount"`
	MissingCount int                         `json:"missingCount"`
	Types        []AccountCompositionItemDTO `json:"types"`
	Details      []AccountCompositionItemDTO `json:"details"`
	DetailSlices []AccountCompositionItemDTO `json:"detailSlices"`
}

func FromAccountComposition(value *domain.AccountComposition) *AccountCompositionDTO {
	if value == nil {
		return nil
	}
	return &AccountCompositionDTO{Currency: value.Currency.String(), TotalExact: value.TotalExact, Complete: value.Complete, Status: value.Status, ValuedCount: value.ValuedCount, MissingCount: value.MissingCount, Types: fromCompositionItems(value.Types), Details: fromCompositionItems(value.Details), DetailSlices: fromCompositionItems(value.DetailSlices)}
}

func fromCompositionItems(values []domain.AccountCompositionItem) []AccountCompositionItemDTO {
	result := make([]AccountCompositionItemDTO, 0, len(values))
	for _, item := range values {
		result = append(result, AccountCompositionItemDTO{Key: item.Key, TypeKey: item.TypeKey, Label: item.Label, DetailRefs: item.DetailRefs, BaseAmount: FromMoneyView(item.BaseAmount), BaseAmountExact: item.BaseAmountExact, ShareBPS: item.ShareBPS, NativeAmount: item.NativeAmount, NativeCurrency: item.NativeCurrency.String(), ValuedCount: item.ValuedCount, MissingCount: item.MissingCount})
	}
	return result
}
