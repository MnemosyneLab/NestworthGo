package desktop

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type returnProjectionControls struct {
	display application.ReturnTrendDisplay
	metric  application.ContributionReturnType
	group   application.ContributionGroupBy
	sort    application.ContributionSort

	mode, kind, groupBy, order ui.Select
	list                       widget.List
}

func (p *returnProjectionControls) trendDisplay() application.ReturnTrendDisplay {
	if p.display == "" {
		return application.ReturnTrendCumulativeAmount
	}
	return p.display
}

func (p *returnProjectionControls) contributionMetric() application.ContributionReturnType {
	if p.metric == "" {
		return application.ContributionTotalReturn
	}
	return p.metric
}

func (p *returnProjectionControls) contributionGroup() application.ContributionGroupBy {
	if p.group == "" {
		return application.ContributionGroupInstrument
	}
	return p.group
}

func (p *returnProjectionControls) contributionSort() application.ContributionSort {
	if p.sort == "" {
		return application.ContributionSortAmountDesc
	}
	return p.sort
}

func (p *returnAnalysisPage) changeProjection() {
	p.current = nil
	p.output.list.Position = layout.Position{}
	p.readMonth(p.month, p.sunday)
}

func (p *returnAnalysisPage) layoutProjection(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	gtx.Constraints.Min.Y = 0
	width := gtx.Constraints.Max.X
	options := p.projectionOptions(gtx, view)
	offset := options.Size.Y + gtx.Dp(10)
	if p.reload.Clicked(gtx) && p.request == nil {
		p.readMonth(p.month, p.sunday)
		gtx.Execute(op.InvalidateCmd{})
	}
	body := ui.Place(gtx, view, image.Pt(0, offset), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
		label := i18n.HistoryRefresh
		if p.err != nil {
			label = i18n.HistoryRetry
		}
		status := i18n.NoMessage
		switch {
		case !p.filters.selection.ready():
			status = i18n.AnalysisScopeRequired
		case p.request != nil:
			status = i18n.ReturnLoading
		case p.err != nil:
			status = i18n.ReturnFailed
		case p.current == nil || p.current.summaryFrom > p.current.summaryTo:
			status = i18n.ReturnNoClosedDays
		}
		rows := []ui.ColumnChild{ui.Rigid(returnMonthButton(view, &p.reload, label, nil, true))}
		if status != i18n.NoMessage {
			rows = append(rows, ui.Rigid(view.Message(l.Text(status), view.Colors.SecondaryInk)))
		}
		if p.current != nil {
			if p.current.trend != nil {
				rows = append(rows, p.trendRows(view, *p.current.trend)...)
			}
			if p.current.contribution != nil {
				rows = append(rows, p.contributionRows(view, *p.current.contribution)...)
			}
		}
		return (ui.Column{Gap: 10, MinFill: 310}).Layout(gtx, rows...)
	})
	return layout.Dimensions{Size: image.Pt(width, offset+body.Size.Y)}
}

func (p *returnAnalysisPage) projectionOptions(gtx layout.Context, view ui.View) layout.Dimensions {
	out, l := &p.output, view.Language
	var fields []analysisFilterField
	if p.tab == returnTrendTab {
		modes := []ui.Choice{
			{ID: string(application.ReturnTrendCumulativeAmount), Label: l.Text(i18n.ReturnCumulative)},
			{ID: string(application.ReturnTrendLinkedRate), Label: l.Text(i18n.ReturnLinked)},
			{ID: string(application.ReturnTrendPeriodReturnAmount), Label: l.Text(i18n.ReturnPeriodAmount)},
		}
		fields = append(fields, analysisSelect(&out.mode, i18n.ReturnDisplay, modes, string(out.trendDisplay()), false, func(value string) {
			out.display = application.ReturnTrendDisplay(value)
			p.changeProjection()
		}))
	} else {
		metrics := []ui.Choice{
			{ID: string(application.ContributionTotalReturn), Label: l.Text(i18n.ReturnTotal)},
			{ID: string(application.ContributionRealized), Label: l.Text(i18n.ReturnRealized)},
			{ID: string(application.ContributionUnrealized), Label: l.Text(i18n.ReturnUnrealized)},
			{ID: string(application.ContributionDividendInterest), Label: l.Text(i18n.ReturnIncome)},
		}
		groups := []ui.Choice{
			{ID: string(application.ContributionGroupInstrument), Label: l.Text(i18n.AnalysisInstrument)},
			{ID: string(application.ContributionGroupAccount), Label: l.Text(i18n.AnalysisAccount)},
			{ID: string(application.ContributionGroupCurrency), Label: l.Text(i18n.AnalysisCurrency)},
		}
		sorts := []ui.Choice{
			{ID: string(application.ContributionSortAmountDesc), Label: l.Text(i18n.ReturnSortLargest)},
			{ID: string(application.ContributionSortAmountAsc), Label: l.Text(i18n.ReturnSortSmallest)},
			{ID: string(application.ContributionSortNameAsc), Label: l.Text(i18n.ReturnSortName)},
		}
		fields = append(fields,
			analysisSelect(&out.kind, i18n.ReturnMetric, metrics, string(out.contributionMetric()), false, func(value string) {
				out.metric = application.ContributionReturnType(value)
				p.changeProjection()
			}),
			analysisSelect(&out.groupBy, i18n.ReturnGroupBy, groups, string(out.contributionGroup()), false, func(value string) {
				out.group = application.ContributionGroupBy(value)
				p.changeProjection()
			}),
			analysisSelect(&out.order, i18n.ReturnSort, sorts, string(out.contributionSort()), false, func(value string) {
				out.sort = application.ContributionSort(value)
				p.changeProjection()
			}))
	}
	return (ui.Grid{MinColumn: 165, MaxColumns: len(fields), Gap: 10, Balanced: true}).Layout(gtx, view, len(fields),
		func(gtx layout.Context, view ui.View, i int) layout.Dimensions { return fields[i](gtx, view) })
}

