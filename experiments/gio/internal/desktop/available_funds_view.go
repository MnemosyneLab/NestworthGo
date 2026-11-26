package desktop

import (
	"fmt"
	"slices"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type fundsSourceFilter uint8

const (
	fundsAll fundsSourceFilter = iota
	fundsAvailable
	fundsLocked
	fundsNeedsRules
	fundsNeedsInfo
	fundsExcluded
	fundsFilterCount
)

var fundsFilterLabels = [fundsFilterCount]i18n.MessageID{
	i18n.FundsFilterAll, i18n.FundsFilterAvailable, i18n.FundsFilterLocked,
	i18n.FundsFilterNeedsRules, i18n.FundsFilterNeedsInfo, i18n.FundsFilterExcluded,
}

type fundsViewState struct {
	horizonOn, customOn        string
	includeEarly, focusSources bool
	filter                     fundsSourceFilter
}

func (s fundsViewState) query() application.LiquidityOverviewQuery {
	query := application.LiquidityOverviewQuery{IncludeEarlyWithdrawal: s.includeEarly}
	if s.customOn != "" {
		query.CustomHorizonOn = &s.customOn
	}
	return query
}

type fundsControls struct {
	customDate   ui.DatePicker
	early        widget.Bool
	horizons     [4]widget.Clickable
	filters      [fundsFilterCount]widget.Clickable
	missingRules widget.Clickable
	needsInfo    widget.Clickable
	showAll      widget.Clickable
}

func fundsPresetDay(day string, days int) string {
	result, _ := domain.AddCalendarDays(day, days)
	return result
}

func fundsBucketForHorizon(buckets []domain.LiquidityBucket, day string) *domain.LiquidityBucket {
	i := slices.IndexFunc(buckets, func(bucket domain.LiquidityBucket) bool { return bucket.HorizonOn == day })
	if i < 0 {
		return nil
	}
	return &buckets[i]
}

func fundsResultForHorizon(source domain.LiquiditySourceResult, day string) *domain.LiquidityBucketResult {
	i := slices.IndexFunc(source.BucketResults, func(result domain.LiquidityBucketResult) bool { return result.HorizonOn == day })
	if i < 0 {
		return nil
	}
	return &source.BucketResults[i]
}

// The filters mirror the Wails source filters for the selected horizon.
func fundsMatchesSourceFilter(source domain.LiquiditySourceResult, day string, filter fundsSourceFilter) bool {
	switch filter {
	case fundsAll:
		return true
	case fundsExcluded:
		return source.Excluded
	}
	if source.Excluded {
		return false
	}
	result := fundsResultForHorizon(source, day)
	needsInfo := len(source.Reasons) != 0 || result != nil && (result.Status == domain.StatusPartial || result.Status == domain.StatusUnavailable)
	switch filter {
	case fundsAvailable:
		return result != nil && result.SelectedRoute != nil && result.Status != domain.StatusUnavailable
	case fundsLocked:
		return !source.DueUnconfirmed && !needsInfo && result != nil && result.Status == domain.StatusComplete && result.SelectedRoute == nil
	case fundsNeedsRules:
		return result != nil && result.NeedsRules
	case fundsNeedsInfo:
		return needsInfo
	default:
		return false
	}
}

func fundsSourceCount(sources []application.LiquidityReadSource, day string, filter fundsSourceFilter) int {
	count := 0
	for _, source := range sources {
		if fundsMatchesSourceFilter(source.Result, day, filter) {
			count++
		}
	}
	return count
}

func fundsMissingRuleText(language i18n.Language, count int) string {
	id := i18n.FundsMissingRuleCountMany
	if count == 1 {
		id = i18n.FundsMissingRuleCountOne
	}
	return fmt.Sprintf(language.Text(id), count)
}

func countFundsSources(sources []application.LiquidityReadSource, day string) (counts [fundsFilterCount]int) {
	for filter := range counts {
		counts[filter] = fundsSourceCount(sources, day, fundsSourceFilter(filter))
	}
	return counts
}

func (p *availableFundsPage) selectedFundsBucket() *domain.LiquidityBucket {
	if selected := fundsBucketForHorizon(p.current.report.Overview.Buckets, p.state.horizonOn); selected != nil {
		return selected
	}
	return &p.current.today
}

func (p *availableFundsPage) layoutFundsControls(gtx layout.Context, view ui.View) layout.Dimensions {
	options := ui.DateOptions{Today: p.localDate, Min: p.localDate, AllowFuture: true, Clearable: true, Format: view.DateFormat, WeekStart: time.Monday}
	if view.WeekStart == settings.WeekStartSunday {
		options.WeekStart = time.Sunday
	}
	return (ui.Grid{MinColumn: 172, MaxColumns: 2, Gap: ui.Space12}).Layout(gtx, view, 2, func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
		if index == 0 {
			return analysisDate(&p.controls.customDate, i18n.FundsCustomDate, options, p.state.customOn, func(value string) {
				if p.state.customOn != value {
					p.state.customOn, p.state.horizonOn = value, value
					p.list.Position = layout.Position{}
					p.read()
					gtx.Execute(op.InvalidateCmd{})
				}
			})(gtx, view)
		}
		if p.request != nil {
			gtx = gtx.Disabled()
		}
		return layout.Inset{Top: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := view.Checkbox(&p.controls.early, view.Language.Text(i18n.FundsConsiderEarly))(gtx)
				if gtx.Enabled() && p.state.includeEarly != p.controls.early.Value {
					p.state.includeEarly = p.controls.early.Value
					p.read()
					gtx.Execute(op.InvalidateCmd{})
				}
				return dims
			})}
			if p.state.includeEarly {
				children = append(children, layout.Rigid(view.Body(ui.TextCaption, view.Language.Text(i18n.FundsEarlyInfo), view.Colors.SecondaryInk)))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space4)}.Layout(gtx, children...)
		})
	})
}

