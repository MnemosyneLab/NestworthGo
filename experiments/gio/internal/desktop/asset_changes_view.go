package desktop

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

var assetTabs = [...]i18n.MessageID{i18n.AssetTrend, i18n.AssetDrivers, i18n.AssetCategories}

func (p *assetChangesPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.update()
	l := view.Language
	for i := range p.tabs {
		if p.tabs[i].Clicked(gtx) && p.state.tab != assetChangesTab(i) {
			p.state.tab = assetChangesTab(i)
			p.change()
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	if p.reload.Clicked(gtx) && p.refsRead == nil && p.request == nil {
		if p.refs.origin == nil {
			p.readRefs()
		} else {
			p.read()
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	gtx.Constraints.Min.Y = 0
	width, gap := gtx.Constraints.Max.X, gtx.Dp(12)
	tabs := view.TabTrack(gtx, len(assetTabs), func(gtx layout.Context, i int) layout.Dimensions {
		return view.TabSegment(&p.tabs[i], l.Text(assetTabs[i]), p.state.tab == assetChangesTab(i))(gtx)
	})
	offset := tabs.Size.Y + gap
	if p.refs.origin != nil {
		filters := ui.Place(gtx, view, image.Pt(0, offset), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
			now := p.clock()
			month := returnMonth(now.In(p.refs.origin.location))
			effective := returnCalendarQuery(month, month.AddDate(0, 1, -1), p.refs.origin, now)
			return p.filters.layout(gtx, view, p.refs.analysisReferences, p.refs.origin, now, effective, func(analysisSelection, analysisSelection) {
				p.change()
			})
		})
		offset += filters.Size.Y + gap
		if p.state.tab == assetCategoriesTab || p.state.tab == assetTrendTab && (p.current == nil || p.current.trend == nil) {
			options := ui.Place(gtx, view, image.Pt(0, offset), width, p.options)
			offset += options.Size.Y + gap
		}
	}
	body := ui.Place(gtx, view, image.Pt(0, offset), width, p.content)
	return layout.Dimensions{Size: image.Pt(width, offset+body.Size.Y)}
}

func (p *assetChangesPage) options(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	var fields []analysisFilterField
	if p.state.tab == assetCategoriesTab {
		types := []ui.Choice{
			{ID: string(application.CategoryIncome), Label: l.Text(i18n.AssetIncome)},
			{ID: string(application.CategorySpending), Label: l.Text(i18n.AssetSpending)},
			{ID: string(application.CategoryFees), Label: l.Text(i18n.AssetFees)},
			{ID: string(application.CategoryDividendInterest), Label: l.Text(i18n.ReturnIncome)},
			{ID: string(application.CategoryInvestmentReturn), Label: l.Text(i18n.ReturnTotal)},
		}
		fields = append(fields, analysisSelect(&p.category, i18n.AssetCategoryType, types, string(p.categoryType()), false, func(value string) {
			p.state.category = application.AnalysisCategoryType(value)
			p.change()
		}))
	} else {
		intervals := []ui.Choice{
			{ID: string(application.TrendDay), Label: l.Text(i18n.AssetDaily)},
			{ID: string(application.TrendWeek), Label: l.Text(i18n.AssetWeekly)},
			{ID: string(application.TrendMonth), Label: l.Text(i18n.AssetMonthly)},
		}
		fields = append(fields,
			analysisSelect(&p.interval, i18n.AssetInterval, intervals, string(p.trendInterval()), false, func(value string) {
				p.state.interval = application.AssetTrendGranularity(value)
				p.change()
			}),
			analysisSelect(&p.metric, i18n.AssetTrendMetric, assetTrendChoices(l), string(p.trendMetric()), false, func(value string) {
				p.state.metric = application.AssetTrendMetric(value)
				p.change()
			}))
	}
	return (ui.Grid{MinColumn: 160, Gap: 10, MaxColumns: len(fields)}).Layout(gtx, view, len(fields),
		func(gtx layout.Context, view ui.View, i int) layout.Dimensions { return fields[i](gtx, view) })
}

func (p *assetChangesPage) content(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	reload := i18n.HistoryRefresh
	if p.err != nil {
		reload = i18n.HistoryRetry
	}
	var rows []ui.ColumnChild
	status := i18n.NoMessage
	switch {
	case p.refsRead != nil || p.request != nil:
		status = i18n.AssetLoading
	case p.err != nil:
		status = i18n.AssetFailed
	case p.refs.origin == nil:
		status = i18n.AssetNoOrigin
	case !p.filters.selection.ready():
		status = i18n.AnalysisScopeRequired
	case p.current == nil || p.current.query.From > p.current.query.To:
		status = i18n.ReturnNoClosedDays
	}
	if status != i18n.NoMessage {
		ink := view.Colors.SecondaryInk
		if status == i18n.AssetFailed {
			ink = view.Colors.ErrorInk
		}
		rows = append(rows, ui.Rigid(view.Message(l.Text(status), ink)))
		if status == i18n.AssetFailed && p.current == nil {
			rows = append(rows, ui.Rigid(returnMonthButton(view, &p.reload, reload, nil, true)))
		}
	}
	if result := p.current; result != nil && result.query.From <= result.query.To {
		rangeLabel := wealthDay(view, result.query.From) + " – " + wealthDay(view, result.query.To)
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
				layout.Flexed(1, view.Body(ui.TextLabel, rangeLabel, view.Colors.Ink)),
				layout.Rigid(returnMonthButton(view, &p.reload, reload, nil, true)))
		}))
		switch {
		case result.drivers != nil:
			if result.drivers.Available && result.drivers.Status == domain.CompletenessPartial && p.openHealth != nil {
				rows = append(rows, ui.Rigid(p.assetCoverageLink(view, result.query)))
			}
			rows = append(rows, p.assetDriverRows(view, *result.drivers, result.query.Valuation)...)
		case result.trend != nil:
			rows = append(rows, p.assetTrendRows(view, *result.trend, result.query)...)
		case result.categories != nil:
			rows = append(rows, p.categoryRows(view, *result.categories)...)
		}
	}
	return (ui.Column{Gap: 10, MinFill: 300}).Layout(gtx, rows...)
}

