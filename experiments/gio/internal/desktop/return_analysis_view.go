package desktop

import (
	"fmt"
	"image"
	"strings"
	"time"

	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *returnAnalysisPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.update(view.WeekStart == "sunday")
	l := view.Language
	for i := range p.tabs {
		if p.tabs[i].Clicked(gtx) {
			p.changeTab(returnAnalysisTab(i))
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	tabs := func(gtx layout.Context) layout.Dimensions { return p.layoutTabs(gtx, view) }
	if p.refs.origin == nil {
		status := i18n.ReturnNoOrigin
		switch {
		case p.refsRead != nil:
			status = i18n.ReturnLoading
		case p.err != nil:
			status = i18n.ReturnFailed
		}
		return ui.Column{Gap: 12}.Layout(gtx, ui.Rigid(tabs),
			ui.Rigid(view.Message(l.Text(status), view.Colors.SecondaryInk)),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				if p.refsRead != nil {
					return layout.Dimensions{}
				}
				if p.reload.Clicked(gtx) {
					p.readRefs()
					gtx.Execute(op.InvalidateCmd{})
				}
				label := i18n.HistoryRefresh
				if p.err != nil {
					label = i18n.HistoryRetry
				}
				view.Focus.Watch(gtx, &p.reload)
				return view.Button(&p.reload, l.Text(label))(gtx)
			}),
		)
	}
	origin := p.refs.origin
	width := gtx.Constraints.Max.X
	gtx.Constraints.Min.Y = 0
	offset := tabs(gtx).Size.Y + gtx.Dp(12)
	filter := ui.Place(gtx, view, image.Pt(0, offset), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
		now := p.clock()
		effective := p.periodQuery(p.month, now)
		return p.filters.layout(gtx, view, p.refs.analysisReferences, origin, now, effective, p.changeFilters)
	})
	offset += filter.Size.Y + gtx.Dp(12)
	layoutBody := p.layoutCalendar
	if p.tab != returnCalendarTab {
		layoutBody = p.layoutProjection
	}
	body := ui.Place(gtx, view, image.Pt(0, offset), width, layoutBody)
	return layout.Dimensions{Size: image.Pt(width, offset+body.Size.Y)}
}

func (p *returnAnalysisPage) layoutTabs(gtx layout.Context, view ui.View) layout.Dimensions {
	labels := [...]i18n.MessageID{i18n.ReturnCalendar, i18n.ReturnTrend, i18n.ReturnContribution}
	return view.TabTrack(gtx, len(labels), func(gtx layout.Context, index int) layout.Dimensions {
		return view.TabSegment(&p.tabs[index], view.Language.Text(labels[index]), p.tab == returnAnalysisTab(index))(gtx)
	})
}

func (p *returnAnalysisPage) layoutCalendar(gtx layout.Context, view ui.View) layout.Dimensions {
	l, origin := view.Language, p.refs.origin
	localNow := p.clock().In(origin.location)
	thisMonth := returnMonth(localNow)
	earliest, _ := time.Parse("2006-01", origin.first[:7])
	p.updateMonthButtons(gtx, earliest, thisMonth)
	p.updateYearSelection(gtx, earliest, thisMonth)
	rows := []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return p.monthControls(gtx, view, earliest, thisMonth)
	})}
	switch {
	case !p.filters.selection.ready():
		label := i18n.AnalysisScopeRequired
		if p.filters.selection.from != "" && p.filters.selection.to != "" && p.filters.selection.from > p.filters.selection.to {
			label = i18n.AnalysisInvalidRange
		}
		rows = append(rows, ui.Rigid(view.Message(l.Text(label), view.Colors.SecondaryInk)))
	case p.request != nil:
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.ReturnLoading), view.Colors.SecondaryInk)))
	case p.err != nil:
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.ReturnFailed), view.Colors.ErrorInk)))
		if p.current != nil {
			rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.WealthHistoryRetained), view.Colors.SecondaryInk)))
		}
	}
	if p.current != nil {
		grid := func(gtx layout.Context) layout.Dimensions {
			if p.year {
				return p.layoutYear(gtx, view, earliest, thisMonth)
			}
			return p.monthGrid(gtx, view, localNow.Format(time.DateOnly))
		}
		rows = append(rows,
			ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.monthSummary(gtx, view) }),
			ui.Rigid(grid))
		if len(p.current.summary.TopContributors) > 0 {
			rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.contributors(gtx, view, p.current.summary.TopContributors)
			}))
		}
	}
	return ui.Column{Gap: 12}.Layout(gtx, rows...)
}

