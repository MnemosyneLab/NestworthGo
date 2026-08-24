package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// NewMarketDataPage is the standalone home for reusable instruments and FX
// preferences. Account pages only consume these records; they do not own the
// management actions.
func NewMarketDataPage(c *Controller) fyne.CanvasObject {
	if c.service == nil {
		return NewComingSoonPage(c, "page.marketDataTitle", "page.marketDataDescription", theme.IconNameStorage)
	}
	instruments, err := c.service.ListInstruments(context.Background(), true)
	if err != nil {
		return errorPanel(c, c.translator.T("portfolio.loadError"), err)
	}
	rows := []fyne.CanvasObject{
		sectionCard(c.translator.T("marketData.instruments"), c.translator.T("portfolio.instrumentsDescription"), container.NewVBox(
			widget.NewButton(c.translator.T("portfolio.instruments"), func() { showInstrumentManagementDialog(c) }),
			marketDataInstrumentSummary(c, instruments),
		)),
		sectionCard(c.translator.T("marketData.fx"), c.translator.T("portfolio.fxRatesDescription"), container.NewVBox(
			widget.NewButton(c.translator.T("portfolio.fxRates"), func() { showFXManagementDialog(c) }),
			marketDataFXSummary(c),
			mutedLabel(c.translator.T("marketData.fxLocalFirst")),
		)),
	}
	return container.NewVBox(rows...)
}

func marketDataInstrumentSummary(c *Controller, instruments []domain.Instrument) fyne.CanvasObject {
	if len(instruments) == 0 {
		return emptyPanel(c.translator.T("portfolio.noInstruments"), c.translator.T("portfolio.noInstrumentsDescription"))
	}
	items := make([]fyne.CanvasObject, 0, len(instruments))
	for _, instrument := range instruments {
		status := c.translator.T("common.active")
		if instrument.ArchivedAt != nil {
			status = c.translator.T("common.archived")
		}
		quote, quoteErr := c.service.CurrentInstrumentQuote(context.Background(), instrument.ID)
		if quoteErr != nil {
			return errorPanel(c, c.translator.T("portfolio.loadError"), quoteErr)
		}
		current := c.translator.T("portfolio.noEvidence")
		if quote != nil {
			current = fmt.Sprintf("%s %s · %s · %s", quote.UnitPrice.Canonical(), quote.Currency, quote.SourceKind, quote.QuotedAt.Format("2006-01-02"))
		}
		quotes, historyErr := c.service.InstrumentQuoteHistory(context.Background(), instrument.ID)
		if historyErr != nil {
			return errorPanel(c, c.translator.T("portfolio.loadError"), historyErr)
		}
		history := c.translator.T("portfolio.noEvidence")
		if len(quotes) > 0 {
			values := make([]string, 0, minInt(len(quotes), 5))
			for index, item := range quotes {
				if index == 5 {
					break
				}
				values = append(values, fmt.Sprintf("%s · %s %s · %s", item.QuotedAt.Format("2006-01-02"), item.UnitPrice.Canonical(), item.Currency, item.SourceKind))
			}
			history = strings.Join(values, "\n")
		}
		items = append(items, settingsSurface(container.NewVBox(
			keyValueRow(instrumentOptionLabel(instrument), status),
			keyValueRow(c.translator.T("portfolio.currentValue"), current),
			mutedLabel(history),
		), fyne.NewSize(1, 100)))
	}
	return container.NewVBox(items...)
}

func marketDataFXSummary(c *Controller) fyne.CanvasObject {
	prefs, err := c.service.ListFXPreferences(context.Background())
	if err != nil {
		return errorPanel(c, c.translator.T("portfolio.loadError"), err)
	}
	quotes, err := c.service.FXQuoteHistory(context.Background())
	if err != nil {
		return errorPanel(c, c.translator.T("portfolio.loadError"), err)
	}
	type pair struct{ a, b domain.CurrencyCode }
	pairs := make(map[string]pair)
	for _, preference := range prefs {
		pairs[fxPairKey(preference.CurrencyA, preference.CurrencyB)] = pair{preference.CurrencyA, preference.CurrencyB}
	}
	for _, quote := range quotes {
		a, b, normalizeErr := domain.NormalizeFXPair(quote.BaseCurrency, quote.QuoteCurrency)
		if normalizeErr == nil {
			pairs[fxPairKey(a, b)] = pair{a, b}
		}
	}
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return emptyPanel(c.translator.T("portfolio.noEvidence"), c.translator.T("marketData.fxLocalFirst"))
	}
	rows := make([]fyne.CanvasObject, 0, len(keys))
	for _, key := range keys {
		item := pairs[key]
		current, currentErr := c.service.CurrentFXQuote(context.Background(), item.a, item.b)
		if currentErr != nil {
			return errorPanel(c, c.translator.T("portfolio.loadError"), currentErr)
		}
		currentText := c.translator.T("portfolio.noEvidence")
		if current != nil {
			currentText = fmt.Sprintf("1 %s → %s %s · %s · %s", current.BaseCurrency, current.Rate.Canonical(), current.QuoteCurrency, current.SourceKind, current.QuotedAt.Format("2006-01-02"))
		}
		historyValues := make([]string, 0, 5)
		for _, quote := range quotes {
			a, b, normalizeErr := domain.NormalizeFXPair(quote.BaseCurrency, quote.QuoteCurrency)
			if normalizeErr != nil || a != item.a || b != item.b {
				continue
			}
			if len(historyValues) == 5 {
				break
			}
			historyValues = append(historyValues, fmt.Sprintf("%s · 1 %s = %s %s · %s", quote.QuotedAt.Format("2006-01-02"), quote.BaseCurrency, quote.Rate.Canonical(), quote.QuoteCurrency, quote.SourceKind))
		}
		history := c.translator.T("portfolio.noEvidence")
		if len(historyValues) > 0 {
			history = strings.Join(historyValues, "\n")
		}
		rows = append(rows, settingsSurface(container.NewVBox(
			keyValueRow(c.translator.T("portfolio.fxEvidence"), fmt.Sprintf("%s / %s", item.a, item.b)),
			keyValueRow(c.translator.T("portfolio.currentValue"), currentText),
			mutedLabel(history),
		), fyne.NewSize(1, 100)))
	}
	return container.NewVBox(rows...)
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