type fundsHorizon struct {
	bucket *domain.LiquidityBucket
	label  i18n.MessageID
}

func (p *availableFundsPage) fundsHorizons() []fundsHorizon {
	overview := p.current.report.Overview
	var options []fundsHorizon
	for i, bucket := range overview.Buckets {
		if i < 3 {
			label := [...]i18n.MessageID{i18n.FundsHorizonToday, i18n.FundsHorizonWeek, i18n.FundsHorizonMonth}[i]
			options = append(options, fundsHorizon{bucket: &overview.Buckets[i], label: label})
		} else if bucket.HorizonOn == p.state.customOn {
			options = append(options, fundsHorizon{bucket: &overview.Buckets[i], label: i18n.FundsCustomDate})
		}
	}
	return options
}

func (p *availableFundsPage) fundsComparison(view ui.View, selected string) layout.Widget {
	l := view.Language
	horizons := p.fundsHorizons()
	sources := p.current.report.Sources
	missing := fundsSourceCount(sources, selected, fundsNeedsRules)
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
			layout.Rigid(view.Body(ui.TextHeading, l.Text(i18n.NavAvailableFunds)+" · "+p.current.report.Overview.BaseCurrency.String(), view.Colors.Ink)),
			layout.Rigid(p.fundsQuickActions(view)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (ui.Grid{MinColumn: 126, MaxColumns: 4, Gap: ui.Space8, Balanced: true}).Layout(gtx, view, len(horizons), func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
					horizon := horizons[index]
					button := &p.controls.horizons[index]
					if button.Clicked(gtx) && p.state.horizonOn != horizon.bucket.HorizonOn {
						p.state.horizonOn = horizon.bucket.HorizonOn
						gtx.Execute(op.InvalidateCmd{})
					}
					cue := fundsHorizonCue(l, horizons, horizon)
					return fundsHorizonButton(gtx, view, button, horizon, horizon.bucket.HorizonOn == selected, fundsHorizonUsesEarly(sources, horizon.bucket.HorizonOn), cue)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if missing == 0 {
					return view.Message(l.Text(i18n.FundsNoMissingRules), view.Colors.SecondaryInk)(gtx)
				}
				if p.controls.missingRules.Clicked(gtx) {
					p.state.filter, p.state.focusSources = fundsNeedsRules, true
					gtx.Execute(op.InvalidateCmd{})
				}
				label := fundsMissingRuleText(l, missing)
				return view.ButtonOf(&p.controls.missingRules, ui.ButtonSpec{Variant: ui.ButtonLink, Size: ui.ButtonSmall, Title: label})(gtx)
			}),
			layout.Rigid(view.Message(l.Text(i18n.FundsComparisonNote), view.Colors.SecondaryInk)),
		)
	}
}