func (p *returnAnalysisPage) updateMonthButtons(gtx layout.Context, earliest, latest time.Time) {
	month := p.month
	before, after := p.periodDirections(earliest, latest)
	modeChanged, step := false, 1
	if p.year {
		step = 12
	}
	switch {
	case p.monthMode.Clicked(gtx) && p.year:
		p.year, modeChanged = false, true
	case p.yearMode.Clicked(gtx) && !p.year:
		p.year, modeChanged = true, true
	case p.previous.Clicked(gtx) && before:
		month = month.AddDate(0, -step, 0)
	case p.next.Clicked(gtx) && after:
		month = month.AddDate(0, step, 0)
	case p.today.Clicked(gtx):
		month = latest
	}
	if p.year {
		if month.Before(earliest) {
			month = earliest
		}
		if month.After(latest) {
			month = latest
		}
	}
	if modeChanged {
		p.current = nil
	}
	reload := p.reload.Clicked(gtx)
	if modeChanged || month != p.month || reload && p.request == nil {
		p.readMonth(month, p.sunday)
		gtx.Execute(op.InvalidateCmd{})
	}
}

func (p *returnAnalysisPage) monthControls(gtx layout.Context, view ui.View, earliest, latest time.Time) layout.Dimensions {
	l := view.Language
	reload := i18n.HistoryRefresh
	if p.err != nil {
		reload = i18n.HistoryRetry
	}
	name, previous, next := l.Text(i18n.DateMonthLayout), i18n.DatePreviousMonth, i18n.DateNextMonth
	if p.year {
		name, previous, next = l.Text(i18n.ReturnYearLayout), i18n.ReturnPreviousYear, i18n.ReturnNextYear
	}
	before, after := p.periodDirections(earliest, latest)
	controls := []layout.Widget{
		returnMonthButton(view, &p.previous, previous, ui.Glyph(lucide.ChevronLeft), before),
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return view.Line(gtx, ui.TextHeading, p.month.Format(name), view.Colors.Ink)
			})
		},
		returnMonthButton(view, &p.next, next, ui.Glyph(lucide.ChevronRight), after),
		returnMonthButton(view, &p.today, i18n.ReturnToday, nil, p.month != latest),
		analysisToggle(view, &p.monthMode, l.Text(i18n.ReturnMonth), !p.year),
		analysisToggle(view, &p.yearMode, l.Text(i18n.ReturnYear), p.year),
		returnMonthButton(view, &p.reload, reload, nil, true),
	}
	return (ui.Wrap{Gap: 8}).Layout(gtx, len(controls), func(gtx layout.Context, index int) layout.Dimensions {
		return controls[index](gtx)
	})
}

func returnMonthButton(view ui.View, button *widget.Clickable, label i18n.MessageID, icon ui.Glyph, enabled bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if !enabled {
			gtx = gtx.Disabled()
		}
		view.Focus.Watch(gtx, button)
		text := view.Language.Text(label)
		spec := ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Name: text, ShortName: text, Icon: icon}
		if icon == nil {
			spec.Title = text
		}
		return view.ButtonOf(button, spec)(gtx)
	}
}