func (p *returnAnalysisPage) trendRows(view ui.View, trend application.ReturnTrendResult) []ui.ColumnChild {
	l := view.Language
	rows := []ui.ColumnChild{ui.Rigid(view.Message(l.Text(i18n.ReturnAmount)+" · "+returnMoney(view, trend.Amount), view.Colors.Ink)),
		ui.Rigid(view.Message(l.Text(i18n.ReturnRate)+" · "+returnRate(view, trend.Rate), view.Colors.Ink)),
		ui.Rigid(view.Message(returnCoverage(l, trend.Coverage, trend.ReturnDayCoverage), view.Colors.SecondaryInk))}
	if !trend.Available {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.ReturnInsufficient), view.Colors.SecondaryInk)))
	}
	if len(trend.Sources) != 0 {
		rows = append(rows, ui.Rigid(view.Body(ui.TextLabel, l.Text(i18n.ReturnComposition), view.Colors.Ink)))
		for _, source := range trend.Sources {
			label := returnComponentLabel(l, domain.ReturnComponent(source.Key))
			rows = append(rows, ui.Rigid(view.Message(label+" · "+returnMoney(view, source.Amount), view.Colors.Ink)))
		}
	}
	if len(trend.Points) != 0 {
		rows = append(rows, ui.Fill(func(gtx layout.Context) layout.Dimensions {
			return p.projectionList(gtx, view, l.Text(i18n.ReturnTrend), len(trend.Points), func(gtx layout.Context, i int) layout.Dimensions {
				point := trend.Points[i]
				value := returnMoney(view, point.Value)
				if trend.Display == application.ReturnTrendLinkedRate {
					value = returnRate(view, point.Rate)
				}
				text := wealthDay(view, point.Date) + " · " + value + " · " + returnCoverage(l, point.Coverage, application.ReturnDayCoverage{})
				return view.Message(text, view.Colors.Ink)(gtx)
			})
		}))
	}
	return rows
}

func (p *returnAnalysisPage) contributionRows(view ui.View, data application.ContributionResult) []ui.ColumnChild {
	l := view.Language
	summary := data.Summary
	rows := []ui.ColumnChild{ui.Rigid(view.Message(l.Text(i18n.ReturnAmount)+" · "+returnAmountText(view, summary.Total, summary.AmountStatus), view.Colors.Ink))}
	if !data.Available || len(data.Rows) == 0 {
		return append(rows, ui.Rigid(view.Message(l.Text(i18n.ReturnInsufficient), view.Colors.SecondaryInk)))
	}
	return append(rows, ui.Fill(func(gtx layout.Context) layout.Dimensions {
		return p.projectionList(gtx, view, l.Text(i18n.ReturnContribution), len(data.Rows), func(gtx layout.Context, i int) layout.Dimensions {
			row := data.Rows[i]
			name := p.refs.names[row.Key]
			kind, currency, _ := strings.Cut(row.Key, ":")
			switch {
			case data.GroupBy == application.ContributionGroupCurrency:
				name = row.Key
			case kind == domain.BucketCash:
				name = l.Text(i18n.AccountCash)
				if currency != "" {
					name += " · " + currency
				}
			case name == "":
				name = l.Text(i18n.OverviewEvidenceUnavailable)
			}
			value := fmt.Sprintf("%s · %s · %s", name, returnMoney(view, row.Amount), returnRate(view, row.Rate))
			if row.Status == domain.CompletenessPartial {
				value += " · " + l.Text(i18n.ReturnInsufficient)
			}
			return view.Message(value, view.Colors.Ink)(gtx)
		})
	}))
}

func (p *returnAnalysisPage) projectionList(gtx layout.Context, view ui.View, title string, count int, row layout.ListElement) layout.Dimensions {
	list := &p.output.list
	list.Axis = layout.Vertical
	border := view.Colors.Divider
	if gtx.Focused(list) {
		border = view.Colors.Accent
	}
	return view.Panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
		view.Focus.Watch(gtx, list)
		event.Op(gtx.Ops, list)
		for _, press := range ui.PressedKeys(gtx, list, key.NameUpArrow, key.NameDownArrow, key.NamePageUp, key.NamePageDown, key.NameHome, key.NameEnd) {
			switch press.Name {
			case key.NameUpArrow:
				list.ScrollBy(-1)
			case key.NameDownArrow:
				list.ScrollBy(1)
			case key.NamePageUp:
				list.ScrollBy(-float32(max(1, list.Position.Count-1)))
			case key.NamePageDown:
				list.ScrollBy(float32(max(1, list.Position.Count-1)))
			case key.NameHome:
				list.ScrollTo(0)
			case key.NameEnd:
				list.ScrollTo(count - 1)
			}
		}
		return accessibility.List(gtx, list, title, func(gtx layout.Context) layout.Dimensions {
			return view.List(list).Layout(gtx, count, func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Top: 5, Bottom: 5, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return row(gtx, i)
				})
			})
		})
	})
}
