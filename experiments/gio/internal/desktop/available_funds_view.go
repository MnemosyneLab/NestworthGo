package desktop

import (
	"fmt"
	"image"
	"slices"
	"strings"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
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
	customDate       ui.DatePicker
	early            widget.Bool
	comparisonAbout  widget.Clickable
	comparisonOrigin image.Point
	horizons         [4]widget.Clickable
	filters          [fundsFilterCount]widget.Clickable
	missingRules     widget.Clickable
	needsInfo        widget.Clickable
	sourceHelp       widget.Clickable
	showAll          widget.Clickable
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

func (p *availableFundsPage) layoutFundsControls(gtx layout.Context, view ui.View, asOf string) layout.Dimensions {
	options := ui.DateOptions{Today: p.localDate, Min: p.localDate, AllowFuture: true, Clearable: true, Format: view.DateFormat, WeekStart: time.Monday}
	if view.WeekStart == settings.WeekStartSunday {
		options.WeekStart = time.Sunday
	}
	const horizontal, vertical = ui.Space16, ui.Space12
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: horizontal, Right: horizontal, Top: vertical, Bottom: vertical}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			view.Origin = view.Origin.Add(image.Pt(gtx.Dp(horizontal), gtx.Dp(vertical)))
			return p.fundsControlsRow(gtx, view, asOf, options)
		})
	})
}

func (p *availableFundsPage) fundsControlsRow(gtx layout.Context, view ui.View, asOf string, options ui.DateOptions) layout.Dimensions {
	width, gap, height := gtx.Constraints.Max.X, gtx.Dp(ui.Space12), gtx.Dp(ui.ControlDefault)
	l := view.Language
	dateLabel, earlyLabel := l.Text(i18n.FundsCustomDate), l.Text(i18n.FundsConsiderEarly)
	captionWidth := marketTextWidth(gtx, view, ui.TextLabel, dateLabel)
	dateWidth := min(width, captionWidth+gap+gtx.Dp(176))
	earlyWidth := min(width, marketTextWidth(gtx, view, ui.TextLabel, earlyLabel)+gtx.Dp(48))
	metadataWidth := marketTextWidth(gtx, view, ui.TextBody, asOf) + gtx.Dp(24)
	controlsWidth := dateWidth + gap + earlyWidth
	dateX, dateY := max(0, width-controlsWidth), 0
	if width < metadataWidth+gap+controlsWidth {
		dateX = 0
	}
	metadata := ui.Place(gtx, view, image.Point{}, max(0, min(width, max(dateX-gap, metadataWidth))), func(gtx layout.Context, view ui.View) layout.Dimensions {
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return layout.Flex{Gap: gtx.Dp(ui.Space8), Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.LayoutGlyph(gtx, lucide.CalendarClock, 16, view.Colors.SecondaryInk)
			}), layout.Flexed(1, view.Message(asOf, view.Colors.SecondaryInk)))
	})
	if dateX == 0 {
		dateY = metadata.Size.Y + gtx.Dp(ui.Space8)
	}
	date := ui.Place(gtx, view, image.Pt(dateX, dateY), dateWidth, func(gtx layout.Context, view ui.View) layout.Dimensions {
		labelWidth := min(captionWidth, gtx.Constraints.Max.X/2)
		label := ui.Place(gtx, view, image.Point{}, labelWidth, func(gtx layout.Context, view ui.View) layout.Dimensions {
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
			return layout.W.Layout(gtx, view.Body(ui.TextLabel, dateLabel, view.Colors.Ink))
		})
		x := labelWidth + gtx.Dp(ui.Space8)
		ui.Place(gtx, view, image.Pt(x, 0), max(0, gtx.Constraints.Max.X-x), func(gtx layout.Context, view ui.View) layout.Dimensions {
			p.controls.customDate.Layout(gtx, view, ui.PickerField{
				Label: i18n.FundsCustomDate, Glyph: lucide.Calendar, Origin: view.Origin, HideLabel: true,
				Next: &p.controls.early,
			}, options, p.state.customOn, func(value string) {
				if p.state.customOn != value {
					p.state.customOn, p.state.horizonOn = value, value
					p.list.Position = layout.Position{}
					p.read()
					gtx.Execute(op.InvalidateCmd{})
				}
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
		})
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, max(height, label.Size.Y))}
	})
	earlyX, earlyY := dateX+dateWidth+gap, dateY
	if controlsWidth > width {
		earlyX, earlyY = 0, dateY+date.Size.Y+gtx.Dp(ui.Space8)
	}
	early := ui.Place(gtx, view, image.Pt(earlyX, earlyY), earlyWidth, func(gtx layout.Context, view ui.View) layout.Dimensions {
		if p.request != nil {
			gtx = gtx.Disabled()
		}
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: ui.Space6, Right: ui.Space6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dims := layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return accessibility.Checkbox(gtx, &p.controls.early, earlyLabel, view.Checkbox(&p.controls.early, earlyLabel))
				})
				if gtx.Enabled() && p.state.includeEarly != p.controls.early.Value {
					p.state.includeEarly = p.controls.early.Value
					p.read()
					gtx.Execute(op.InvalidateCmd{})
				}
				return dims
			})
		})
	})
	return layout.Dimensions{Size: image.Pt(width, max(height, metadata.Size.Y, dateY+date.Size.Y, earlyY+early.Size.Y))}
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
			layout.Rigid(p.fundsComparisonHeading(view)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (ui.Grid{MinColumn: 208, MaxColumns: len(horizons), Gap: ui.Space12}).Layout(gtx, view, len(horizons), func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
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
		)
	}
}

