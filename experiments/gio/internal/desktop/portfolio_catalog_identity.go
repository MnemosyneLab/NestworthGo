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
	i18n.PortfolioFormName, i18n.PortfolioFormShortName, i18n.PortfolioInstrumentSymbol,
	i18n.PortfolioInstrumentMarket, i18n.PortfolioInstrumentISIN, i18n.PortfolioInstrumentCountry, i18n.PortfolioInstrumentNote,
}

func catalogIdentityValues(item domain.Instrument) [portfolioIdentityFieldCount]string {
	return [portfolioIdentityFieldCount]string{
		item.Name, item.ShortName, productString(item.Symbol), productString(item.MarketCode),
		productString(item.ISIN), productString(item.CountryCode), productString(item.Note),
	}
}

func newPortfolioCatalogDraft(item domain.Instrument, button *widget.Clickable) *portfolioCatalogDraft {
	sheet := newAnalysisSheet()
	sheet.width, sheet.modal, sheet.focus = 448, true, false
	if item.ID != "" {
		sheet.width = 576
	}
	d := &portfolioCatalogDraft{instrument: item, returnTo: button, sheet: sheet, opening: true}
	for i, value := range catalogIdentityValues(item) {
		d.fields[i] = ui.NewTextField(i != portfolioInstrumentNote)
		d.fields[i].Editor.SetText(value)
	}
	d.config = newPortfolioInstrumentConfig(item)
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
