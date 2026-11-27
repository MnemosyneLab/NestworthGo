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
	if p.err != nil && p.refsRead == nil && p.request == nil && p.reload.Clicked(gtx) {
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
		kinds := []application.AnalysisCategoryType{application.CategoryIncome, application.CategorySpending, application.CategoryFees,
			application.CategoryInvestmentReturn, application.CategoryDividendInterest}
		types := make([]ui.Choice, 0, len(kinds))
		for _, kind := range kinds {
			types = append(types, ui.Choice{ID: string(kind), Label: l.Text(assetCategoryLabel(kind))})
		}
		fields = append(fields, analysisSelect(&p.category, i18n.AssetCategoryType, types, string(p.categoryType()), false, func(value string) {
			p.state.category = application.AnalysisCategoryType(value)
			p.change()
		}), func(gtx layout.Context, view ui.View) layout.Dimensions {
			return view.Message(l.Text(i18n.AssetCategoryHint), view.Colors.SecondaryInk)(gtx)
		})
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
		if status == i18n.AssetFailed {
			rows = append(rows, ui.Rigid(returnMonthButton(view, &p.reload, i18n.HistoryRetry, nil, true)))
		}
	}
	if result := p.current; result != nil && result.query.From <= result.query.To {
		rangeLabel := wealthDay(view, result.query.From) + " – " + wealthDay(view, result.query.To)
		rows = append(rows, ui.Rigid(view.Body(ui.TextLabel, rangeLabel, view.Colors.Ink)))
		switch {
		case result.drivers != nil:
			if result.drivers.Available && result.drivers.Status == domain.CompletenessPartial && p.openHealth != nil {
				rows = append(rows, ui.Rigid(p.assetCoverageLink(view, result.query)))
			}
			rows = append(rows, p.assetDriverRows(view, *result.drivers, result.query)...)
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

func (p *assetChangesPage) assetDriverRows(view ui.View, data application.AssetChangeResult, query domain.AnalysisQuery) []ui.ColumnChild {
	l := view.Language
	var rows []ui.ColumnChild
	if data.ValuationForced == nil || !data.Available {
		rows = append(rows, ui.Rigid(view.Message(analysisBasis(l, query.Valuation, data.ValuationForced), view.Colors.SecondaryInk)))
	}
	if !data.Available {
		hint := i18n.AssetDriverNoDataHint
		if data.MissingReason == "no usable asset days are available for this period" {
			hint = i18n.AssetDriverNoUsableDays
		}
		return append(rows, ui.Rigid(assetEmpty(view, i18n.AssetDriverNoData, hint)))
	}
	rows = append(rows, ui.Rigid(assetDriverSummary(view, data, query.Scope.Kind)), ui.Rigid(p.driverContributions(view, data)))
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
				label, amount := l.Text(groupLabel), assetSignedMoney(view, group.Amount)
				lines := []layout.FlexChild{
					layout.Rigid(headingRow(view, label+" · "+amount, label, amount, view.Colors.Ink)),
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

func assetDriverSummary(view ui.View, data application.AssetChangeResult, scope domain.ScopeKind) layout.Widget {
	title := i18n.AssetNetWorthChange
	switch scope {
	case domain.ScopeAccount:
		title = i18n.AssetAccountValueChange
	case domain.ScopeInstrument:
		title = i18n.AssetInvestmentValueChange
	}
	l := view.Language
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
			rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextHeading, l.Text(title), view.Colors.Ink))}
			if data.ValuationForced != nil {
				rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.AssetValuationForced), view.Colors.WarningInk)))
			}
			rows = append(rows, ui.Rigid(assetSummaryFigures(view, data.Summary, false, true)))
			rateHint := l.Text(i18n.AssetSummaryRateHint)
			if data.Summary.ChangeRate == nil {
				reason := i18n.AssetSummaryRateIncomplete
				if data.Summary.ChangeRateMissingReason == "nonpositive_beginning" {
					reason = i18n.AssetSummaryRateNonpositive
				}
				rateHint += " " + l.Text(reason)
			}
			rows = append(rows, ui.Rigid(view.Message(rateHint, view.Colors.SecondaryInk)))
			if data.Status == domain.CompletenessPartial {
				coverage := l.Text(i18n.AssetPartialCoverage)
				if data.MissingReason != "" {
					coverage += " · " + l.Text(assetCoverageCopy(data.MissingReason))
				}
				rows = append(rows, ui.Rigid(view.Message(coverage, view.Colors.WarningInk)))
			}
			return (ui.Column{Gap: ui.Space12}).Layout(gtx, rows...)
		})
	}
}

