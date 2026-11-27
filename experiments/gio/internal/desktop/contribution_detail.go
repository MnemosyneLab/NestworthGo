package desktop

import (
	"context"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type contributionSheet struct {
	analysisSheet
	query   domain.AnalysisQuery
	group   application.ContributionGroupBy
	metric  application.ContributionReturnType
	key     string
	name    string
	trigger event.Tag
	request <-chan historyResult[application.ContributionItemResult]
	cancel  context.CancelFunc
	value   *application.ContributionItemResult
	err     error
}

func (p *returnAnalysisPage) openContribution(rowKey, name string, trigger event.Tag) {
	result := p.current
	p.item = &contributionSheet{
		analysisSheet: newAnalysisSheet(), query: result.query,
		group: result.contribution.GroupBy, metric: result.contribution.ReturnType,
		key: rowKey, name: name, trigger: trigger,
	}
	p.output.selectedKey = rowKey
	p.readContribution()
}

func (p *returnAnalysisPage) readContribution() {
	sheet := p.item
	ctx, cancel := context.WithCancel(p.ctx)
	sheet.cancel, sheet.err = cancel, nil
	query, metric, group, id := sheet.query, sheet.metric, sheet.group, sheet.key
	sheet.request = runHistoryRead(&p.workers, p.invalidate, func() (application.ContributionItemResult, error) {
		return p.reader.ContributionItem(ctx, query, metric, group, id)
	})
}

func (p *returnAnalysisPage) closeContribution() {
	p.item.cancel()
	p.returnFocus = p.item.trigger
	p.item = nil
}

func (p *returnAnalysisPage) layoutContributionSheet(gtx layout.Context, view ui.View) {
	sheet := p.item
	if sheet.dismissed(gtx) {
		p.closeContribution()
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
		p.readContribution()
		gtx.Execute(key.FocusCmd{Tag: &sheet.close})
		gtx.Execute(op.InvalidateCmd{})
	}
	var footer layout.Widget
	if p.openHistory != nil && sheet.value != nil && sheet.request == nil && sheet.err == nil {
		if filters, ok := contributionHistoryFilters(sheet.value.HistoryHint); ok {
			if sheet.action.Clicked(gtx) {
				p.closeContribution()
				p.openHistory(filters, sheet.trigger)
				gtx.Execute(op.InvalidateCmd{})
				return
			}
			footer = contributionLedgerFooter(view, sheet, filters)
		}
	}
	sheet.layoutPanel(gtx, view, sheet.name, func(gtx layout.Context, view ui.View) layout.Dimensions {
		return p.contributionDetails(gtx, view, sheet)
	}, footer, true)
}

func contributionLedgerFooter(view ui.View, sheet *contributionSheet, filters history.Filters) layout.Widget {
	l := view.Language
	var rows []ui.ColumnChild
	if sheet.query.Filters.Currency != nil || sheet.query.Filters.AssetClass != "" ||
		sheet.group == application.ContributionGroupCurrency || sheet.group == application.ContributionGroupAssetClass {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.ContributionHistoryWider), view.Colors.SecondaryInk)))
	}
	if sheet.query.Filters.MemberID != nil {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.ContributionHistoryMember), view.Colors.SecondaryInk)))
	}
	if filters.Account == "" && filters.Instrument == "" && len(sheet.value.ByAccount) > 1 {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.ContributionHistoryFullRange), view.Colors.SecondaryInk)))
	}
	rows = append(rows, ui.Rigid(view.Button(&sheet.action, l.Text(i18n.ContributionOpenHistory))))
	return func(gtx layout.Context) layout.Dimensions { return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...) }
}

func contributionHistoryFilters(hint application.ContributionHistoryHint) (history.Filters, bool) {
	filters := history.Filters{From: hint.From, To: hint.To}
	if _, ok := ui.ParseCivilDay(hint.From); !ok {
		return history.Filters{}, false
	}
	if _, ok := ui.ParseCivilDay(hint.To); !ok || hint.To < hint.From {
		return history.Filters{}, false
	}
	if hint.AccountID != "" {
		id, err := domain.ParseAccountID(hint.AccountID)
		if err != nil {
			return history.Filters{}, false
		}
		filters.Account = id
	}
	if hint.InstrumentID != "" {
		id, err := domain.ParseInstrumentID(hint.InstrumentID)
		if err != nil {
			return history.Filters{}, false
		}
		filters.Instrument = id
	}
	for _, value := range hint.Kinds {
		kind, err := domain.ParseActivityKind(value)
		if err != nil {
			return history.Filters{}, false
		}
		filters.Kinds = append(filters.Kinds, kind)
	}
	if len(filters.Kinds) == 1 {
		filters.Kind, filters.Kinds = filters.Kinds[0], nil
	}
	return filters, true
}

