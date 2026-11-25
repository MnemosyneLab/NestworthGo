package desktop

import (
	"context"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type assetDriverSheet struct {
	analysisSheet
	row     application.AssetChangeRow
	query   domain.AnalysisQuery
	trigger event.Tag
	request <-chan historyResult[application.AssetDriverDetailResult]
	cancel  context.CancelFunc
	value   *application.AssetDriverDetailResult
	err     error
	links   map[string]*widget.Clickable
}

func (p *assetChangesPage) restoreDriverFocus(gtx layout.Context, view ui.View, tag event.Tag) {
	view.Focus.Watch(gtx, tag)
	if gtx.Enabled() && p.driverFocus == tag {
		p.driverFocus = nil
		gtx.Execute(key.FocusCmd{Tag: tag})
	}
}

func (p *assetChangesPage) openDriver(row application.AssetChangeRow, trigger event.Tag) {
	if p.driver != nil {
		p.driver.cancel()
	}
	p.driver = &assetDriverSheet{analysisSheet: newAnalysisSheet(), row: row, query: p.current.query,
		trigger: trigger, links: make(map[string]*widget.Clickable)}
	p.readDriver()
}

func (p *assetChangesPage) readDriver() {
	sheet := p.driver
	ctx, cancel := context.WithCancel(p.ctx)
	sheet.cancel, sheet.err = cancel, nil
	sheet.request = runHistoryRead(&p.workers, p.invalidate, func() (application.AssetDriverDetailResult, error) {
		return p.reader.AssetDriverDetail(ctx, sheet.query, string(sheet.row.Bucket))
	})
}

func (p *assetChangesPage) closeDriver() {
	p.driver.cancel()
	p.driverFocus, p.driver = p.driver.trigger, nil
}

func (p *assetChangesPage) layoutDriverSheet(gtx layout.Context, view ui.View) {
	sheet := p.driver
	if sheet.dismissed(gtx) {
		p.closeDriver()
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	select {
	case result := <-sheet.request:
		sheet.cancel()
		sheet.request, sheet.err = nil, result.err
		if result.err == nil {
			sheet.value = &result.value
		}
	default:
	}
	if sheet.err != nil && sheet.retry.Clicked(gtx) {
		p.readDriver()
		gtx.Execute(key.FocusCmd{Tag: &sheet.close})
		gtx.Execute(op.InvalidateCmd{})
	}
	action := i18n.NoMessage
	if kind := returnMetricForDriver(sheet.row.Bucket); p.openReturns != nil && sheet.value != nil && kind != "" {
		action = i18n.AssetOpenReturn
		if sheet.action.Clicked(gtx) {
			selection := p.filters.selection
			selection.from, selection.to = sheet.query.From, sheet.query.To
			p.closeDriver()
			p.openReturns(selection, p.filters.expanded, kind, sheet.trigger)
			gtx.Execute(op.InvalidateCmd{})
			return
		}
	}
	sheet.layout(gtx, view, assetBucketLabel(view.Language, sheet.row.Bucket), func(gtx layout.Context) layout.Dimensions {
		return p.driverDetails(gtx, view, sheet)
	}, action)
}

func returnMetricForDriver(bucket domain.AttributionBucket) application.ContributionReturnType {
	switch bucket {
	case domain.BucketDividendInterest:
		return application.ContributionDividendInterest
	case domain.BucketPriceChange, domain.BucketFXImpact, domain.BucketFXConversionSpread, domain.BucketFee:
		return application.ContributionTotalReturn
	default:
		return ""
	}
}

func (p *assetChangesPage) driverDetails(gtx layout.Context, view ui.View, sheet *assetDriverSheet) layout.Dimensions {
	l := view.Language
	dateRange := wealthDay(view, sheet.query.From) + " – " + wealthDay(view, sheet.query.To)
	rows := []layout.Widget{view.Message(dateRange, view.Colors.SecondaryInk)}
	switch {
	case sheet.request != nil:
		rows = append(rows, view.Message(l.Text(i18n.AssetLoading), view.Colors.SecondaryInk))
	case sheet.err != nil:
		rows = append(rows, view.Message(l.Text(i18n.AssetFailed), view.Colors.ErrorInk), view.Button(&sheet.retry, l.Text(i18n.HistoryRetry)))
	case sheet.value != nil:
		result := sheet.value
		rows = append(rows, view.Body(ui.TextLabel, l.Text(i18n.AssetDriverTotal)+" · "+returnMoney(view, sheet.row.Amount), view.Colors.Ink))
		if result.ValuationForced != nil {
			rows = append(rows, view.Message(l.Text(i18n.AnalysisBaseFallback), view.Colors.SecondaryInk))
		}
		if result.Status == domain.CompletenessPartial {
			rows = append(rows, view.Message(l.Text(i18n.OverviewIncomplete), view.Colors.SecondaryInk))
		}
		if len(result.ResidualDetails) > 0 {
			rows = append(rows, view.Body(ui.TextLabel, l.Text(i18n.AssetDriverComponentDays), view.Colors.Ink))
			for _, detail := range result.ResidualDetails {
				rows = append(rows, p.driverResidual(view, sheet, detail))
			}
		}
		for _, dimension := range []struct {
			label i18n.MessageID
			rows  []application.AnalysisDimensionAmount
		}{
			{i18n.AssetDriverByInstrument, result.ByInstrument}, {i18n.AssetDriverByAccount, result.ByAccount},
		} {
			if len(dimension.rows) == 0 {
				continue
			}
			rows = append(rows, view.Body(ui.TextLabel, l.Text(dimension.label), view.Colors.Ink))
			for _, value := range dimension.rows {
				name := p.driverDimension(l, value)
				rows = append(rows, view.Message(name+" · "+returnMoney(view, value.Amount), view.Colors.Ink))
			}
		}
		if len(result.ResidualDetails)+len(result.ByInstrument)+len(result.ByAccount) == 0 {
			rows = append(rows, view.Message(l.Text(i18n.AssetNoDriverDetail), view.Colors.SecondaryInk))
		}
	}
	return view.List(&sheet.list).Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
		return (layout.Inset{Bottom: 8}).Layout(gtx, rows[index])
	})
}