func (p *availableFundsPage) fundsComparisonHeading(view ui.View) layout.Widget {
	l := view.Language
	baseCurrency := p.current.report.Overview.BaseCurrency.String()
	return func(gtx layout.Context) layout.Dimensions {
		gap := gtx.Dp(ui.Space8)
		return layout.Flex{Gap: gap, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				var titleWidth, currencyWidth int
				return layout.Flex{Gap: gap, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						dims := view.Body(ui.TextHeading, l.Text(i18n.NavAvailableFunds), view.Colors.Ink)(gtx)
						titleWidth = dims.Size.X
						return dims
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						dims := view.Badge(gtx, ui.BadgeNeutral, baseCurrency)
						currencyWidth = dims.Size.X
						return dims
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						button := &p.controls.comparisonAbout
						if gtx.Enabled() && view.Overlay != nil {
							anchor := p.controls.comparisonOrigin.Add(image.Pt(titleWidth+currencyWidth+2*gap, gtx.Dp(ui.Space2)))
							size := gtx.Dp(28)
							view.WithScrollAnchors(&p.list).RegisterPopover(gtx, button, image.Rectangle{Min: anchor, Max: anchor.Add(image.Pt(size, size))}, ui.Popover{
								Body: func(gtx layout.Context) layout.Dimensions {
									return (ui.Column{Gap: ui.Space8}).Layout(gtx,
										ui.Rigid(view.Message(l.Text(i18n.FundsComparisonNote), view.Colors.Ink)),
										ui.Rigid(view.Message(l.Text(i18n.FundsEarlyInfo), view.Colors.SecondaryInk)))
								},
							})
						}
						return view.ButtonOf(button, ui.ButtonSpec{
							Variant: ui.ButtonGhost, Size: ui.ButtonCompact, Icon: lucide.Info, Name: l.Text(i18n.FundsComparisonAbout),
						})(gtx)
					}),
				)
			}), layout.Rigid(p.fundsQuickActions(view)))
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
	amount := fundsComparisonMoney(view, value)
	date := historyDay(view, bucket.HorizonOn)
	partial := bucket.FullUnreserved == nil && value != nil
	description := []string{date, amount, l.Text(hint)}
	if partial {
		description = append(description, l.Text(i18n.FundsPartialShort))
	}
	if early {
		description = append(description, l.Text(i18n.FundsEarlyIncluded))
	}
	if cue != "" {
		description = append(description, cue)
	}
	variant := ui.ButtonOutline
	if selected {
		variant = ui.ButtonSoft
	}
	return view.ButtonOf(button, ui.ButtonSpec{Variant: variant, Fill: true, Toggle: true, Selected: selected,
		Name: label, Description: strings.Join(description, " · "),
		Content: func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Gap: gtx.Dp(ui.Space8), Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, view.Body(ui.TextLabel, label, colors.Ink)),
						layout.Rigid(fundsHorizonIndicator(view, selected)))
				}),
				layout.Rigid(view.Body(ui.TextCaption, date, colors.SecondaryInk)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: ui.Space4}.Layout(gtx, view.Body(ui.TextMetricLG, amount, colors.Ink))
				}),
				layout.Rigid(view.Body(ui.TextCaption, l.Text(hint), colors.SecondaryInk)),
			}
			var markers []layout.Widget
			if partial {
				markers = append(markers, func(gtx layout.Context) layout.Dimensions {
					return view.Badge(gtx, ui.BadgeWarning, l.Text(i18n.FundsPartialShort))
				})
			}
			if early {
				markers = append(markers, func(gtx layout.Context) layout.Dimensions {
					return view.Badge(gtx, ui.BadgeWarning, l.Text(i18n.FundsEarlyIncluded))
				})
			}
			if cue != "" {
				markers = append(markers, view.Body(ui.TextCaption, cue, colors.SecondaryInk))
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := (ui.Wrap{Gap: ui.Space4, Middle: true}).Layout(gtx, len(markers), func(gtx layout.Context, i int) layout.Dimensions {
					return markers[i](gtx)
				})
				dims.Size.Y = max(dims.Size.Y, gtx.Dp(20))
				return dims
			}))
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(3)}.Layout(gtx, children...)
		},
	})(gtx)
}

