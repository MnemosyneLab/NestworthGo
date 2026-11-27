package desktop

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"slices"
	"strings"
	"sync"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type overviewReader interface {
	Overview(context.Context, domain.AccountFilter) (domain.OverviewResult, error)
}

type overviewRead struct {
	value  domain.OverviewResult
	readAt time.Time
	err    error
}

type overviewPage struct {
	ctx        context.Context
	reader     overviewReader
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup
	request    <-chan overviewRead
	cancel     context.CancelFunc

	current       *overviewRead
	loading       bool
	needsRefresh  bool
	err           error
	reload        widget.Clickable
	list          widget.List
	allocation    overviewAllocation
	breakdowns    overviewBreakdowns
	trend         overviewTrend
	funds         overviewFunds
	upcoming      overviewUpcoming
	healthScan    overviewHealthScan
	firstActivity widget.Clickable
	recent        widget.Clickable
	health        widget.Clickable
	fixHealth     widget.Clickable
	accounts      widget.Clickable
	missingLinks  map[string]*widget.Clickable
	openHealth    func(healthFindingFocus, event.Tag)
	openHistory   func(bool, event.Tag)
	openLedger    func(event.Tag)
	openAccounts  func(event.Tag)
	actions       []layout.Widget
}

func newOverviewPage(ctx context.Context, reader overviewReader, clock func() time.Time, invalidate func()) *overviewPage {
	page := &overviewPage{
		ctx: ctx, reader: reader, clock: clock, invalidate: invalidate,
		list:         widget.List{List: layout.List{Axis: layout.Vertical}},
		trend:        overviewTrend{},
		missingLinks: make(map[string]*widget.Clickable),
	}
	page.trend.reader, _ = reader.(overviewTrendReader)
	page.upcoming.reader, _ = reader.(upcomingActionsReader)
	page.healthScan.reader, _ = reader.(dataHealthReader)
	return page
}

func (p *overviewPage) read() {
	if p.loading || p.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	result := make(chan overviewRead, 1)
	p.request, p.cancel, p.loading, p.err = result, cancel, true, nil
	p.workers.Go(func() {
		readAt := p.clock()
		value, err := p.reader.Overview(ctx, domain.AccountFilter{})
		if ctx.Err() != nil {
			return
		}
		result <- overviewRead{value: value, readAt: readAt, err: err}
		p.invalidate()
	})
}

func (p *overviewPage) retireRead() {
	if p.cancel != nil {
		p.cancel()
	}
	p.request, p.cancel, p.loading = nil, nil, false
}

func (p *overviewPage) refresh() {
	p.retireRead()
	p.needsRefresh = true
	p.trend.invalidate()
	p.funds.invalidate()
	p.upcoming.invalidate()
	p.healthScan.invalidate()
}

func (p *overviewPage) update() {
	select {
	case result := <-p.request:
		p.retireRead()
		p.err = result.err
		if p.err == nil {
			p.current = &result
			p.list.Position = restoredPosition(p.list.Position, p.rowCount())
		}
	default:
	}
	if !p.loading && (p.needsRefresh || p.current == nil && p.err == nil) {
		p.needsRefresh = false
		p.read()
	}
}