func fundsHorizonCue(language i18n.Language, horizons []fundsHorizon, horizon fundsHorizon) string {
	if horizon.bucket.FullUnreserved == nil {
		return ""
	}
	var previous fundsHorizon
	for _, entry := range horizons {
		if entry.bucket.HorizonOn < horizon.bucket.HorizonOn && (previous.bucket == nil || entry.bucket.HorizonOn > previous.bucket.HorizonOn) {
			previous = entry
		}
	}
	if previous.bucket == nil || previous.bucket.FullUnreserved == nil {
		return ""
	}
	id := i18n.FundsSameAs
	switch horizon.bucket.FullUnreserved.Amount().Cmp(previous.bucket.FullUnreserved.Amount()) {
	case -1:
		id = i18n.FundsLessThan
	case 1:
		id = i18n.FundsMoreThan
	}
	return fmt.Sprintf(language.Text(id), language.Text(previous.label))
}

func fundsHorizonButton(gtx layout.Context, view ui.View, button *widget.Clickable, horizon fundsHorizon, selected, early bool, cue string) layout.Dimensions {
	l, colors := view.Language, view.Colors
	label := l.Text(horizon.label)
	bucket := horizon.bucket
	value, hint := bucket.FullUnreserved, i18n.FundsSpendableHint
	if value == nil {
		value, hint = bucket.KnownUnreservedSubtotal, i18n.FundsSubtotalHint
		if value == nil {
			hint = i18n.FundsUnknownHint
		}
	}
	amount := l.Text(i18n.OverviewFundsUnknownAmount)
	if value != nil {
		amount = fundsAmountText(view, value)
	}
	date := historyDay(view, bucket.HorizonOn)
	variant := ui.ButtonOutline
	if selected {
		variant = ui.ButtonSoft
	}
	return view.ButtonOf(button, ui.ButtonSpec{Variant: variant, Fill: true, Toggle: true, Selected: selected,
		Name: label, Description: date + " · " + amount + " · " + l.Text(hint),
		Content: func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(view.Body(ui.TextLabel, label, colors.Ink)),
				layout.Rigid(view.Body(ui.TextCaption, date, colors.SecondaryInk)),
				layout.Rigid(view.Body(ui.TextMoneyMD, amount, colors.Ink)),
				layout.Rigid(view.Body(ui.TextCaption, l.Text(hint), colors.SecondaryInk)),
			}
			if early {
				children = append(children, layout.Rigid(view.Body(ui.TextCaption, l.Text(i18n.FundsEarlyIncluded), colors.SecondaryInk)))
			}
			if cue != "" {
				children = append(children, layout.Rigid(view.Body(ui.TextCaption, cue, colors.SecondaryInk)))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(3)}.Layout(gtx, children...)
		},
	})(gtx)
}

func fundsHorizonUsesEarly(sources []application.LiquidityReadSource, day string) bool {
	return slices.ContainsFunc(sources, func(source application.LiquidityReadSource) bool {
		result := fundsResultForHorizon(source.Result, day)
		return !source.Result.Excluded && result != nil && result.SelectedRoute != nil && result.SelectedRoute.Kind == domain.RouteEarly
	})
}

func (p *availableFundsPage) fundsQuickActions(view ui.View) layout.Widget {
	button := view.ButtonOf(&p.jumpSources, ui.ButtonSpec{
		Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: view.Language.Text(i18n.FundsSources),
	})
	return func(gtx layout.Context) layout.Dimensions { return layout.W.Layout(gtx, button) }
}