func (p *returnAnalysisPage) monthSummary(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	data := p.current.summary
	if p.current.summaryFrom > p.current.summaryTo {
		return view.Message(l.Text(i18n.ReturnNoClosedDays), view.Colors.SecondaryInk)(gtx)
	}
	if !data.Available && data.Summary.MissingValuationDays == 0 {
		id := i18n.ReturnNoInvestments
		if len(data.Days) > 0 {
			id = i18n.ReturnInsufficient
		}
		return view.Message(l.Text(id), view.Colors.SecondaryInk)(gtx)
	}
	summary := data.Summary
	caption := fmt.Sprintf(l.Text(i18n.ReturnSummaryRange), wealthDay(view, string(p.current.summaryFrom)),
		wealthDay(view, string(p.current.summaryTo)), string(data.Currency))
	return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(view.Body(ui.TextLabel, caption, view.Colors.Ink)),
				layout.Rigid(view.Message(analysisBasis(l, data.Valuation, data.ValuationForced), view.Colors.SecondaryInk)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					labels := [...]i18n.MessageID{i18n.ReturnAmount, i18n.ReturnRate, i18n.ReturnBeginning, i18n.ReturnEnding}
					values := [...]string{returnAmountText(view, summary.ReturnAmount, summary.AmountStatus), returnRate(view, summary.ReturnRate),
						returnMoney(view, summary.BeginningInvestedValue), returnMoney(view, summary.EndingInvestedValue)}
					return (ui.Grid{MinColumn: 140, Gap: 8, MaxColumns: 4, Balanced: true}).Layout(gtx, view, len(labels),
						func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
								layout.Rigid(view.Message(l.Text(labels[i]), view.Colors.SecondaryInk)),
								layout.Rigid(view.Body(ui.TextLabel, values[i], view.Colors.Ink)),
							)
						})
				}),
				layout.Rigid(view.Message(returnCoverage(l, summary.Coverage, summary.ReturnDayCoverage), view.Colors.SecondaryInk)),
			)
		})
	})
}

var returnWeekdayLabels = [...]i18n.MessageID{i18n.WeekdaySunday, i18n.WeekdayMonday, i18n.WeekdayTuesday,
	i18n.WeekdayWednesday, i18n.WeekdayThursday, i18n.WeekdayFriday, i18n.WeekdaySaturday}

func (p *returnAnalysisPage) monthGrid(gtx layout.Context, view ui.View, today string) layout.Dimensions {
	l := view.Language
	start, count := returnGrid(p.month, p.sunday)
	columns := ui.Grid{MinColumn: 1, MaxColumns: 7, Gap: 4}
	board := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return columns.Layout(gtx, view, 7, func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
			return layout.Center.Layout(gtx, view.Body(ui.TextCaption,
				l.Text(returnWeekdayLabels[(int(start.Weekday())+i)%7]), view.Colors.SecondaryInk))
		})
	})}
	board = append(board, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return columns.Layout(gtx, view, count, func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
			return p.dayCell(gtx, view, start.AddDate(0, 0, i), today, &p.cells[i])
		})
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(5)}.Layout(gtx, board...)
}

func (p *returnAnalysisPage) dayCell(gtx layout.Context, view ui.View, date time.Time, today string, button *widget.Clickable) layout.Dimensions {
	l := view.Language
	day := date.Format(time.DateOnly)
	data, exists := p.current.days[domain.LocalDate(day)]
	title, caption := fmt.Sprint(date.Day()), ""
	if date.Month() != p.month.Month() || date.Day() == 1 {
		title = date.Format(l.Text(i18n.DateDayLayout))
	}
	switch {
	case day == today:
		caption = l.Text(i18n.ReturnDayOpen)
	case day < today && exists && data.Available:
		caption = returnRate(view, data.ReturnRate) + " · " + returnAmountText(view, data.ReturnAmount, data.AmountStatus)
	case day < today:
		caption = l.Text(i18n.ReturnNoData)
	}
	name := wealthDay(view, day)
	if caption != "" {
		name += " · " + caption
	}
	enabled := day < today && exists && gtx.Enabled()
	border := view.Colors.Divider
	if gtx.Focused(button) {
		border = view.Colors.Accent
	}
	if !enabled {
		gtx = gtx.Disabled()
	} else {
		view.Focus.Watch(gtx, button)
		if p.returnFocus == button {
			p.returnFocus = nil
			gtx.Execute(key.FocusCmd{Tag: button})
		}
		if button.Clicked(gtx) {
			p.openDay(domain.LocalDate(day), button)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	gtx.Constraints.Min.Y = gtx.Dp(76)
	return accessibility.Button(gtx, button, name, "", false, func(gtx layout.Context) layout.Dimensions {
		return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			semantic.Button.Add(gtx.Ops)
			semantic.DescriptionOp(name).Add(gtx.Ops)
			return view.Panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
						layout.Rigid(view.Body(ui.TextLabel, title, view.Colors.Ink)),
						layout.Rigid(view.Body(ui.TextCaption, caption, view.Colors.SecondaryInk)),
					)
				})
			})
		})
	})
}

