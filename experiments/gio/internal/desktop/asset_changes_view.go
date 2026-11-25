package desktop

import (
	"image"

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
	tabs := (ui.Wrap{Gap: 8}).Layout(gtx, len(assetTabs), func(gtx layout.Context, i int) layout.Dimensions {
		return analysisToggle(view, &p.tabs[i], l.Text(assetTabs[i]), p.state.tab == assetChangesTab(i))(gtx)
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
		if p.state.tab != assetDriversTab {
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
		metrics := []ui.Choice{
			{ID: string(application.TrendNetWorth), Label: l.Text(i18n.NetWorth)},
			{ID: string(application.TrendAssets), Label: l.Text(i18n.Assets)},
			{ID: string(application.TrendLiabilities), Label: l.Text(i18n.Liabilities)},
			{ID: string(application.TrendNetChange), Label: l.Text(i18n.AssetNetChange)},
		}
		intervals := []ui.Choice{
			{ID: string(application.TrendDay), Label: l.Text(i18n.AssetDaily)},
			{ID: string(application.TrendWeek), Label: l.Text(i18n.AssetWeekly)},
			{ID: string(application.TrendMonth), Label: l.Text(i18n.AssetMonthly)},
		}
		fields = append(fields,
			analysisSelect(&p.metric, i18n.ReturnDisplay, metrics, string(p.trendMetric()), false, func(value string) {
				p.state.metric = application.AssetTrendMetric(value)
				p.change()
			}),
			analysisSelect(&p.interval, i18n.AssetInterval, intervals, string(p.trendInterval()), false, func(value string) {
				p.state.interval = application.AssetTrendGranularity(value)
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
	rows := []ui.ColumnChild{ui.Rigid(returnMonthButton(view, &p.reload, reload, nil, true))}
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
		rows = append(rows, ui.Rigid(view.Message(l.Text(status), view.Colors.SecondaryInk)))
	}
	if result := p.current; result != nil && result.query.From <= result.query.To {
		rangeLabel := wealthDay(view, result.query.From) + " – " + wealthDay(view, result.query.To)
		rows = append(rows, ui.Rigid(view.Body(ui.TextLabel, rangeLabel, view.Colors.Ink)))
		switch {
		case result.drivers != nil:
			rows = append(rows, assetDriverRows(view, *result.drivers, result.query.Valuation)...)
		case result.trend != nil:
			rows = append(rows, p.assetTrendRows(view, *result.trend)...)
		case result.categories != nil:
			rows = append(rows, p.categoryRows(view, *result.categories)...)
		}
	}
	return (ui.Column{Gap: 10, MinFill: 300}).Layout(gtx, rows...)
}

func assetDriverRows(view ui.View, data application.AssetChangeResult, valuation domain.Valuation) []ui.ColumnChild {
	l := view.Language
	rows := []ui.ColumnChild{ui.Rigid(view.Message(analysisBasis(l, valuation, data.ValuationForced), view.Colors.SecondaryInk))}
	if !data.Available {
		return append(rows, ui.Rigid(view.Message(l.Text(i18n.AssetNoData), view.Colors.SecondaryInk)))
	}
	rows = append(rows, ui.Rigid(assetSummary(view, data.Summary, false)))
	if data.Status == domain.CompletenessPartial {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.OverviewIncomplete), view.Colors.SecondaryInk)))
	}
	for _, group := range data.Groups {
		groupLabel := i18n.AssetOther
		switch group.Key {
		case "cash":
			groupLabel = i18n.AssetCashFlows
		case "market":
			groupLabel = i18n.AssetMarketMoves
		}
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lines := []layout.FlexChild{
						layout.Rigid(view.Body(ui.TextLabel, l.Text(groupLabel)+" · "+returnMoney(view, group.Amount), view.Colors.Ink)),
					}
					for _, row := range group.Rows {
						label := assetBucketLabel(l, row.Bucket) + " · " + returnMoney(view, row.Amount)
						lines = append(lines, layout.Rigid(view.Message(label, view.Colors.Ink)))
					}
					return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(7)}.Layout(gtx, lines...)
				})
			})
		}))
	}
	return rows
}