func (p *overviewPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.update()
	p.updateTrend(gtx, view)
	if p.err != nil && p.reload.Clicked(gtx) {
		p.read()
		p.trend.invalidate()
		p.funds.invalidate()
		p.upcoming.invalidate()
		p.healthScan.invalidate()
	}
	p.updateFunds(gtx)
	p.updateUpcoming()
	p.updateOverviewHealth()
	if p.current != nil && p.current.value.AccountCount > 0 {
		p.allocation.update(view, p.current)
	}
	p.actions = p.actions[:0]
	if p.openHealth != nil {
		if p.health.Clicked(gtx) {
			p.openHealth(healthFindingFocus{}, &p.health)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	if p.err != nil {
		p.actions = append(p.actions, returnMonthButton(view, &p.reload, i18n.HistoryRetry, nil, !p.loading))
	}
	return pageListGap(gtx, view, &p.list, view.Language.Text(i18n.NavOverview), p.rowCount(), 0, func(gtx layout.Context, row int) layout.Dimensions {
		return layout.Inset{Bottom: ui.Space16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return p.overviewRow(gtx, view, row)
		})
	})
}

// intro is the first row of the page, as in Wails: what the page shows on the
// left; the account count, data health and update time on the right, with the
// page actions beside them, over a hairline.
func (p *overviewPage) intro(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	description := view.Body(ui.TextBody, l.Text(i18n.OverviewDescription), view.Colors.SecondaryInk)
	cluster := p.statusCluster(view)
	var head layout.Widget
	if !ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, 720) {
		head = func(gtx layout.Context) layout.Dimensions {
			return (ui.Column{Gap: ui.Space12}).Layout(gtx, ui.Rigid(description), ui.Rigid(cluster))
		}
	} else {
		head = func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space16)}.Layout(gtx,
				layout.Flexed(1, description),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(620))
					return layout.E.Layout(gtx, cluster)
				}),
			)
		}
	}
	rows := []ui.ColumnChild{ui.Rigid(head)}
	if progress := p.progress(gtx, view); progress != nil {
		rows = append(rows, ui.Rigid(progress))
	}
	rows = append(rows, ui.Rigid(view.Hairline))
	return (ui.Column{Gap: ui.Space12}).Layout(gtx, rows...)
}

// statusCluster is the right side of the intro.
func (p *overviewPage) statusCluster(view ui.View) layout.Widget {
	l := view.Language
	var items []layout.Widget
	if p.current != nil {
		if count := p.current.value.AccountCount; count > 0 {
			label := fmt.Sprintf(l.Text(i18n.OverviewActiveAccounts), count)
			items = append(items, func(gtx layout.Context) layout.Dimensions { return view.Badge(gtx, ui.BadgeNeutral, label) })
		}
		if p.openHealth != nil {
			items = append(items, p.healthButton(view))
		}
		stamp := format.LocalDateTime(p.current.readAt.In(view.TimeLocation), settings.Settings{
			DateFormat: view.DateFormat, TimeFormat: view.TimeFormat,
		}, settings.Language(l))
		updated := fmt.Sprintf(l.Text(i18n.OverviewUpdatedAt), stamp)
		items = append(items, func(gtx layout.Context) layout.Dimensions {
			return view.Line(gtx, ui.TextCaption, updated, view.Colors.SecondaryInk)
		})
	}
	for _, action := range p.actions {
		items = append(items, action)
	}
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Wrap{Gap: ui.Space8, Middle: true}).Layout(gtx, len(items), func(gtx layout.Context, i int) layout.Dimensions {
			return items[i](gtx)
		})
	}
}

// healthButton opens Data health and carries the number of open input gaps.
func (p *overviewPage) healthButton(view ui.View) layout.Widget {
	label := view.Language.Text(i18n.NavDataHealth)
	title := label
	if report := p.healthScan.report; report != nil {
		if report.IssueCount > 0 {
			title = fmt.Sprintf(view.Language.Text(i18n.OverviewHealthIssues), report.IssueCount)
		} else {
			title += " · " + view.Language.Text(i18n.HealthHealthy)
			if p.healthScan.provisional {
				title += " · " + view.Language.Text(i18n.HealthProvisional)
			}
		}
	} else if p.healthScan.reader == nil && len(p.current.value.MissingInputs) > 0 {
		title = fmt.Sprintf(view.Language.Text(i18n.OverviewHealthIssues), len(p.current.value.MissingInputs))
	}
	return func(gtx layout.Context) layout.Dimensions {
		view.Focus.Watch(gtx, &p.health)
		return view.ButtonOf(&p.health, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: title, Name: title})(gtx)
	}
}