func (p *assetChangesPage) assetCoverageLink(view ui.View, query domain.AnalysisQuery) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		ready := p.request == nil && p.err == nil
		if p.coverage.Clicked(gtx) && ready {
			p.openHealth(healthFindingFocus{from: query.From, to: query.To}, &p.coverage)
			gtx.Execute(op.InvalidateCmd{})
		}
		return returnMonthButton(view, &p.coverage, i18n.OverviewTrendGaps, nil, ready)(gtx)
	}
}

func (p *assetChangesPage) assetDriverRows(view ui.View, data application.AssetChangeResult, valuation domain.Valuation) []ui.ColumnChild {
	l := view.Language
	rows := []ui.ColumnChild{ui.Rigid(view.Message(analysisBasis(l, valuation, data.ValuationForced), view.Colors.SecondaryInk))}
	if !data.Available {
		return append(rows, ui.Rigid(view.Message(l.Text(i18n.AssetNoData), view.Colors.SecondaryInk)))
	}
	rows = append(rows, ui.Rigid(assetSummary(view, data.Summary, false)))
	if data.Status == domain.CompletenessPartial {
		rows = append(rows, ui.Rigid(view.Message(l.Text(assetCoverageCopy(data.MissingReason)), view.Colors.WarningInk)))
	}
	rows = append(rows, ui.Rigid(p.driverWaterfall(view, data)))
	if len(data.Groups) > 0 || data.ResidualIssueCount > 0 {
		rows = append(rows, ui.Rigid(view.Body(ui.TextLabel, l.Text(i18n.AssetAttribution), view.Colors.Ink)))
	}
	seenResidual := false
	for _, group := range data.Groups {
		groupLabel := i18n.AssetOther
		switch group.Key {
		case "cash":
			groupLabel = i18n.AssetCashFlows
		case "market":
			groupLabel = i18n.AssetMarketMoves
		case "residual":
			groupLabel = i18n.AssetResidual
		}
		for _, row := range group.Rows {
			seenResidual = seenResidual || row.Bucket == domain.BucketResidual
		}
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
				lines := []layout.FlexChild{
					layout.Rigid(headingRow(view, l.Text(groupLabel)+" · "+returnMoney(view, group.Amount), l.Text(groupLabel),
						returnMoney(view, group.Amount), view.Colors.Ink)),
					layout.Rigid(view.Hairline),
				}
				for _, row := range group.Rows {
					lines = append(lines, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.Bleed(gtx, ui.Space12, p.driverButton(view, row))
					}))
				}
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, lines...)
			})
		}))
	}
	if data.ResidualIssueCount > 0 && !seenResidual {
		rows = append(rows, ui.Rigid(p.driverButton(view, application.AssetChangeRow{
			Key: string(domain.BucketResidual), Bucket: domain.BucketResidual,
		})))
	}
	return rows
}