func (p *assetChangesPage) driverDimension(l i18n.Language, row application.AnalysisDimensionAmount) string {
	for _, id := range []string{row.AccountID, row.InstrumentID} {
		if name := p.refs.names[id]; name != "" {
			return name
		}
	}
	if kind, currency, _ := strings.Cut(row.Key, ":"); kind == domain.BucketCash {
		name := l.Text(i18n.AccountCash)
		if currency != "" {
			name += " · " + currency
		}
		return name
	}
	if row.Label != "" && row.Label != row.Key && row.Label != row.InstrumentID && row.Label != row.AccountID {
		return row.Label
	}
	return l.Text(i18n.OverviewEvidenceUnavailable)
}

func (p *assetChangesPage) driverResidual(view ui.View, sheet *assetDriverSheet, value application.AssetResidualDetail) layout.Widget {
	l := view.Language
	var names []string
	for _, id := range []string{value.AccountID, value.InstrumentID} {
		if name := p.refs.names[id]; name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		names = append(names, l.Text(i18n.OverviewEvidenceUnavailable))
	}
	row := strings.Join(append(names, wealthDay(view, value.Date), returnMoney(view, value.Amount)), " · ")
	return func(gtx layout.Context) layout.Dimensions {
		items := []ui.ColumnChild{ui.Rigid(view.Message(row, view.Colors.Ink))}
		if p.openHistory != nil {
			button := portfolioButton(sheet.links, value.Date+"/"+value.ComponentKey)
			if button.Clicked(gtx) {
				p.closeDriver()
				p.openHistory(history.Filters{From: value.Date, To: value.Date, Account: domain.AccountID(value.AccountID), Instrument: domain.InstrumentID(value.InstrumentID)}, "", sheet.trigger)
				gtx.Execute(op.InvalidateCmd{})
			}
			items = append(items, ui.Rigid(view.Button(button, l.Text(i18n.ContributionOpenHistory))))
		}
		return (ui.Column{Gap: 4}).Layout(gtx, items...)
	}
}