func (p *overviewPage) overviewRow(gtx layout.Context, view ui.View, row int) layout.Dimensions {
	if row == 0 {
		return p.intro(gtx, view)
	}
	result, l := p.current.value, view.Language
	composition := p.compositionIndex()
	switch {
	case result.AccountCount == 0:
		rows := []ui.ColumnChild{ui.Rigid(view.Message(l.Text(i18n.OverviewEmpty), view.Colors.SecondaryInk))}
		if p.openAccounts != nil {
			if p.accounts.Clicked(gtx) {
				p.openAccounts(&p.accounts)
				gtx.Execute(op.InvalidateCmd{})
			}
			rows = append(rows, ui.Rigid(returnMonthButton(view, &p.accounts, i18n.OverviewOpenAccounts, nil, true)))
		}
		return (ui.Column{Gap: 8}).Layout(gtx, rows...)
	case row == 1:
		return p.financialRow(gtx, view)
	case row == 2 && p.hasActionRow():
		return p.overviewActionRow(gtx, view)
	case row == composition-1 && p.hasCompositionSummary():
		return p.breakdowns.composition(gtx, view, result)
	case row == p.trendIndex() && row < composition:
		return p.overviewTrendCard(gtx, view, result.HistoryStarted)
	case row == composition:
		return p.allocation.section(gtx, view, result)
	case row == p.recentActivityIndex():
		return p.overviewRecentActivity(gtx, view, result)
	default:
		return p.breakdowns.dimensions(gtx, view, result, row-p.dimensionsIndex())
	}
}

func (p *overviewPage) rowCount() int {
	if p.current == nil {
		return 1
	}
	if p.current.value.AccountCount == 0 {
		return 2
	}
	return p.recentActivityIndex() + 1
}

func (p *overviewPage) recentActivityIndex() int {
	n := p.dimensionsIndex()
	if p.hasDimensions() {
		n += 2
	}
	return n
}

func (p *overviewPage) dimensionsIndex() int {
	return p.compositionIndex() + 1
}

func (p *overviewPage) compositionIndex() int {
	index := p.trendIndex()
	if p.trend.reader != nil || p.openHistory != nil {
		index++
	}
	if p.hasCompositionSummary() {
		index++
	}
	return index
}

func (p *overviewPage) trendIndex() int {
	return 2 + boolInt(p.hasActionRow())
}

func (p *overviewPage) hasActionRow() bool {
	return p.hasAttentionSection() || p.upcoming.reader != nil
}

func (p *overviewPage) hasAttentionSection() bool {
	return len(p.current.value.MissingInputs) > 0 || p.upcoming.checks != nil && p.upcoming.openTasks != nil ||
		p.healthScan.reader != nil && p.openHealth != nil || p.canRecordFirstActivity()
}

func (p *overviewPage) canRecordFirstActivity() bool {
	return p.openLedger != nil && p.current.value.HistoryStarted && len(p.current.value.RecentActivities) == 0
}

func (p *overviewPage) overviewActionRow(gtx layout.Context, view ui.View) layout.Dimensions {
	if !p.hasAttentionSection() {
		return p.overviewUpcomingCard(gtx, view)
	}
	if p.upcoming.reader == nil {
		return p.attention(gtx, view, p.current.value.MissingInputs)
	}
	grid := ui.Grid{MinColumn: 392, MaxColumns: 2, Gap: ui.Space16}
	return grid.Layout(gtx, view, 2, func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
		if i == 0 {
			return p.attention(gtx, view, p.current.value.MissingInputs)
		}
		return p.overviewUpcomingCard(gtx, view)
	})
}

func (p *overviewPage) hasCompositionSummary() bool {
	return len(overviewSummaries(p.current.value)) > 0
}

func (p *overviewPage) hasDimensions() bool {
	if p.current == nil || p.current.value.AccountCount == 0 {
		return false
	}
	for _, dimension := range overviewDimensions(p.current.value) {
		if len(dimension.items) > 0 {
			return true
		}
	}
	return false
}

// overviewAttentionLimit is how many open input gaps the card lists; the rest
// are one click away in Data health, which the card links to.
const overviewAttentionLimit = 3

