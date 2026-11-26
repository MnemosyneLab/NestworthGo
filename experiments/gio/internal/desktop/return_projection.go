package desktop

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
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
	ranking                    contributionRanking
	rankButtons                map[string]*widget.Clickable
	selectedKey                string
	trendChart                 charts.LineChart
	trendTable                 chartDataTable
	trendSelection             charts.Selection
	resetTrend                 widget.Clickable
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
	if p.group == "" || p.contributionMetric() == application.ContributionRealized &&
		(p.group == application.ContributionGroupCurrency || p.group == application.ContributionGroupAssetClass) {
		return application.ContributionGroupInstrument
	}
	return p.group
}

func (p *returnProjectionControls) contributionSort() application.ContributionSort {
	if p.sort == "" || p.contributionMetric() != application.ContributionTotalReturn &&
		(p.sort == application.ContributionSortRateAsc || p.sort == application.ContributionSortRateDesc) {
		return application.ContributionSortAmountDesc
	}
	return p.sort
}

func (p *returnAnalysisPage) changeProjection() {
	p.current = nil
	p.output.list.Position = layout.Position{}
	p.output.ranking.list.Position = layout.Position{}
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
				rows = append(rows, p.trendRows(gtx, view, *p.current.trend, p.current.summaryFrom, p.current.summaryTo)...)
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
		}
		if out.contributionMetric() != application.ContributionRealized {
			groups = append(groups,
				ui.Choice{ID: string(application.ContributionGroupCurrency), Label: l.Text(i18n.AnalysisCurrency)},
				ui.Choice{ID: string(application.ContributionGroupAssetClass), Label: l.Text(i18n.AnalysisAssetClass)})
		}
		sorts := []ui.Choice{
			{ID: string(application.ContributionSortAmountDesc), Label: l.Text(i18n.ReturnSortLargest)},
			{ID: string(application.ContributionSortAmountAsc), Label: l.Text(i18n.ReturnSortSmallest)},
			{ID: string(application.ContributionSortNameAsc), Label: l.Text(i18n.ReturnSortName)},
		}
		if out.contributionMetric() == application.ContributionTotalReturn {
			sorts = append(sorts,
				ui.Choice{ID: string(application.ContributionSortRateDesc), Label: l.Text(i18n.RankingRateDesc)},
				ui.Choice{ID: string(application.ContributionSortRateAsc), Label: l.Text(i18n.RankingRateAsc)})
		}
		fields = append(fields,
			analysisSelect(&out.kind, i18n.ReturnMetric, metrics, string(out.contributionMetric()), false, func(value string) {
				out.metric = application.ContributionReturnType(value)
				out.group, out.sort = out.contributionGroup(), out.contributionSort()
				out.selectedKey = ""
				p.changeProjection()
			}),
			analysisSelect(&out.groupBy, i18n.ReturnGroupBy, groups, string(out.contributionGroup()), false, func(value string) {
				out.group = application.ContributionGroupBy(value)
				out.selectedKey = ""
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

func (p *returnAnalysisPage) trendRows(gtx layout.Context, view ui.View, trend application.ReturnTrendResult, from, to domain.LocalDate) []ui.ColumnChild {
	l := view.Language
	amount := returnMoney(view, trend.Amount)
	if trend.Amount != nil && trend.Status == domain.CompletenessPartial {
		amount += " · " + l.Text(i18n.OverviewKnownSubtotal)
	}
	rows := []ui.ColumnChild{ui.Rigid(view.Message(l.Text(i18n.ReturnAmount)+" · "+amount, view.Colors.Ink)),
		ui.Rigid(view.Message(l.Text(i18n.ReturnRate)+" · "+returnRate(view, trend.Rate), view.Colors.Ink)),
		ui.Rigid(view.Message(returnCoverage(l, trend.Coverage, trend.ReturnDayCoverage), view.Colors.SecondaryInk))}
	if !trend.Available {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.ReturnInsufficient), view.Colors.SecondaryInk)))
	}
	if len(trend.Points) != 0 {
		spec := returnTrendLine(view, trend, from, to)
		rows = append(rows, ui.Rigid(analysisLineCard(view, &p.output.trendChart, spec, &p.output.trendSelection, &p.output.trendTable, chartTableLabels(l, i18n.ChartCoverage))))
	}
	if len(trend.Sources) != 0 {
		rows = append(rows, ui.Rigid(view.Body(ui.TextLabel, l.Text(i18n.ReturnComposition), view.Colors.Ink)))
		for _, source := range trend.Sources {
			label := returnComponentLabel(l, domain.ReturnComponent(source.Key))
			rows = append(rows, ui.Rigid(view.Message(label+" · "+returnMoney(view, source.Amount), view.Colors.Ink)))
		}
	}
	if p.output.trendDisplay() != application.ReturnTrendCumulativeAmount {
		if p.output.resetTrend.Clicked(gtx) {
			p.output.display = application.ReturnTrendCumulativeAmount
			p.changeProjection()
			gtx.Execute(key.FocusCmd{Tag: &p.output.mode.Trigger})
			gtx.Execute(op.InvalidateCmd{})
		}
		rows = append(rows, ui.Rigid(returnMonthButton(view, &p.output.resetTrend, i18n.ReturnTrendReset, nil, true)))
	}
	return rows
}

func analysisResultList(gtx layout.Context, view ui.View, list *widget.List, title string, count int, row layout.ListElement) layout.Dimensions {
	list.Axis = layout.Vertical
	border := view.Colors.Divider
	if gtx.Focused(list) {
		border = view.Colors.Accent
	}
	return view.Panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
		return keyboardList(gtx, view, list, title, count, func(gtx layout.Context, i int) layout.Dimensions {
			// A hairline above every row but the first turns the list into a
			// table without drawing a box per row.
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if i == 0 {
						return layout.Dimensions{}
					}
					return view.Hairline(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 8, Bottom: 8, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return row(gtx, i)
					})
				}),
			)
		})
	})
}

// pageList is the scrolling body of a page whose rows are cards and sections.
// Unlike analysisResultList it draws no panel of its own: the rows sit on the
// page background, and the list extends into the page's right gutter so its
// scrollbar rests against the window edge, as the Wails page scrollbar does.
// Keyboard focus is shown as an accent outline.
func pageList(gtx layout.Context, view ui.View, list *widget.List, title string, count int, row layout.ListElement) layout.Dimensions {
	return pageListGap(gtx, view, list, title, count, ui.Space16, row)
}

// pageListGap is pageList with a caller-chosen space under each row, for lists
// whose rows join into one card.
func pageListGap(gtx layout.Context, view ui.View, list *widget.List, title string, count int, gap unit.Dp, row layout.ListElement) layout.Dimensions {
	list.Axis = layout.Vertical
	bleed := gtx.Dp(pageInset.Right)
	gtx.Constraints.Max.X += bleed
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	dims := keyboardList(gtx, view, list, title, count, func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Right: pageInset.Right, Bottom: gap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return row(gtx, i)
		})
	})
	if gtx.Focused(list) {
		ui.StrokeRoundRect(gtx, image.Rectangle{Max: dims.Size}, ui.RadiusXL, 2, ui.WithAlpha(view.Colors.Accent, 0.5))
	}
	return dims
}

// keyboardList makes a list a single tab stop that scrolls with the arrow,
// page, Home and End keys.
func keyboardList(gtx layout.Context, view ui.View, list *widget.List, title string, count int, row layout.ListElement) layout.Dimensions {
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
		return view.List(list).Layout(gtx, count, row)
	})
}