func assetSummary(view ui.View, totals application.AssetChangeSummary, debts bool) layout.Widget {
	l := view.Language
	return func(gtx layout.Context) layout.Dimensions {
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				labels := [...]i18n.MessageID{i18n.AssetBeginning, i18n.AssetEnding, i18n.AssetNetChange, i18n.AssetChangeRate}
				values := [...]string{assetMoney(view, totals.BeginningValue, debts), assetMoney(view, totals.EndingValue, debts),
					assetMoney(view, totals.Change, debts), returnRate(view, totals.ChangeRate)}
				return (ui.Grid{MinColumn: 135, Gap: 10, MaxColumns: len(labels), Balanced: true}).Layout(gtx, view, len(labels),
					func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(5)}.Layout(gtx,
							layout.Rigid(view.Message(l.Text(labels[i]), view.Colors.SecondaryInk)),
							layout.Rigid(view.Body(ui.TextLabel, values[i], view.Colors.Ink)))
					})
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

func (p *assetChangesPage) assetTrendRows(view ui.View, data application.AssetTrendResult) []ui.ColumnChild {
	l := view.Language
	var rows []ui.ColumnChild
	debts := p.trendMetric() == application.TrendLiabilities
	if data.ValueChange != nil {
		rows = append(rows, ui.Rigid(assetSummary(view, *data.ValueChange, debts)))
	}
	if !data.Available {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.AssetNoData), view.Colors.SecondaryInk)))
	}
	if len(data.Points) != 0 {
		rows = append(rows, ui.Fill(func(gtx layout.Context) layout.Dimensions {
			return analysisResultList(gtx, view, &p.list, l.Text(i18n.AssetTrend), len(data.Points), func(gtx layout.Context, i int) layout.Dimensions {
				point := data.Points[i]
				value := assetMoney(view, point.Value, debts)
				if display := point.DisplayValuation; display != nil {
					money := display.Total
					if money == nil && display.KnownSubtotal != nil {
						money = display.KnownSubtotal
						value = valuationMoney(view, *money, debts) + " · " + l.Text(i18n.OverviewKnownSubtotal)
					} else if money != nil {
						value = valuationMoney(view, *money, debts)
						if display.Quality.Coverage == domain.CoverageProvisional {
							value += " · " + l.Text(i18n.WealthHistoryProvisional)
						}
					}
				}
				return view.Message(wealthDay(view, point.Period)+" · "+value, view.Colors.Ink)(gtx)
			})
		}))
	}
	return rows
}

func (p *assetChangesPage) categoryRows(view ui.View, data application.CategoriesResult) []ui.ColumnChild {
	l := view.Language
	rows := []ui.ColumnChild{ui.Rigid(view.Message(l.Text(i18n.AssetCategoryTotal)+" · "+returnMoney(view, data.Total), view.Colors.Ink))}
	if len(data.Rows) == 0 {
		return append(rows, ui.Rigid(view.Message(l.Text(i18n.AssetNoData), view.Colors.SecondaryInk)))
	}
	return append(rows, ui.Fill(func(gtx layout.Context) layout.Dimensions {
		return analysisResultList(gtx, view, &p.list, l.Text(i18n.AssetCategories), len(data.Rows), func(gtx layout.Context, i int) layout.Dimensions {
			row := data.Rows[i]
			name := p.refs.names[row.Key]
			if row.AssetClass == domain.BucketCash && row.InstrumentID == "" && row.AccountID == "" {
				name = l.Text(i18n.AccountCash)
			} else if name == "" {
				name = l.Text(i18n.OverviewEvidenceUnavailable)
			}
			return view.Message(name+" · "+returnMoney(view, row.Amount), view.Colors.Ink)(gtx)
		})
	}))
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