// attention is the "Needs attention" card: the first open input gaps, each a
// link to its place in Data health, and a link to the full list.
func (p *overviewPage) attention(gtx layout.Context, view ui.View, missing []domain.MissingInputView) layout.Dimensions {
	l := view.Language
	shown := missing[:min(len(missing), overviewAttentionLimit)]
	rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextHeading, l.Text(i18n.OverviewAttentionTitle), view.Colors.Ink))}
	for i, item := range shown {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if i == 0 {
						return layout.Dimensions{}
					}
					return view.Hairline(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.overviewMissingRow(gtx, view, item) }),
			)
		}))
	}
	rows = append(rows, p.overviewHealthRows(view)...)
	count := len(missing)
	if report := p.healthScan.report; report != nil {
		count = max(count, report.IssueCount, len(p.healthScan.findings))
	}
	if p.openHealth != nil && count > 0 {
		rows = append(rows, ui.Rigid(p.fixInHealth(view, count)))
	}
	if p.canRecordFirstActivity() {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.OverviewRecordFirstActivity), view.Colors.SecondaryInk)),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				if p.firstActivity.Clicked(gtx) {
					p.openLedger(&p.firstActivity)
					gtx.Execute(op.InvalidateCmd{})
				}
				return returnMonthButton(view, &p.firstActivity, i18n.OverviewRecordOpenLedger, nil, true)(gtx)
			}))
	} else if len(rows) == 1 && p.healthScan.report != nil && p.current.value.HistoryStarted {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.OverviewNoDataActions), view.Colors.SecondaryInk)))
	}
	if p.upcoming.checks != nil && p.upcoming.openTasks != nil {
		rows = append(rows, ui.Rigid(p.overviewTaskSummary(view)))
	}
	return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
	})
}

// fixInHealth is the link at the foot of the attention card.
func (p *overviewPage) fixInHealth(view ui.View, count int) layout.Widget {
	title := view.Language.Text(i18n.OverviewFixInHealth)
	if count > overviewAttentionLimit {
		title = fmt.Sprintf("%s (%d)", title, count)
	}
	return func(gtx layout.Context) layout.Dimensions {
		view.Focus.Watch(gtx, &p.fixHealth)
		if p.fixHealth.Clicked(gtx) {
			p.openHealth(healthFindingFocus{}, &p.fixHealth)
			gtx.Execute(op.InvalidateCmd{})
		}
		return layout.W.Layout(gtx, view.ButtonOf(&p.fixHealth, ui.ButtonSpec{Variant: ui.ButtonLink, Size: ui.ButtonSmall, Title: title, Name: title}))
	}
}

func (p *overviewPage) overviewMissingRow(gtx layout.Context, view ui.View, item domain.MissingInputView) layout.Dimensions {
	label := overviewIssue(view.Language, item)
	if p.openHealth == nil {
		return view.Message(label, view.Colors.Ink)(gtx)
	}
	focus := healthFindingFocus{label: label}
	switch item.Kind {
	case domain.MissingFXRate:
		focus.currencyA, focus.currencyB = item.BaseCurrency.String(), item.QuoteCurrency.String()
	case domain.MissingAccountValue:
		focus.account = item.AccountID.String()
	default:
		if item.InstrumentID != nil {
			focus.instrument = item.InstrumentID.String()
		}
		if focus.instrument == "" {
			focus.account = item.AccountID.String()
		}
	}
	id := strings.Join([]string{string(item.Kind), focus.account, focus.instrument, focus.currencyA, focus.currencyB}, "/")
	button := portfolioButton(p.missingLinks, id)
	view.Focus.Watch(gtx, button)
	if button.Clicked(gtx) {
		p.openHealth(focus, button)
		gtx.Execute(op.InvalidateCmd{})
	}
	colors := view.Colors
	return ui.Bleed(gtx, ui.Space8, view.ButtonOf(button, ui.ButtonSpec{Name: label, Variant: ui.ButtonGhost, Fill: true, Content: func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.IconTile(gtx, lucide.CircleAlert, 32, ui.Mix(colors.Surface, colors.Warning, 0.18), colors.WarningInk)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return view.Body(ui.TextBody, label, colors.Ink)(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.LayoutGlyph(gtx, lucide.ChevronRight, 16, colors.SecondaryInk)
			}),
		)
	}}))
}

// progress is the loading or failure line under the intro, if there is one.
func (p *overviewPage) progress(gtx layout.Context, view ui.View) layout.Widget {
	l := view.Language
	switch {
	case p.loading:
		id := i18n.OverviewLoading
		if p.current != nil {
			id = i18n.OverviewRefreshing
		}
		return view.Message(l.Text(id), view.Colors.SecondaryInk)
	case p.err != nil:
		id := i18n.OverviewError
		if p.current != nil {
			id = i18n.OverviewStale
		}
		return view.Message(l.Text(id), view.Colors.ErrorInk)
	}
	return nil
}