func assetSummary(view ui.View, totals application.AssetChangeSummary, debts bool) layout.Widget {
	figures := assetSummaryFigures(view, totals, debts, false)
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.CardPadded(gtx, figures)
	}
}

func assetSummaryFigures(view ui.View, totals application.AssetChangeSummary, debts, drivers bool) layout.Widget {
	figures := [...]struct {
		label i18n.MessageID
		value string
		style ui.TextStyle
	}{
		{i18n.AssetBeginning, assetLevelMoney(view, totals.BeginningValue, debts), ui.TextMoneyMD},
		{i18n.AssetEnding, assetLevelMoney(view, totals.EndingValue, debts), ui.TextMoneyMD},
		{i18n.AssetNetChange, assetMoney(view, totals.Change, debts), ui.TextMoneyMD},
		{i18n.AssetChangeRate, returnRate(view, totals.ChangeRate), ui.TextMetricMD},
	}
	if drivers {
		figures[2].label, figures[3].label = i18n.AssetSummaryChange, i18n.AssetSummaryRate
		figures[2].value = assetSignedMoney(view, totals.Change)
	}
	// The change and its rate carry the gain or loss color. A debt that grows
	// is a loss, so its sign flips.
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
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Grid{MinColumn: 135, Gap: 12, MaxColumns: len(figures), Balanced: true}).Layout(gtx, view, len(figures),
			func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
				figure := figures[i]
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
					layout.Rigid(view.Message(view.Language.Text(figure.label), view.Colors.SecondaryInk)),
					layout.Rigid(summaryFigure(view, figure.style, figure.value, ink(i))))
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
	if !data.Available {
		title, hint := i18n.AssetCategoryNoPositions, i18n.AssetCategoryNoPositionsHint
		if len(data.Rows) > 0 {
			title, hint = i18n.AssetCategoryLowHistory, i18n.AssetCategoryLowHistoryHint
		}
		return []ui.ColumnChild{ui.Rigid(assetEmpty(view, title, hint))}
	}
	rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextHeading, l.Text(assetCategoryLabel(p.categoryType())), view.Colors.Ink))}
	if data.Total != nil {
		label, total := l.Text(i18n.AssetCategoryTotal), assetSignedMoney(view, data.Total)
		rows = append(rows, ui.Rigid(labeledRow(view, label+" · "+total, label, total, view.Colors.Ink)))
	}
	if data.ValuationForced != nil {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.AnalysisBaseFallback), view.Colors.SecondaryInk)))
	}
	if data.Status == domain.CompletenessPartial {
		rows = append(rows, ui.Rigid(view.Message(l.Text(assetCoverageCopy(data.MissingReason)), view.Colors.WarningInk)))
	}
	if len(data.Rows) == 0 {
		return append(rows, ui.Rigid(assetEmpty(view, i18n.AssetCategoryEmpty, i18n.AssetCategoryEmptyHint)))
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

func assetCategoryLabel(kind application.AnalysisCategoryType) i18n.MessageID {
	switch kind {
	case application.CategoryIncome:
		return i18n.AssetIncome
	case application.CategoryFees:
		return i18n.AssetFees
	case application.CategoryInvestmentReturn:
		return i18n.AssetCategoryInvestment
	case application.CategoryDividendInterest:
		return i18n.AssetCategoryDividend
	default:
		return i18n.AssetSpending
	}
}

func assetEmpty(view ui.View, title, hint i18n.MessageID) layout.Widget {
	l := view.Language
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
			return (ui.Column{Gap: ui.Space4}).Layout(gtx,
				ui.Rigid(view.Body(ui.TextLabel, l.Text(title), view.Colors.Ink)),
				ui.Rigid(view.Message(l.Text(hint), view.Colors.SecondaryInk)))
		})
	}
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
	case domain.BucketDividendInterest:
		return l.Text(i18n.AssetCategoryDividend)
	case domain.BucketPriceChange:
		return l.Text(i18n.AssetDriverPrice)
	case domain.BucketFXImpact:
		return l.Text(i18n.AssetDriverFX)
	case domain.BucketFXConversionSpread:
		return l.Text(i18n.AssetDriverFXSpread)
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
