package mcpserver

import (
	"context"

	"github.com/waltwang/nestworth-go/internal/agentcapability"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

// InstrumentChanges is the update_instrument patch. Omitted fields keep the
// stored value. JSON null clears shortName, symbol, marketCode, countryCode,
// isin, note, providerKey and providerSymbol, and resets iconKey to the type
// default. An empty string is rejected. Null is rejected for name, type,
// quoteCurrency, metalTemplate, quantityUnit, quoteSource and sortOrder.
// There is no replace flag: a complete form is expressed by setting or
// clearing each field.
type InstrumentChanges struct {
	ShortName      agentcapability.PatchString `json:"shortName,omitempty,omitzero"`
	Name           *string                     `json:"name,omitempty"`
	Type           *string                     `json:"type,omitempty"`
	MetalTemplate  *string                     `json:"metalTemplate,omitempty"`
	QuantityUnit   *string                     `json:"quantityUnit,omitempty"`
	QuoteCurrency  *string                     `json:"quoteCurrency,omitempty"`
	Symbol         agentcapability.PatchString `json:"symbol,omitempty,omitzero"`
	MarketCode     agentcapability.PatchString `json:"marketCode,omitempty,omitzero"`
	CountryCode    agentcapability.PatchString `json:"countryCode,omitempty,omitzero"`
	ISIN           agentcapability.PatchString `json:"isin,omitempty,omitzero"`
	Note           agentcapability.PatchString `json:"note,omitempty,omitzero"`
	IconKey        agentcapability.PatchString `json:"iconKey,omitempty,omitzero"`
	SortOrder      *int                        `json:"sortOrder,omitempty"`
	QuoteSource    *string                     `json:"quoteSource,omitempty"`
	ProviderKey    agentcapability.PatchString `json:"providerKey,omitempty,omitzero"`
	ProviderSymbol agentcapability.PatchString `json:"providerSymbol,omitempty,omitzero"`
}

func (s *Service) updateInstrument(ctx context.Context, in UpdateInstrumentInput) (any, error) {
	id, err := domain.ParseInstrumentID(in.ID)
	if err != nil {
		return nil, err
	}
	updated, err := s.app.PatchInstrument(ctx, id, in.Changes.toPatch())
	if err != nil {
		return nil, err
	}
	return wire.FromInstrument(updated), nil
}

func (in InstrumentChanges) toPatch() application.InstrumentPatch {
	sortOrder := application.IntField{}
	if in.SortOrder != nil {
		sortOrder = application.IntField{Op: application.FieldSet, Value: *in.SortOrder}
	}
	return application.InstrumentPatch{
		Name: textField(in.Name), Type: textField(in.Type), QuoteCurrency: textField(in.QuoteCurrency),
		MetalTemplate: textField(in.MetalTemplate), QuantityUnit: textField(in.QuantityUnit),
		QuoteSource: textField(in.QuoteSource), ShortName: stringField(in.ShortName),
		Symbol: stringField(in.Symbol), MarketCode: stringField(in.MarketCode),
		CountryCode: stringField(in.CountryCode), ISIN: stringField(in.ISIN),
		Note: stringField(in.Note), IconKey: stringField(in.IconKey),
		ProviderKey: stringField(in.ProviderKey), ProviderSymbol: stringField(in.ProviderSymbol),
		SortOrder: sortOrder,
	}
}

func stringField(patch agentcapability.PatchString) application.StringField {
	if patch.Null() {
		return application.StringField{Op: application.FieldClear}
	}
	if value, ok := patch.Value(); ok {
		return application.StringField{Op: application.FieldSet, Value: value}
	}
	return application.StringField{}
}

func textField(value *string) application.StringField {
	if value == nil {
		return application.StringField{}
	}
	return application.StringField{Op: application.FieldSet, Value: *value}
}