func (p *returnAnalysisPage) layoutDay(gtx layout.Context, view ui.View) {
	sheet := p.day
	if sheet.dismissed(gtx) {
		p.closeDay()
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	select {
	case result := <-sheet.request:
		sheet.cancel()
		sheet.request, sheet.err = nil, result.err
		if result.err == nil {
			sheet.value, sheet.hasValue = result.value, true
		}
	default:
	}
	if sheet.err != nil && sheet.retry.Clicked(gtx) {
		p.readDay()
		gtx.Execute(key.FocusCmd{Tag: &sheet.close})
		gtx.Execute(op.InvalidateCmd{})
	}
	action := i18n.NoMessage
	if p.openAssets != nil && sheet.hasValue {
		action = i18n.AssetOpenDay
		if sheet.action.Clicked(gtx) {
			selection := p.filters.selection
			selection.from, selection.to = sheet.date, sheet.date
			p.closeDay()
			p.openAssets(selection, sheet.button)
			gtx.Execute(op.InvalidateCmd{})
			return
		}
	}
	sheet.layout(gtx, view, wealthDay(view, sheet.date), func(gtx layout.Context) layout.Dimensions {
		return p.dayDetails(gtx, view, sheet)
	}, action)
}

func (p *returnAnalysisPage) dayDetails(gtx layout.Context, view ui.View, sheet *returnDaySheet) layout.Dimensions {
	l := view.Language
	switch {
	case sheet.request != nil:
		return view.Message(l.Text(i18n.ReturnLoading), view.Colors.SecondaryInk)(gtx)
	case sheet.err != nil:
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(10)}.Layout(gtx,
			layout.Rigid(view.Message(l.Text(i18n.ReturnFailed), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&sheet.retry, l.Text(i18n.HistoryRetry))),
		)
	case !sheet.hasValue:
		return view.Message(l.Text(i18n.ReturnNoData), view.Colors.SecondaryInk)(gtx)
	}
	data := sheet.value
	return view.List(&sheet.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		var children []layout.FlexChild
		for _, value := range []string{analysisBasis(l, data.Valuation, data.ValuationForced) + " · " + string(data.Currency),
			l.Text(i18n.ReturnAmount) + " · " + returnAmountText(view, data.ReturnAmount, data.AmountStatus),
			l.Text(i18n.ReturnRate) + " · " + returnRate(view, data.ReturnRate),
			returnCoverage(l, data.Coverage, application.ReturnDayCoverage{})} {
			children = append(children, layout.Rigid(view.Message(value, view.Colors.Ink)))
		}
		if len(data.Composition) > 0 {
			children = append(children, layout.Rigid(view.Body(ui.TextLabel, l.Text(i18n.ReturnComposition), view.Colors.Ink)))
			for _, component := range data.Composition {
				label := returnComponentLabel(l, component.Component)
				children = append(children, layout.Rigid(view.Message(label+" · "+returnMoney(view, component.Amount), view.Colors.Ink)))
			}
		}
		if len(data.Contributors) > 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.contributors(gtx, view, data.Contributors)
			}))
		}
		for _, issue := range data.Issues {
			label := i18n.ReturnInsufficient
			if issue.MissingReason == "funding cash is recorded after the product start" {
				label = i18n.WealthHistoryFundingGap
			}
			children = append(children, layout.Rigid(view.Message(l.Text(label), view.Colors.SecondaryInk)))
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, children...)
	})
}

