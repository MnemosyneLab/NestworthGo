package desktop

import (
	"context"

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

type assetCategorySheet struct {
	analysisSheet
	row     application.CategoryRow
	query   domain.AnalysisQuery
	kind    application.AnalysisCategoryType
	trigger event.Tag
	cancel  context.CancelFunc
	request <-chan historyResult[application.CategoryDetailResult]
	value   *application.CategoryDetailResult
	err     error
	links   []widget.Clickable
}

func (p *assetChangesPage) categoryButton(gtx layout.Context, view ui.View, row application.CategoryRow) layout.Dimensions {
	if p.categoryButtons == nil {
		p.categoryButtons = make(map[string]*widget.Clickable)
	}
	button := portfolioButton(p.categoryButtons, row.Key)
	view.Focus.Watch(gtx, button)
	if gtx.Enabled() && p.categoryFocus == button {
		p.categoryFocus = nil
		gtx.Execute(key.FocusCmd{Tag: button})
	}
	name := p.categoryName(view.Language, row.Key, row.AccountID, row.InstrumentID)
	if button.Clicked(gtx) {
		p.categoryDetail = &assetCategorySheet{analysisSheet: newAnalysisSheet(), row: row,
			query: p.current.query, kind: p.categoryType(), trigger: button}
		p.readCategoryDetail(p.categoryDetail)
		gtx.Execute(op.InvalidateCmd{})
	}
	return view.ButtonOf(button, ui.ButtonSpec{Title: name + " · " + returnMoney(view, row.Amount), Variant: ui.ButtonOutline})(gtx)
}

func (p *assetChangesPage) categoryName(l i18n.Language, categoryKey, accountID, instrumentID string) string {
	for _, id := range []string{accountID, instrumentID, categoryKey} {
		if name := p.refs.names[id]; name != "" {
			return name
		}
	}
	if categoryKey == domain.BucketCash {
		return l.Text(i18n.AccountCash)
	}
	return l.Text(i18n.OverviewEvidenceUnavailable)
}

func (p *assetChangesPage) readCategoryDetail(sheet *assetCategorySheet) {
	ctx, cancel := context.WithCancel(p.ctx)
	sheet.cancel, sheet.err = cancel, nil
	sheet.request = runHistoryRead(&p.workers, p.invalidate, func() (application.CategoryDetailResult, error) {
		return p.reader.CategoryDetail(ctx, sheet.query, sheet.kind, sheet.row.Key)
	})
}

func (p *assetChangesPage) closeCategoryDetail(sheet *assetCategorySheet) {
	sheet.cancel()
	p.categoryDetail, p.categoryFocus = nil, sheet.trigger
}

func (p *assetChangesPage) layoutCategorySheet(gtx layout.Context, view ui.View) {
	sheet := p.categoryDetail
	if sheet.dismissed(gtx) {
		p.closeCategoryDetail(sheet)
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	select {
	case result := <-sheet.request:
		sheet.cancel()
		sheet.request, sheet.err = nil, result.err
		if result.err == nil {
			sheet.value = &result.value
			sheet.links = make([]widget.Clickable, len(result.value.ActivityRefs))
		}
	default:
	}
	if sheet.err != nil && sheet.retry.Clicked(gtx) {
		p.readCategoryDetail(sheet)
		gtx.Execute(key.FocusCmd{Tag: &sheet.close})
		gtx.Execute(op.InvalidateCmd{})
	}
	title := p.categoryName(view.Language, sheet.row.Key, sheet.row.AccountID, sheet.row.InstrumentID)
	sheet.layout(gtx, view, title, func(gtx layout.Context) layout.Dimensions {
		rows := p.categoryDetails(view, sheet)
		return view.List(&sheet.list).Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
			return (layout.Inset{Bottom: 8}).Layout(gtx, rows[index])
		})
	}, i18n.NoMessage)
}

func (p *assetChangesPage) categoryDetails(view ui.View, sheet *assetCategorySheet) []layout.Widget {
	l := view.Language
	dateRange := wealthDay(view, sheet.query.From) + " – " + wealthDay(view, sheet.query.To)
	rows := []layout.Widget{view.Message(dateRange, view.Colors.SecondaryInk)}
	if sheet.request != nil {
		return append(rows, view.Message(l.Text(i18n.AssetLoading), view.Colors.SecondaryInk))
	}
	if sheet.err != nil {
		return append(rows, view.Message(l.Text(i18n.AssetFailed), view.Colors.ErrorInk), view.Button(&sheet.retry, l.Text(i18n.HistoryRetry)))
	}

	data := sheet.value
	rows = append(rows, view.Body(ui.TextLabel, l.Text(i18n.AssetCategoryTotal)+" · "+returnMoney(view, sheet.row.Amount), view.Colors.Ink))
	if data.ValuationForced != nil {
		rows = append(rows, view.Message(l.Text(i18n.AnalysisBaseFallback), view.Colors.SecondaryInk))
	}
	if data.Status == domain.CompletenessPartial {
		rows = append(rows, view.Message(l.Text(i18n.OverviewIncomplete), view.Colors.SecondaryInk))
	}
	if len(data.Children) > 0 {
		rows = append(rows, view.Body(ui.TextLabel, l.Text(i18n.AssetCategoryComponents), view.Colors.Ink))
		for _, item := range data.Children {
			label := p.categoryName(l, item.Key, item.AccountID, item.InstrumentID)
			rows = append(rows, view.Message(label+" · "+returnMoney(view, item.Amount), view.Colors.Ink))
		}
	}
	rows = append(rows, view.Body(ui.TextLabel, l.Text(i18n.AssetCategoryRecords), view.Colors.Ink))
	if len(data.ActivityRefs) == 0 {
		return append(rows, view.Message(l.Text(i18n.AssetCategoryNoRecords), view.Colors.SecondaryInk))
	}
	for i, ref := range data.ActivityRefs {
		rows = append(rows, view.Message(wealthDay(view, ref.Date)+" · "+returnMoney(view, ref.Amount), view.Colors.Ink))
		if p.openHistory == nil {
			continue
		}
		button := &sheet.links[i]
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			if button.Clicked(gtx) {
				p.closeCategoryDetail(sheet)
				p.openHistory(history.Filters{From: ref.Date, To: ref.Date, Account: domain.AccountID(ref.AccountID),
					Instrument: domain.InstrumentID(ref.InstrumentID)}, domain.ActivityID(ref.ActivityID), sheet.trigger)
				gtx.Execute(op.InvalidateCmd{})
			}
			return view.Button(button, l.Text(i18n.ContributionOpenHistory))(gtx)
		})
	}
	return rows
}