func fundsHorizonIndicator(view ui.View, selected bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(20)
		bounds := image.Rect(0, 0, size, size)
		if selected {
			ui.FillRoundRect(gtx, bounds, ui.RadiusFull, view.Colors.Accent)
			defer op.Offset(image.Pt(gtx.Dp(3), gtx.Dp(3))).Push(gtx.Ops).Pop()
			ui.LayoutGlyph(gtx, lucide.Check, 14, view.Colors.OnAccent)
		} else {
			ui.StrokeRoundRect(gtx, bounds, ui.RadiusFull, 1, view.Colors.Divider)
		}
		return layout.Dimensions{Size: bounds.Size()}
	}
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

func (p *availableFundsPage) fundsSourcesHeader(view ui.View, bucket *domain.LiquidityBucket, counts [fundsFilterCount]int, focusAfterJump bool) layout.Widget {
	l := view.Language
	allExcluded := counts[fundsExcluded] == counts[fundsAll]
	allLocked := !allExcluded && !slices.ContainsFunc(p.current.report.Sources, func(row application.LiquidityReadSource) bool {
		result := fundsResultForHorizon(row.Result, bucket.HorizonOn)
		return !row.Result.Excluded && (result == nil || result.Status != domain.StatusComplete || result.SelectedRoute != nil)
	})
	title := fmt.Sprintf(l.Text(i18n.FundsSourcesByDate), historyDay(view, bucket.HorizonOn))
	return func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			items := []layout.FlexChild{layout.Flexed(1, p.fundsSourceHeading(view, title, bucket.Warnings))}
			if p.reservationLedger != nil {
				items = append(items, layout.Rigid(p.globalReservationButton(view, &p.manageReservations)))
			}
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, items...)
		})}
		switch {
		case allExcluded:
			children = append(children, layout.Rigid(view.Message(l.Text(i18n.FundsAllExcluded), view.Colors.SecondaryInk)))
		case allLocked:
			children = append(children, layout.Rigid(view.Message(l.Text(i18n.FundsAllLocked), view.Colors.SecondaryInk)))
		}
		if bucket.Status != domain.StatusComplete && !allExcluded {
			children = append(children, layout.Rigid(p.fundsNeedsInfoBanner(view, counts[fundsNeedsInfo])))
		}
		children = append(children, layout.Rigid(p.fundsSourceFilters(view, counts, focusAfterJump)))
		if counts[p.state.filter] > 0 && wideFundsSources(gtx) {
			children = append(children, layout.Rigid(fundsSourceHeadings(view)))
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, children...)
	}
}