func (p *overviewPage) totals(gtx layout.Context, view ui.View) layout.Dimensions {
	result := p.current.value
	values := result.DisplayValuation
	status, tone := i18n.OverviewCurrentEstimate, ui.BadgeSuccess
	if !result.Complete || values.NetWorth.Quality.Availability != domain.ValuationFull {
		status, tone = i18n.OverviewIncomplete, ui.BadgeWarning
	}
	l, colors := view.Language, view.Colors
	caption, hero := overviewMoneyText(view, i18n.NetWorth, values.NetWorth)
	return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (ui.Wrap{Gap: ui.Space8, Middle: true}).Layout(gtx, 2, func(gtx layout.Context, i int) layout.Dimensions {
					if i == 0 {
						return view.Body(ui.TextLabel, caption, colors.SecondaryInk)(gtx)
					}
					return view.Badge(gtx, tone, l.Text(status))
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.Line(gtx, ui.TextHero, hero, colors.Ink)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (ui.Column{Gap: ui.Space4}).Layout(gtx,
					ui.Rigid(view.Body(ui.TextCaption, fmt.Sprintf(l.Text(i18n.OverviewBaseCurrency), p.current.value.Currency), colors.SecondaryInk)),
					ui.Rigid(func(gtx layout.Context) layout.Dimensions {
						return overviewEvidence(gtx, view, values.NetWorth.Quality, i18n.OverviewQuoteDates)
					}))
			}),
			layout.Rigid(view.Hairline),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Gap: gtx.Dp(ui.Space24)}.Layout(gtx,
					layout.Flexed(1, overviewFigure(view, i18n.Assets, values.Assets, lucide.TrendingUp, colors.Success)),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						// A rule in the gutter separates the two figures, as in Wails.
						macro := op.Record(gtx.Ops)
						dims := overviewFigure(view, i18n.Liabilities, values.Liabilities, lucide.TrendingDown, colors.Destructive)(gtx)
						call := macro.Stop()
						rule := image.Rect(-gtx.Dp(ui.Space12), 0, -gtx.Dp(ui.Space12)+max(1, gtx.Dp(1)), dims.Size.Y)
						paint.FillShape(gtx.Ops, colors.Divider, clip.Rect(rule).Op())
						call.Add(gtx.Ops)
						return dims
					}),
				)
			}),
		)
	})
}

// overviewMoneyText resolves the caption and the figure of a valuation amount.
// When only a known subtotal exists the caption says so.
func overviewMoneyText(view ui.View, label i18n.MessageID, value domain.ValuationAmount) (caption, display string) {
	caption = view.Language.Text(label)
	money := value.Total
	if money == nil && value.KnownSubtotal != nil {
		money = value.KnownSubtotal
		qualifier := i18n.OverviewKnownSubtotal
		if label == i18n.NetWorth {
			qualifier = i18n.OverviewKnownNet
		}
		caption += " · " + view.Language.Text(qualifier)
	}
	display = view.Language.Text(i18n.OverviewNoValue)
	if money != nil {
		display = valuationMoney(view, *money, label == i18n.Liabilities)
	}
	return caption, display
}

// overviewFigure is a labelled secondary figure with a trend glyph. Liability
// figures use the destructive ink, as in Wails.
func overviewFigure(view ui.View, label i18n.MessageID, value domain.ValuationAmount, icon ui.Glyph, ink color.NRGBA) layout.Widget {
	caption, display := overviewMoneyText(view, label, value)
	return func(gtx layout.Context) layout.Dimensions {
		figure := ui.TextMoneyMD
		figure.Size = 20
		figureInk := view.Colors.Ink
		if label == i18n.Liabilities {
			figureInk = ink
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space4)}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space6)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ui.LayoutGlyph(gtx, icon, 14, ink) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return view.Line(gtx, ui.TextCaption, caption, view.Colors.SecondaryInk)
					}))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return view.Line(gtx, figure, display, figureInk) }),
		)
	}
}