func (p *returnAnalysisPage) contributionDetails(gtx layout.Context, view ui.View, sheet *contributionSheet) layout.Dimensions {
	l := view.Language
	switch {
	case sheet.request != nil:
		return view.Message(l.Text(i18n.ReturnLoading), view.Colors.SecondaryInk)(gtx)
	case sheet.err != nil:
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(10)}.Layout(gtx,
			layout.Rigid(view.Message(l.Text(i18n.ReturnFailed), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&sheet.retry, l.Text(i18n.HistoryRetry))))
	case sheet.value == nil:
		return view.Message(l.Text(i18n.ReturnNoData), view.Colors.SecondaryInk)(gtx)
	}
	value := sheet.value
	rows := []layout.Widget{view.Message(l.Text(i18n.ReturnContribution)+" · "+returnSignedMoney(view, value.Amount), view.Colors.Ink)}
	if sheet.metric == application.ContributionTotalReturn || (sheet.metric == application.ContributionUnrealized && value.Rate != nil) {
		rows = append(rows, view.Message(l.Text(i18n.ReturnRate)+" · "+returnRate(view, value.Rate), view.Colors.Ink))
		if value.Coverage.TotalDays > 0 {
			rows = append(rows, view.Message(returnCoverage(l, value.Coverage, application.ReturnDayCoverage{}), view.Colors.SecondaryInk))
		}
	}
	if value.ValuationForced != nil {
		rows = append(rows, view.Message(l.Text(i18n.AnalysisBaseFallback), view.Colors.WarningInk))
	}
	if value.Status == domain.CompletenessPartial || value.Status == domain.CompletenessUnavailable {
		rows = append(rows, view.Message(l.Text(contributionAvailabilityCopy(value.AnalysisAvailability)), view.Colors.WarningInk))
	}
	rows = append(rows, view.Body(ui.TextCaption, l.Text(i18n.RankingIndependent), view.Colors.SecondaryInk))
	if len(value.Components) > 0 {
		rows = append(rows, view.Body(ui.TextLabel, l.Text(i18n.ReturnComposition), view.Colors.Ink))
		for _, part := range value.Components {
			name := p.refs.names[part.InstrumentID]
			if name == "" {
				name = returnComponentLabel(l, domain.ReturnComponent(part.Key))
			}
			amount := returnSignedMoney(view, part.Amount)
			rows = append(rows, labeledRow(view, name+" · "+amount, name, amount, view.Colors.Ink))
		}
	}
	if len(value.ByAccount) > 0 {
		rows = append(rows, view.Body(ui.TextLabel, l.Text(i18n.RankingByAccount), view.Colors.Ink))
		for _, part := range value.ByAccount {
			name := p.refs.names[part.AccountID]
			if name == "" {
				name = l.Text(i18n.AccountReferenceUnavailable)
			}
			amount := returnSignedMoney(view, part.Amount)
			rows = append(rows, labeledRow(view, name+" · "+amount, name, amount, view.Colors.Ink))
		}
	}
	if _, valid := contributionHistoryFilters(value.HistoryHint); !valid {
		rows = append(rows, view.Message(l.Text(i18n.ContributionHistoryUnavailable), view.Colors.SecondaryInk))
	}
	return analysisResultList(gtx, view, &sheet.list, l.Text(i18n.ReturnContribution), len(rows), func(gtx layout.Context, index int) layout.Dimensions {
		return rows[index](gtx)
	})
}

func contributionAvailabilityCopy(value application.AnalysisAvailability) i18n.MessageID {
	switch value.MissingReason {
	case "one or more return days are incomplete":
		return i18n.ContributionDaysIncomplete
	case "return days are unavailable":
		return i18n.ContributionDaysUnavailable
	case "funding cash is recorded after the product start":
		return i18n.WealthHistoryFundingGap
	case "realized gain foreign-exchange rate is unavailable":
		return i18n.ContributionRealizedFXUnavailable
	}
	if value.Status == domain.CompletenessUnavailable {
		return i18n.ReturnNoData
	}
	return i18n.OverviewIncomplete
}
