package desktop

import (
	"strings"

	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const (
	portfolioInstrumentName = iota
	portfolioInstrumentShortName
	portfolioInstrumentSymbol
	portfolioInstrumentMarket
	portfolioInstrumentISIN
	portfolioInstrumentCountry
	portfolioInstrumentNote
	portfolioIdentityFieldCount
)

var catalogIdentityLabels = [portfolioIdentityFieldCount]i18n.MessageID{
	i18n.PortfolioCatalogName, i18n.PortfolioCatalogShortName, i18n.PortfolioCatalogSymbol,
	i18n.PortfolioCatalogMarket, i18n.PortfolioCatalogISIN, i18n.PortfolioCatalogCountry, i18n.PreviewNotes,
}

func catalogIdentityValues(item domain.Instrument) [portfolioIdentityFieldCount]string {
	return [portfolioIdentityFieldCount]string{
		item.Name, item.ShortName, productString(item.Symbol), productString(item.MarketCode),
		productString(item.ISIN), productString(item.CountryCode), productString(item.Note),
	}
}

func newPortfolioCatalogDraft(item domain.Instrument, button *widget.Clickable) *portfolioCatalogDraft {
	d := &portfolioCatalogDraft{instrument: item, returnTo: button}
	for i, value := range catalogIdentityValues(item) {
		d.fields[i] = ui.NewTextField(i != portfolioInstrumentNote)
		d.fields[i].Editor.SetText(value)
	}
	return d
}

func (d *portfolioCatalogDraft) identityPatch() application.InstrumentPatch {
	values := catalogIdentityValues(d.instrument)
	field := func(i int) application.StringField {
		value := strings.TrimSpace(d.fields[i].Text())
		switch {
		case values[i] == value:
			return application.StringField{}
		case value == "" && i != portfolioInstrumentName:
			return application.StringField{Op: application.FieldClear}
		default:
			return application.StringField{Op: application.FieldSet, Value: value}
		}
	}
	return application.InstrumentPatch{
		Name: field(portfolioInstrumentName), ShortName: field(portfolioInstrumentShortName),
		Symbol: field(portfolioInstrumentSymbol), MarketCode: field(portfolioInstrumentMarket),
		ISIN: field(portfolioInstrumentISIN), CountryCode: field(portfolioInstrumentCountry), Note: field(portfolioInstrumentNote),
	}
}