func overviewAmount(view ui.View, label i18n.MessageID, value domain.ValuationAmount, size unit.Sp) layout.Widget {
	caption, display := overviewMoneyText(view, label, value)
	return func(gtx layout.Context) layout.Dimensions {
		label := material.Label(view.Theme, size, display)
		label.Color, label.Font.Weight = view.Colors.Ink, font.Medium
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(2)}.Layout(gtx,
			layout.Rigid(view.Message(caption, view.Colors.SecondaryInk)),
			layout.Rigid(label.Layout),
		)
	}
}

func valuationReadTime(view ui.View, instant time.Time) string {
	stamp := format.LocalDateTime(instant.In(view.TimeLocation), settings.Settings{
		DateFormat: view.DateFormat, TimeFormat: view.TimeFormat,
	}, settings.Language(view.Language))
	return fmt.Sprintf(view.Language.Text(i18n.OverviewReadAt), stamp, view.TimeLocation)
}

func valuationMoney(view ui.View, money domain.MoneyView, liability bool) string {
	amount := money.Amount
	// Debt magnitudes are unsigned in the service. Match Wails' visual sign.
	if liability && amount != "0" && !strings.HasPrefix(amount, "-") {
		amount = "-" + amount
	}
	return format.Money(amount, money.Currency.String(), settings.Settings{
		DecimalSeparator: view.DecimalSeparator, GroupingSeparator: view.GroupingSeparator,
	})
}

func overviewEvidence(gtx layout.Context, view ui.View, quality domain.ValuationQuality, referenceDates i18n.MessageID) layout.Dimensions {
	var rows []layout.FlexChild
	l := view.Language
	if quality.OldestMarketDate != nil {
		localDay := func(value string) string {
			if day, ok := ui.ParseCivilDay(value); ok {
				return ui.FormatDay(day, l, view.DateFormat)
			}
			return value
		}
		dates := localDay(*quality.OldestMarketDate)
		if quality.NewestMarketDate != nil && *quality.NewestMarketDate != *quality.OldestMarketDate {
			dates += " – " + localDay(*quality.NewestMarketDate)
		}
		rows = append(rows, layout.Rigid(view.Body(ui.TextCaption, fmt.Sprintf(l.Text(referenceDates), dates), view.Colors.SecondaryInk)))
	}
	if quality.CarriedCount > 0 || quality.StaleCount > 0 {
		rows = append(rows, layout.Rigid(view.Body(ui.TextCaption, l.Text(i18n.OverviewCarried), view.Colors.SecondaryInk)))
	}
	if slices.Contains(quality.Reasons, domain.ReasonHistoryUnverified) {
		rows = append(rows, layout.Rigid(view.Body(ui.TextCaption, l.Text(i18n.OverviewHistoryUnverified), view.Colors.SecondaryInk)))
	}
	if slices.Contains(quality.Reasons, domain.ReasonEvidenceUnavailable) {
		rows = append(rows, layout.Rigid(view.Body(ui.TextCaption, l.Text(i18n.OverviewEvidenceUnavailable), view.Colors.SecondaryInk)))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx, rows...)
}

var overviewMissing = map[domain.MissingInputKind]i18n.MessageID{
	domain.MissingInstrumentPrice: i18n.OverviewMissingPrice,
	domain.MissingInstrument:      i18n.OverviewMissingInstrument,
	domain.MissingFXRate:          i18n.OverviewMissingFX,
	domain.MissingAccountValue:    i18n.OverviewMissingAccount,
	domain.MissingHistoryCoverage: i18n.OverviewHistoryUnverified,
	domain.MissingHoldingAmount:   i18n.OverviewEvidenceUnavailable,
}

func overviewIssue(language i18n.Language, missing domain.MissingInputView) string {
	message, ok := overviewMissing[missing.Kind]
	if !ok {
		message = i18n.OverviewEvidenceUnavailable
	}
	parts := []string{language.Text(message)}
	if missing.Kind == domain.MissingFXRate && missing.BaseCurrency != "" && missing.QuoteCurrency != "" {
		parts = append(parts, missing.BaseCurrency.String()+"/"+missing.QuoteCurrency.String())
	}
	if name := domain.InstrumentDisplayLabel(missing.InstrumentName, missing.InstrumentSymbol, missing.InstrumentShortName); name != "" {
		parts = append(parts, name)
	}
	if missing.AccountName != "" {
		parts = append(parts, missing.AccountName)
	}
	return strings.Join(parts, " · ")
}