func (p *availableFundsPage) fundsSourceHeading(view ui.View, title string, warnings []string) layout.Widget {
	l := view.Language
	label := l.Text(i18n.FundsSourceAmountsHelp)
	trigger := &p.controls.sourceHelp
	return func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(ui.ControlCompact)
		gap := gtx.Dp(ui.Space6)
		var titleSize image.Point
		return layout.Flex{Alignment: layout.Middle, Gap: gap}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = max(0, gtx.Constraints.Max.X-size-gap)
				dims := view.Body(ui.TextHeading, title, view.Colors.Ink)(gtx)
				titleSize = dims.Size
				return dims
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				origin := p.rowOrigin.Add(image.Pt(titleSize.X+gap, max(0, (titleSize.Y-size)/2)))
				bounds := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(size, size))}
				if view.Overlay != nil && gtx.Enabled() {
					view.RegisterPopover(gtx, trigger, bounds, ui.Popover{Body: func(gtx layout.Context) layout.Dimensions {
						items := []ui.ColumnChild{ui.Rigid(view.Message(l.Text(i18n.FundsSourceNativeCurrency), view.Colors.Ink))}
						for _, warning := range warnings {
							items = append(items, ui.Rigid(view.Message(l.Text(fundsReason(warning)), view.Colors.SecondaryInk)))
						}
						return (ui.Column{Gap: ui.Space8}).Layout(gtx, items...)
					}})
				}
				view.Focus.Watch(gtx, trigger)
				return view.ButtonOf(trigger, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonCompact,
					Icon: lucide.Info, Name: label, ShortName: label})(gtx)
			}),
		)
	}
}

func (p *availableFundsPage) fundsNeedsInfoBanner(view ui.View, count int) layout.Widget {
	l := view.Language
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Min}
			ui.FillRoundRect(gtx, bounds, ui.RadiusLG, ui.Mix(view.Colors.Surface, view.Colors.Warning, 0.12))
			ui.StrokeRoundRect(gtx, bounds, ui.RadiusLG, 1, ui.Mix(view.Colors.Divider, view.Colors.Warning, 0.42))
			return layout.Dimensions{Size: bounds.Size()}
		}, func(gtx layout.Context) layout.Dimensions {
			return (layout.Inset{Top: ui.Space8, Bottom: ui.Space8, Left: ui.Space12, Right: ui.Space12}).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				icon := func(gtx layout.Context) layout.Dimensions {
					return ui.LayoutGlyph(gtx, lucide.TriangleAlert, 16, view.Colors.WarningInk)
				}
				children := []layout.FlexChild{layout.Rigid(icon),
					layout.Flexed(1, view.Body(ui.TextLabel, l.Text(i18n.FundsSourcesNeedInfo), view.Colors.WarningInk))}
				if p.state.filter != fundsNeedsInfo && count > 0 {
					button := &p.controls.needsInfo
					if button.Clicked(gtx) {
						p.state.filter, p.state.focusSources = fundsNeedsInfo, true
						gtx.Execute(op.InvalidateCmd{})
					}
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						view.Focus.Watch(gtx, button)
						return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall,
							Title: l.Text(i18n.FundsShowNeedsInfo)})(gtx)
					}))
				}
				return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, children...)
			})
		})
	}
}

func (p *availableFundsPage) fundsSourceFilters(view ui.View, counts [fundsFilterCount]int, focusAfterJump bool) layout.Widget {
	l := view.Language
	return func(gtx layout.Context) layout.Dimensions {
		filters := []fundsSourceFilter{fundsAll}
		for filter := fundsAvailable; filter < fundsFilterCount; filter++ {
			if filter == p.state.filter || counts[filter] > 0 {
				filters = append(filters, filter)
			}
		}
		return view.TabTrack(gtx, len(filters), func(gtx layout.Context, index int) layout.Dimensions {
			filter := filters[index]
			button := &p.controls.filters[filter]
			if button.Clicked(gtx) && p.state.filter != filter {
				p.state.filter = filter
				gtx.Execute(op.InvalidateCmd{})
			}
			selected := p.state.filter == filter
			// A click during list layout is applied by the next frame's list
			// jump. A measured but clipped source header cannot consume it.
			if gtx.Enabled() && selected && focusAfterJump && p.state.focusSources {
				gtx.Execute(key.FocusCmd{Tag: button})
				p.state.focusSources = false
			}
			view.Focus.Watch(gtx, button)
			label := fmt.Sprintf("%s %d", l.Text(fundsFilterLabels[filter]), counts[filter])
			return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonSegment, Size: ui.ButtonSmall,
				Title: label, Toggle: true, Selected: selected})(gtx)
		})
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