func assetSummary(view ui.View, totals application.AssetChangeSummary, debts bool) layout.Widget {
	l := view.Language
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
			labels := [...]i18n.MessageID{i18n.AssetBeginning, i18n.AssetEnding, i18n.AssetNetChange, i18n.AssetChangeRate}
			values := [...]string{assetLevelMoney(view, totals.BeginningValue, debts), assetLevelMoney(view, totals.EndingValue, debts),
				assetMoney(view, totals.Change, debts), returnRate(view, totals.ChangeRate)}
			// The change and its rate carry the gain or loss color. A debt that
			// grows is a loss, so its sign flips.
			tone := returnTone(totals.ChangeRate, totals.Change)
			if debts {
				tone = -tone
			}
			ink := func(i int) color.NRGBA {
				switch {
				case i < 2 || tone == 0:
					return view.Colors.Ink
				case tone > 0:
					return view.Colors.Success
				}
				return view.Colors.Destructive
			}
			return (ui.Grid{MinColumn: 135, Gap: 12, MaxColumns: len(labels), Balanced: true}).Layout(gtx, view, len(labels),
				func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
						layout.Rigid(view.Message(l.Text(labels[i]), view.Colors.SecondaryInk)),
						layout.Rigid(summaryFigure(view, values[i], ink(i))))
				})
		})
	}
}

func assetMoney(view ui.View, value *domain.SignedMoney, debts bool) string {
	if !debts || value == nil {
		return returnMoney(view, value)
	}
	return valuationMoney(view, domain.MoneyView{Amount: value.Amount().Neg().String(), Currency: value.Currency()}, false)
}

func assetLevelMoney(view ui.View, value *domain.SignedMoney, debts bool) string {
	if value == nil {
		return returnMoney(view, nil)
	}
	return valuationMoney(view, domain.MoneyView{Amount: value.CanonicalAmount(), Currency: value.Currency()}, debts)
}

func (p *assetChangesPage) categoryRows(view ui.View, data application.CategoriesResult) []ui.ColumnChild {
	l := view.Language
	label, total := l.Text(i18n.AssetCategoryTotal), returnMoney(view, data.Total)
	rows := []ui.ColumnChild{ui.Rigid(labeledRow(view, label+" · "+total, label, total, view.Colors.Ink))}
	if data.ValuationForced != nil {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.AnalysisBaseFallback), view.Colors.SecondaryInk)))
	}
	if data.Status == domain.CompletenessPartial {
		rows = append(rows, ui.Rigid(view.Message(l.Text(assetCoverageCopy(data.MissingReason)), view.Colors.WarningInk)))
	}
	if !data.Available || len(data.Rows) == 0 {
		return append(rows, ui.Rigid(view.Message(l.Text(i18n.AssetNoData), view.Colors.SecondaryInk)))
	}
	if len(data.Rows) <= 6 {
		// A short list is one card as tall as its rows, not a scroller with
		// empty space under them.
		return append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, 2*len(data.Rows))
				for i := range data.Rows {
					if i > 0 {
						children = append(children, layout.Rigid(view.Hairline))
					}
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: 8, Bottom: 8, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return p.categoryButton(gtx, view, data.Rows[i])
						})
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		}))
	}
	return append(rows, ui.Fill(func(gtx layout.Context) layout.Dimensions {
		return analysisResultList(gtx, view, &p.list, l.Text(i18n.AssetCategories), len(data.Rows), func(gtx layout.Context, i int) layout.Dimensions {
			return p.categoryButton(gtx, view, data.Rows[i])
		})
	}))
}

func assetCoverageCopy(reason string) i18n.MessageID {
	switch reason {
	case "one or more asset days are incomplete":
		return i18n.AssetDaysIncomplete
	case "funding cash is recorded after the product start":
		return i18n.WealthHistoryFundingGap
	default:
		return i18n.OverviewIncomplete
	}
}

func assetBucketLabel(l i18n.Language, bucket domain.AttributionBucket) string {
	switch bucket {
	case domain.BucketExternalFlow:
		return l.Text(i18n.AssetExternalFlow)
	case domain.BucketIncome:
		return l.Text(i18n.AssetIncome)
	case domain.BucketSpending:
		return l.Text(i18n.AssetSpending)
	case domain.BucketFee:
		return l.Text(i18n.AssetFees)
	case domain.BucketLiabilityImpact:
		return l.Text(i18n.AssetLiabilityImpact)
	case domain.BucketAdjustment:
		return l.Text(i18n.AssetAdjustment)
	case domain.BucketResidual:
		return l.Text(i18n.AssetResidual)
	default:
		return returnComponentLabel(l, domain.ReturnComponent(bucket))
	}
}
