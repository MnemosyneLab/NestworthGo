package application

import (
	"strings"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// FieldOp is the presence of one field on a partial instrument edit.
// The zero value keeps the stored field.
type FieldOp uint8

const (
	FieldKeep FieldOp = iota
	FieldSet
	FieldClear
)

// StringField is one string on an instrument patch. FieldSet carries Value.
type StringField struct {
	Op    FieldOp
	Value string
}

// IntField is one integer on an instrument patch. Zero is a real sort order
// when Op is FieldSet, and FieldClear is rejected because every instrument has
// an order.
type IntField struct {
	Op    FieldOp
	Value int
}

// InstrumentPatch is the MCP partial instrument edit. It is applied to the
// current row inside the write lock. It does not use the GUI replace flag.
type InstrumentPatch struct {
	Name, Type, QuoteCurrency, MetalTemplate, QuantityUnit, QuoteSource StringField
	ShortName, Symbol, MarketCode, CountryCode, ISIN                    StringField
	Note, IconKey, ProviderKey, ProviderSymbol                          StringField
	SortOrder                                                           IntField
}

func instrumentInputFromCurrent(current domain.Instrument) InstrumentInput {
	short := current.ShortName
	input := InstrumentInput{
		Replace: true, Name: current.Name, ShortName: &short, Type: string(current.Type),
		MetalTemplate: current.MetalTemplate, QuantityUnit: current.QuantityUnit,
		QuoteCurrency: current.QuoteCurrency.String(), Symbol: pointerText(current.Symbol),
		MarketCode: pointerText(current.MarketCode), CountryCode: pointerText(current.CountryCode),
		ISIN: pointerText(current.ISIN), IconKey: pointerText(current.IconKey), SortOrder: current.SortOrder,
		QuoteSource: string(current.QuoteSource), ProviderKey: pointerText(current.ProviderKey),
		ProviderSymbol: pointerText(current.ProviderSymbol),
	}
	if current.Note != nil {
		note := *current.Note
		input.Note = &note
	}
	return input
}

func applyInstrumentPatch(input *InstrumentInput, patch InstrumentPatch) error {
	if err := assignString("name", patch.Name, false, func(value string) { input.Name = value }); err != nil {
		return err
	}
	if err := assignString("type", patch.Type, false, func(value string) { input.Type = value }); err != nil {
		return err
	}
	if err := assignString("quoteCurrency", patch.QuoteCurrency, false, func(value string) { input.QuoteCurrency = value }); err != nil {
		return err
	}
	if err := assignString("metalTemplate", patch.MetalTemplate, false, func(value string) { input.MetalTemplate = value }); err != nil {
		return err
	}
	if err := assignString("quantityUnit", patch.QuantityUnit, false, func(value string) { input.QuantityUnit = value }); err != nil {
		return err
	}
	if err := assignString("quoteSource", patch.QuoteSource, false, func(value string) { input.QuoteSource = value }); err != nil {
		return err
	}
	if err := assignString("shortName", patch.ShortName, true, func(value string) { input.ShortName = &value }); err != nil {
		return err
	}
	if err := assignString("symbol", patch.Symbol, true, func(value string) { input.Symbol = value }); err != nil {
		return err
	}
	if err := assignString("marketCode", patch.MarketCode, true, func(value string) { input.MarketCode = value }); err != nil {
		return err
	}
	if err := assignString("countryCode", patch.CountryCode, true, func(value string) { input.CountryCode = value }); err != nil {
		return err
	}
	if err := assignString("isin", patch.ISIN, true, func(value string) { input.ISIN = value }); err != nil {
		return err
	}
	if err := assignNote(patch.Note, input); err != nil {
		return err
	}
	if err := assignString("iconKey", patch.IconKey, true, func(value string) { input.IconKey = value }); err != nil {
		return err
	}
	if err := assignString("providerKey", patch.ProviderKey, true, func(value string) { input.ProviderKey = value }); err != nil {
		return err
	}
	if err := assignString("providerSymbol", patch.ProviderSymbol, true, func(value string) { input.ProviderSymbol = value }); err != nil {
		return err
	}
	switch patch.SortOrder.Op {
	case FieldKeep:
	case FieldSet:
		input.SortOrder = patch.SortOrder.Value
	case FieldClear:
		return &domain.Error{Code: domain.ErrValidation, Field: "sortOrder", Message: "cannot be cleared; omit it to keep the stored value, including 0"}
	default:
		return &domain.Error{Code: domain.ErrValidation, Field: "sortOrder", Message: "invalid patch"}
	}
	return nil
}

func assignString(field string, edit StringField, clearable bool, set func(string)) error {
	switch edit.Op {
	case FieldKeep:
		return nil
	case FieldClear:
		if !clearable {
			return &domain.Error{Code: domain.ErrValidation, Field: field, Message: "cannot be cleared; omit it to keep the stored value"}
		}
		// An empty string is the domain's cleared optional text. Icon keys fall
		// back to the instrument type default, which is the clear for that field.
		set("")
		return nil
	case FieldSet:
		if strings.TrimSpace(edit.Value) == "" {
			return &domain.Error{Code: domain.ErrValidation, Field: field, Message: "must be nonempty; null clears a clearable field and omission keeps the stored value"}
		}
		set(edit.Value)
		return nil
	default:
		return &domain.Error{Code: domain.ErrValidation, Field: field, Message: "invalid patch"}
	}
}

func assignNote(edit StringField, input *InstrumentInput) error {
	switch edit.Op {
	case FieldKeep:
		return nil
	case FieldClear:
		input.Note = nil
		return nil
	case FieldSet:
		if strings.TrimSpace(edit.Value) == "" {
			return &domain.Error{Code: domain.ErrValidation, Field: "note", Message: "must be nonempty; null clears a clearable field and omission keeps the stored value"}
		}
		note := edit.Value
		input.Note = &note
		return nil
	default:
		return &domain.Error{Code: domain.ErrValidation, Field: "note", Message: "invalid patch"}
	}
}