func analysisBasis(l i18n.Language, valuation domain.Valuation, forced *string) string {
	switch {
	case forced != nil:
		return l.Text(i18n.AnalysisBaseFallback)
	case valuation == domain.ValuationNative:
		return l.Text(i18n.AnalysisNative)
	default:
		return l.Text(i18n.AnalysisBase)
	}
}

func (p *returnAnalysisPage) contributors(gtx layout.Context, view ui.View, values []application.ReturnContributor) layout.Dimensions {
	l := view.Language
	rows := []layout.FlexChild{layout.Rigid(view.Body(ui.TextLabel, l.Text(i18n.ReturnContributors), view.Colors.Ink))}
	for _, value := range values {
		name := p.refs.names[value.Key]
		kind, currency, _ := strings.Cut(value.Key, ":")
		switch {
		case kind == domain.BucketCash && currency == "":
			name = l.Text(i18n.AccountCash)
		case kind == domain.BucketCash:
			name = l.Text(i18n.AccountCash) + " · " + currency
		case name == "":
			name = l.Text(i18n.HistoryInstrumentUnavailable)
		}
		rows = append(rows, layout.Rigid(view.Message(name+" · "+returnMoney(view, value.Amount), view.Colors.Ink)))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx, rows...)
}

func returnComponentLabel(l i18n.Language, component domain.ReturnComponent) string {
	labels := map[domain.ReturnComponent]i18n.MessageID{
		domain.ReturnPriceChange: i18n.ReturnPrice, domain.ReturnFXImpact: i18n.ReturnFX,
		domain.ReturnFXConversionSpread: i18n.ReturnFXSpread, domain.ReturnDividendInterest: i18n.ReturnIncome,
		domain.ReturnInvestmentFee: i18n.ReturnFee,
	}
	label, ok := labels[component]
	if !ok {
		label = i18n.OverviewEvidenceUnavailable
	}
	return l.Text(label)
}

func returnCoverage(l i18n.Language, core domain.RateCoverage, gaps application.ReturnDayCoverage) string {
	parts := []string{fmt.Sprintf(l.Text(i18n.ReturnCoverage), core.RatedDays, core.TotalDays)}
	if gaps.MissingValuationDays > 0 {
		parts = append(parts, fmt.Sprintf(l.Text(i18n.ReturnMissingValuation), gaps.MissingValuationDays))
	}
	if gaps.AmountWithoutRateDays > 0 {
		parts = append(parts, fmt.Sprintf(l.Text(i18n.ReturnAmountOnly), gaps.AmountWithoutRateDays))
	}
	return strings.Join(parts, " · ")
}

func returnMoney(view ui.View, value *domain.SignedMoney) string {
	if value == nil {
		return view.Language.Text(i18n.OverviewNoValue)
	}
	return valuationMoney(view, domain.MoneyView{Amount: value.CanonicalAmount(), Currency: value.Currency()}, false)
}

func returnAmountText(view ui.View, value *domain.SignedMoney, status string) string {
	text := returnMoney(view, value)
	if status == application.ReturnAmountPartial {
		return text + " · " + view.Language.Text(i18n.OverviewKnownSubtotal)
	}
	return text
}

func returnRate(view ui.View, value *decimal.Decimal) string {
	if value == nil {
		return view.Language.Text(i18n.OverviewNoValue)
	}
	percentage := value.Shift(2).RoundBank(2)
	if percentage.IsZero() {
		return "0%"
	}
	prefix := ""
	if percentage.IsPositive() {
		prefix = "+"
	}
	return prefix + strings.Replace(percentage.String(), ".", view.DecimalSeparator, 1) + "%"
}
