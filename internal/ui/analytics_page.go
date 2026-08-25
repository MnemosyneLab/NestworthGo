package ui

import (
	"context"
	"fmt"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
)

func NewAnalyticsPage(c *Controller) fyne.CanvasObject {
	t := c.translator
	if c.service == nil {
		return NewComingSoonPage(c, "page.analyticsTitle", "page.analyticsDescription", "storage")
	}
	selected := domain.TrendRange(c.trendRange)
	if selected == "" {
		selected = domain.Trend30Days
	}
	trend, err := c.service.NetWorthTrend(context.Background(), selected)
	if err != nil {
		return errorPanel(c, t.T("page.analyticsTitle"), err)
	}
	portfolio, err := c.service.Portfolio(context.Background(), domain.AccountFilter{})
	if err != nil {
		return errorPanel(c, t.T("page.analyticsTitle"), err)
	}
	gains, err := loadPortfolioGainViews(c, portfolio)
	if err != nil {
		return errorPanel(c, t.T("page.analyticsTitle"), err)
	}
	realized, err := c.service.RealizedGain(context.Background(), domain.GainScope{}, selected)
	if err != nil {
		return errorPanel(c, t.T("page.analyticsTitle"), err)
	}
	rangeSelect := widget.NewSelect([]string{t.T("analytics.range30"), t.T("analytics.range1year"), t.T("analytics.rangeAll")}, nil)
	selectedLabel := t.T("analytics.range30")
	if selected == domain.TrendOneYear {
		selectedLabel = t.T("analytics.range1year")
	} else if selected == domain.TrendAllTime {
		selectedLabel = t.T("analytics.rangeAll")
	}
	rangeSelect.SetSelected(selectedLabel)
	rows := []fyne.CanvasObject{}
	if feedback := snapshotFeedback(c); feedback != nil {
		rows = append(rows, feedback)
	}
	trendBody := container.NewVBox(analyticsTrendRows(c, trend)...)
	realizedBody := container.NewVBox(realizedGainRows(c, realized)...)
	decompositionBody := container.NewVBox(currencyDecompositionRows(c, gains)...)
	rows = append(rows,
		trendBody,
		sectionCard(t.T("analytics.realizedGain"), t.T("analytics.realizedGainDescription"), realizedBody),
		sectionCard(t.T("analytics.currencyDecomposition"), t.T("analytics.currencyDecompositionDescription"), decompositionBody),
	)
	rangeSelect.OnChanged = func(value string) {
		var next domain.TrendRange
		switch value {
		case t.T("analytics.range1year"):
			next = domain.TrendOneYear
		case t.T("analytics.rangeAll"):
			next = domain.TrendAllTime
		default:
			next = domain.Trend30Days
		}
		c.trendRange = string(next)
		updatedTrend, trendErr := c.service.NetWorthTrend(context.Background(), next)
		if trendErr != nil {
			trendBody.Objects = []fyne.CanvasObject{widget.NewLabel(c.translator.TranslateError(trendErr))}
		} else {
			trendBody.Objects = analyticsTrendRows(c, updatedTrend)
		}
		trendBody.Refresh()
		updated, updateErr := c.service.RealizedGain(context.Background(), domain.GainScope{}, next)
		if updateErr != nil {
			realizedBody.Objects = []fyne.CanvasObject{widget.NewLabel(c.translator.TranslateError(updateErr))}
		} else {
			realizedBody.Objects = realizedGainRows(c, updated)
		}
		realizedBody.Refresh()
	}
	return surface(container.NewVBox(container.NewHBox(widget.NewLabel(t.T("analytics.range")), rangeSelect), container.NewVBox(rows...)), fyne.NewSize(640, 520))
}

func analyticsTrendRows(c *Controller, trend domain.NetWorthTrend) []fyne.CanvasObject {
	t := c.translator
	rows := []fyne.CanvasObject{widget.NewLabelWithStyle(t.T("analytics.trend"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})}
	for _, point := range trend.Points {
		value := t.T("portfolio.unavailable")
		if point.Value != nil {
			value = format.Money(point.Value.CanonicalAmount(), point.Value.Currency().String(), c.preference)
		}
		status := t.T("analytics.statusPartial")
		if point.Complete {
			status = t.T("analytics.statusComplete")
		}
		rows = append(rows, widget.NewLabel(fmt.Sprintf("%s    %s    %s", point.LocalDate, value, status)))
	}
	if len(trend.Points) == 0 {
		rows = append(rows, widget.NewLabel(t.T("analytics.empty")))
	}
	return rows
}

func realizedGainRows(c *Controller, realized domain.RealizedGainView) []fyne.CanvasObject {
	t := c.translator
	rows := []fyne.CanvasObject{
		widget.NewLabel(fmt.Sprintf("%s: %s %s %s", t.T("analytics.realizedByInstrument"), realized.From, t.T("common.to"), realized.To)),
		gainGroupTable(c, t.T("analytics.byInstrument"), realized.ByInstrument),
		gainGroupTable(c, t.T("analytics.byAccount"), realized.ByAccount),
	}
	if !realized.Available && realized.MissingReason != "" {
		rows = append(rows, mutedLabel(t.T("portfolio.unavailable")))
	}
	return rows
}

func gainGroupTable(c *Controller, title string, groups []domain.GainGroupView) fyne.CanvasObject {
	t := c.translator
	rows := []fyne.CanvasObject{widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})}
	for _, group := range groups {
		value := t.T("portfolio.unavailable")
		if group.Available {
			value = format.Money(group.Gain.Amount, group.Gain.Currency.String(), c.preference)
		}
		rows = append(rows, container.NewGridWithColumns(2, widget.NewLabel(group.Label), widget.NewLabel(value)))
	}
	if len(groups) == 0 {
		rows = append(rows, mutedLabel(t.T("analytics.noGains")))
	}
	return settingsSurface(container.NewVBox(rows...), fyne.NewSize(1, 100))
}

func currencyDecompositionRows(c *Controller, gains map[domain.HoldingID]domain.HoldingGainView) []fyne.CanvasObject {
	t := c.translator
	ids := make([]domain.HoldingID, 0, len(gains))
	for id := range gains {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	rows := []fyne.CanvasObject{
		container.NewGridWithColumns(4,
			widget.NewLabelWithStyle(t.T("portfolio.position"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle(t.T("analytics.totalGain"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle(t.T("analytics.instrumentMovement"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle(t.T("analytics.currencyMovement"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		),
	}
	for _, id := range ids {
		gain := gains[id]
		total, instrument, currency := t.T("portfolio.unavailable"), t.T("portfolio.unavailable"), t.T("portfolio.unavailable")
		if gain.UnrealizedGainBase != nil {
			total = format.Money(gain.UnrealizedGainBase.Amount, gain.UnrealizedGainBase.Currency.String(), c.preference)
		}
		if gain.InstrumentMovement != nil {
			instrument = format.Money(gain.InstrumentMovement.Amount, gain.InstrumentMovement.Currency.String(), c.preference)
		}
		if gain.CurrencyMovement != nil {
			currency = format.Money(gain.CurrencyMovement.Amount, gain.CurrencyMovement.Currency.String(), c.preference)
		}
		rows = append(rows, container.NewGridWithColumns(4,
			widget.NewLabel(instrumentIdentityLabel(c, gain.InstrumentName, gain.InstrumentSymbol)),
			widget.NewLabel(total), widget.NewLabel(instrument), widget.NewLabel(currency),
		))
	}
	if len(ids) == 0 {
		rows = append(rows, mutedLabel(t.T("analytics.noPositions")))
	}
	return rows
}