func (p *availableFundsPage) fundsSourcesHeader(view ui.View, bucket *domain.LiquidityBucket, counts [fundsFilterCount]int) layout.Widget {
	l := view.Language
	allLocked := counts[fundsExcluded] < counts[fundsAll] && !slices.ContainsFunc(p.current.report.Sources, func(row application.LiquidityReadSource) bool {
		result := fundsResultForHorizon(row.Result, bucket.HorizonOn)
		return !row.Result.Excluded && (result == nil || result.Status != domain.StatusComplete || result.SelectedRoute != nil)
	})
	return func(gtx layout.Context) layout.Dimensions {
		filters := []fundsSourceFilter{fundsAll}
		for filter := fundsAvailable; filter < fundsFilterCount; filter++ {
			if filter == p.state.filter || counts[filter] > 0 {
				filters = append(filters, filter)
			}
		}
		title := fmt.Sprintf(l.Text(i18n.FundsSourcesByDate), historyDay(view, bucket.HorizonOn))
		header := []layout.Widget{view.Body(ui.TextHeading, title, view.Colors.Ink)}
		if p.reservationLedger != nil {
			header = append(header, p.globalReservationButton(view, &p.manageReservations))
		}
		children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: ui.Space12, Middle: true}).Layout(gtx, len(header), func(gtx layout.Context, i int) layout.Dimensions {
				return header[i](gtx)
			})
		})}
		switch {
		case counts[fundsExcluded] == counts[fundsAll]:
			children = append(children, layout.Rigid(view.Message(l.Text(i18n.FundsAllExcluded), view.Colors.SecondaryInk)))
		case allLocked:
			children = append(children, layout.Rigid(view.Message(l.Text(i18n.FundsAllLocked), view.Colors.SecondaryInk)))
		}
		if bucket.Status != domain.StatusComplete && counts[fundsExcluded] < counts[fundsAll] {
			children = append(children, layout.Rigid(view.Message(l.Text(i18n.FundsSourcesNeedInfo), view.Colors.SecondaryInk)))
			if p.state.filter != fundsNeedsInfo && counts[fundsNeedsInfo] > 0 {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.controls.needsInfo.Clicked(gtx) {
						p.state.filter, p.state.focusSources = fundsNeedsInfo, true
						gtx.Execute(op.InvalidateCmd{})
					}
					return view.ButtonOf(&p.controls.needsInfo, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: l.Text(i18n.FundsShowNeedsInfo)})(gtx)
				}))
			}
		}
		for _, warning := range bucket.Warnings {
			children = append(children, layout.Rigid(view.Message(l.Text(fundsReason(warning)), view.Colors.SecondaryInk)))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.TabTrack(gtx, len(filters), func(gtx layout.Context, i int) layout.Dimensions {
				filter := filters[i]
				button := &p.controls.filters[filter]
				if button.Clicked(gtx) && p.state.filter != filter {
					p.state.filter = filter
					gtx.Execute(op.InvalidateCmd{})
				}
				selected := p.state.filter == filter
				if gtx.Enabled() && selected && p.state.focusSources {
					gtx.Execute(key.FocusCmd{Tag: button})
					p.state.focusSources = false
				}
				label := fmt.Sprintf("%s %d", l.Text(fundsFilterLabels[filter]), counts[filter])
				return view.TabSegment(button, label, selected)(gtx)
			})
		}))
		if counts[p.state.filter] > 0 && wideFundsSources(gtx) {
			children = append(children, layout.Rigid(fundsSourceHeadings(view)))
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, children...)
	}
}

func (p *availableFundsPage) noMatchingFundsSources(view ui.View) layout.Widget {
	l := view.Language
	return func(gtx layout.Context) layout.Dimensions {
		if p.controls.showAll.Clicked(gtx) {
			p.state.filter, p.state.focusSources = fundsAll, true
			gtx.Execute(op.InvalidateCmd{})
		}
		return fundsCard(view, l.Text(i18n.FundsNoMatchingSources), view.Button(&p.controls.showAll, l.Text(i18n.FundsShowAllSources)))(gtx)
	}
}
