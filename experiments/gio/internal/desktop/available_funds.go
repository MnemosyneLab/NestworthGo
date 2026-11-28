package desktop

import (
	"context"
	"fmt"
	"image"
	"slices"
	"sync"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type availableFundsReader interface {
	WithRead(context.Context, func(context.Context) error) error
	LiquidityRead(context.Context, application.LiquidityOverviewQuery) (application.LiquidityReadResult, error)
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
}

type availableFundsRead struct {
	report   application.LiquidityReadResult
	today    domain.LiquidityBucket
	accounts map[domain.AccountID]domain.Account
	native   map[string]map[domain.CurrencyCode]application.LiquidityNativeAmounts
}

func readAvailableFunds(ctx context.Context, service availableFundsReader) (availableFundsRead, error) {
	return readAvailableFundsQuery(ctx, service, application.LiquidityOverviewQuery{})
}

func readAvailableFundsQuery(ctx context.Context, service availableFundsReader, query application.LiquidityOverviewQuery) (out availableFundsRead, err error) {
	err = service.WithRead(ctx, func(ctx context.Context) error {
		report, err := service.LiquidityRead(ctx, query)
		if err != nil {
			return err
		}
		today := fundsBucketForHorizon(report.Overview.Buckets, report.Overview.LocalDate)
		if today == nil {
			return &domain.Error{Code: domain.ErrIntegrity}
		}
		accounts, err := service.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
		if err != nil {
			return err
		}
		out = availableFundsRead{report: report, today: *today, accounts: make(map[domain.AccountID]domain.Account, len(accounts))}
		for _, row := range accounts {
			out.accounts[row.Account.ID] = row.Account
		}
		return nil
	})
	if err == nil {
		out.native = make(map[string]map[domain.CurrencyCode]application.LiquidityNativeAmounts, len(out.report.Overview.Buckets))
		for _, bucket := range out.report.Overview.Buckets {
			out.native[bucket.HorizonOn] = out.report.NativeSourceAmounts(bucket.HorizonOn)
		}
	}
	return out, err
}

type fundsLinks struct {
	account, instrument, reserve, product, policy, details widget.Clickable
	showDetails                                            bool
	menu                                                   ui.Select
}

type availableFundsPage struct {
	ctx                 context.Context
	service             availableFundsReader
	invalidate          func()
	workers             sync.WaitGroup
	cancel              context.CancelFunc
	request             <-chan historyResult[availableFundsRead]
	current             *availableFundsRead
	reservationLedger   fundsReservationLedger
	reservation         *fundsReservationSession
	reservationFocus    *widget.Clickable
	policyLedger        fundsPolicyLedger
	policy              *fundsPolicyDraft
	policyFocus         *widget.Clickable
	policyNotice        i18n.MessageID
	actionFocus         fundsActionFocus
	actionNotice        i18n.MessageID
	productLedger       fundsProductLedger
	productClock        func() time.Time
	setup               *fundsProductSetup
	addProduct          widget.Clickable
	productSaved        bool
	restoreAddFocus     bool
	err                 error
	state               fundsViewState
	controls            fundsControls
	localDate, timezone string
	nextRefresh         time.Time

	list               widget.List
	rowOrigin          image.Point
	rowWidth           int
	rowHeights         []int
	reload             widget.Clickable
	manageReservations widget.Clickable
	jumpSources        widget.Clickable
	links              map[string]*fundsLinks
	openAccount        func(domain.AccountID, string, event.Tag)
	openInstrument     func(domain.InstrumentID, string, event.Tag)
	productPage        *productDetailPage
	openProduct        func(domain.ProductContractID, string, event.Tag)
}

func newAvailableFundsPage(ctx context.Context, service availableFundsReader, invalidate func()) *availableFundsPage {
	policyLedger, _ := service.(fundsPolicyLedger)
	return &availableFundsPage{ctx: ctx, service: service, invalidate: invalidate, policyLedger: policyLedger,
		list: widget.List{List: layout.List{Axis: layout.Vertical}}, links: make(map[string]*fundsLinks)}
}

func (p *availableFundsPage) enter(position layout.Position, state fundsViewState) {
	p.list.Position, p.current, p.state = position, nil, state
	p.controls.early.Value = p.state.includeEarly
	p.productSaved, p.restoreAddFocus = false, false
	p.reservationFocus = nil
	p.policyFocus, p.policyNotice = nil, i18n.NoMessage
	p.actionFocus, p.actionNotice = fundsActionFocus{}, i18n.NoMessage
	p.read()
}

func (p *availableFundsPage) leave() {
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel, p.request = nil, nil
}

func (p *availableFundsPage) read() {
	p.leave()
	p.err = nil
	p.nextRefresh = time.Time{}
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel = cancel
	query := p.state.query()
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (availableFundsRead, error) {
		return readAvailableFundsQuery(ctx, p.service, query)
	})
}

func (p *availableFundsPage) update() {
	select {
	case result := <-p.request:
		p.leave()
		p.err = result.err
		if p.err != nil {
			p.current = nil
			return
		}
		p.current = &result.value
		p.localDate, p.timezone = p.current.report.Overview.LocalDate, p.current.report.Overview.Timezone
		if fundsBucketForHorizon(p.current.report.Overview.Buckets, p.state.horizonOn) == nil {
			p.state.horizonOn = ""
		}
		// Retain the button instances for surviving sources so Back can
		// restore focus even when its read replaces the report.
		active := make(map[string]bool)
		for _, row := range p.current.report.Sources {
			active[row.Result.SourceKey] = true
		}
		for _, row := range p.current.report.Overview.UnresolvedReservations {
			active[row.Reservation.ID.String()] = true
		}
		for key, buttons := range p.links {
			if active[key] {
				continue
			}
			// A released unresolved reservation can remove its entry row.
			// Restore focus to the persistent page control in that case.
			if p.reservation != nil && p.reservation.entry == &buttons.reserve {
				p.reservation.entry = &p.manageReservations
			}
			if p.reservationFocus == &buttons.reserve {
				p.reservationFocus = &p.manageReservations
			}
			delete(p.links, key)
		}
	default:
	}
}

func (p *availableFundsPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.openActionFocus(gtx, view)
	if d := p.setup; d != nil && d.poll() {
		p.closeProductSetup()
		p.productSaved = true
		p.read()
	}
	if p.err != nil && p.request == nil && p.reload.Clicked(gtx) {
		p.read()
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.request == nil && p.jumpSources.Clicked(gtx) {
		p.state.focusSources = true
	}
	l := view.Language
	if p.current != nil && p.request == nil && p.reservation == nil && p.policy == nil && p.setup == nil {
		switch {
		case p.nextRefresh.IsZero():
			p.nextRefresh = gtx.Now.Add(time.Minute)
		case !gtx.Now.Before(p.nextRefresh):
			p.read()
		}
		if !p.nextRefresh.IsZero() {
			gtx.Execute(op.InvalidateCmd{At: p.nextRefresh})
		}
	}
	var status i18n.MessageID
	switch {
	case p.err != nil:
		status = i18n.FundsReadFailed
	case p.request != nil && p.current == nil:
		status = i18n.FundsLoading
	case p.request != nil:
		status = i18n.FundsUpdating
	}
	var listOrigin image.Point
	return (ui.Column{Gap: ui.Space8}).LayoutWithOrigins(gtx, view.Origin, func(index int, origin image.Point) {
		if index == 2 {
			listOrigin = origin
		}
	},
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.err == nil && p.localDate != "" && (p.current == nil || len(p.current.report.Sources) != 0) {
				caption := l.Text(status)
				if status == i18n.NoMessage {
					caption = fmt.Sprintf(l.Text(i18n.FundsToday), historyDay(view, p.localDate), p.timezone)
				}
				return p.layoutFundsControls(gtx, view, caption)
			}
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if status == i18n.NoMessage && p.current != nil {
						overview := p.current.report.Overview
						text := fmt.Sprintf(l.Text(i18n.FundsToday), historyDay(view, overview.LocalDate), overview.Timezone)
						return view.Message(text, view.Colors.Ink)(gtx)
					}
					color := view.Colors.SecondaryInk
					if p.err != nil {
						color = view.Colors.ErrorInk
					}
					return view.Message(l.Text(status), color)(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.err == nil {
						return layout.Dimensions{}
					}
					view.Focus.Watch(gtx, &p.reload)
					return view.Button(&p.reload, l.Text(i18n.HistoryRetry))(gtx)
				}))
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.policyNotice != i18n.NoMessage {
				return view.Message(l.Text(p.policyNotice), view.Colors.Ink)(gtx)
			}
			if p.productSaved {
				return view.Message(l.Text(i18n.ProductSetupSaved), view.Colors.Ink)(gtx)
			}
			if p.actionNotice != i18n.NoMessage {
				return view.Message(l.Text(p.actionNotice), view.Colors.SecondaryInk)(gtx)
			}
			return layout.Dimensions{}
		}),
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			if p.current == nil {
				return layout.Dimensions{}
			}
			listView := view.WithScrollAnchors(&p.list)
			viewport := image.Rectangle{Min: listOrigin, Max: listOrigin.Add(gtx.Constraints.Max)}
			if view.Overlay != nil {
				view.Overlay.ScrollOutsidePanel(gtx, &p.list, viewport)
			}
			rows, sourcesIndex := p.rows(listView)
			if p.state.focusSources && p.request == nil {
				if sourcesIndex >= 0 {
					p.list.Position = layout.Position{First: sourcesIndex}
				} else {
					p.state.focusSources = false
				}
			}
			p.list.Position = restoredPosition(p.list.Position, len(rows))
			dims := pageListGapWithRowOrigins(gtx, listView, &p.list, l.Text(i18n.NavAvailableFunds), len(rows), ui.Space12,
				listOrigin, &p.rowHeights, func(origin image.Point, width, index int) {
					// Popover clipping applies the list's pixel offset after layout.
					p.rowOrigin, p.rowWidth = origin.Add(image.Pt(0, p.list.Position.Offset)), width
					if index == 0 {
						p.controls.comparisonOrigin = p.rowOrigin
					}
				}, func(gtx layout.Context, index int) layout.Dimensions {
					if p.request != nil {
						gtx = gtx.Disabled()
					}
					return rows[index](gtx)
				})
			if view.Overlay != nil {
				view.Overlay.OffsetScrollAnchor(&p.list, viewport)
			}
			return dims
		}))
}

func (p *availableFundsPage) closeProductSetup() {
	p.setup, p.restoreAddFocus = nil, true
}

func (p *availableFundsPage) rows(view ui.View) ([]layout.Widget, int) {
	data := p.current
	overview, bucket := data.report.Overview, p.selectedFundsBucket()
	groups := bucket.NativeCurrencyGroups
	var rows []layout.Widget
	if len(data.report.Sources) != 0 {
		rows = append(rows, p.fundsComparison(view, bucket.HorizonOn))
	} else if p.reservationLedger != nil {
		rows = append(rows, p.globalReservationButton(view, &p.manageReservations))
	}
	if len(groups) != 0 || len(data.report.Sources) != 0 {
		horizon := fundsHorizon{bucket: bucket, label: i18n.FundsHorizonToday}
		for _, option := range p.fundsHorizons() {
			if option.bucket.HorizonOn == bucket.HorizonOn {
				horizon = option
				break
			}
		}
		rows = append(rows, fundsBreakdown(view, horizon, data.native[bucket.HorizonOn]))
	}
	// Across currencies the converted base total is a distinct projection.
	// A single native group in the base currency already shows those amounts.
	if len(groups) != 1 || groups[0].Currency != overview.BaseCurrency {
		rows = append(rows, fundsBaseSummary(view, overview.BaseCurrency, bucket))
	}
	sourcesIndex := -1
	if len(data.report.Sources) == 0 {
		for _, warning := range bucket.Warnings {
			rows = append(rows, view.Message(view.Language.Text(fundsReason(warning)), view.Colors.SecondaryInk))
		}
		rows = append(rows, view.Message(view.Language.Text(i18n.FundsEmpty), view.Colors.SecondaryInk))
	} else {
		counts := countFundsSources(data.report.Sources, bucket.HorizonOn)
		sourcesIndex = len(rows)
		rows = append(rows, p.fundsSourcesHeader(view, bucket, counts, p.state.focusSources && p.request == nil))
		for _, source := range data.report.Sources {
			if fundsMatchesSourceFilter(source.Result, bucket.HorizonOn, p.state.filter) {
				rows = append(rows, p.source(view, source, bucket.HorizonOn))
			}
		}
		if counts[p.state.filter] == 0 {
			rows = append(rows, p.noMatchingFundsSources(view))
		}
	}
	for _, unresolved := range overview.UnresolvedReservations {
		reservation := unresolved.Reservation
		rows = append(rows, fundsCard(view, fmt.Sprintf(view.Language.Text(i18n.FundsUnresolved), reservation.Label),
			fundsAmount(view, i18n.FundsRequested, &reservation.Amount),
			view.Message(view.Language.Text(fundsReason(unresolved.Reason)), view.Colors.SecondaryInk),
			p.sourceLinks(view, reservation.ID.String(), reservation.Source.AccountID, "", ""),
			p.globalReservationButton(view, &p.sourceButtons(reservation.ID.String()).reserve)))
	}
	return rows, sourcesIndex
}

func (p *availableFundsPage) source(view ui.View, row application.LiquidityReadSource, day string) layout.Widget {
	source := row.Result
	l := view.Language
	unknown := l.Text(i18n.OverviewFundsUnknownAmount)
	values := fundsSourceValues{
		current:   fundsMoneyFigure(view, i18n.FundsCurrent, source.CurrentNativeValue),
		expected:  fundsFigure{i18n.FundsLater, unknown},
		spendable: fundsMoneyFigure(view, i18n.FundsAvailableAfter, nil),
	}
	if source.CurrentNativeValue == nil {
		values.current.value = unknown
	}
	values.spendable.value = unknown
	var details []layout.Widget
	result := fundsResultForHorizon(source, day)
	if result != nil && !source.Excluded {
		if result.UnreservedNative != nil {
			values.spendable = fundsMoneyFigure(view, i18n.FundsAvailableAfter, result.UnreservedNative)
		}
		if result.NetNative != nil {
			before := fundsMoneyFigure(view, i18n.FundsAvailableBefore, result.NetNative)
			details = append(details, labeledAmount(view, before.pattern, before.value, view.Colors.Ink))
		}
		if result.AppliedReserveNative != nil && !result.AppliedReserveNative.IsZero() {
			reserve := fundsMoneyFigure(view, i18n.FundsApplied, result.AppliedReserveNative)
			values.applied = &reserve
		}
		if result.ReserveShortfallNative != nil && !result.ReserveShortfallNative.IsZero() {
			details = append(details, fundsAmount(view, i18n.FundsShortfall, result.ReserveShortfallNative))
		}
	}
	if source.Excluded {
		values.expected.value = "—"
	}
	if source.Excluded || source.DueUnconfirmed {
		values.spendable.value = "—"
	}
	if !source.ReservationRequested.IsZero() {
		details = append(details, fundsAmount(view, i18n.FundsRequested, &source.ReservationRequested))
	}
	reasons := make([]i18n.MessageID, 0, 4)
	addReasons := func(values []string) {
		for _, value := range values {
			id := fundsReason(value)
			if !slices.Contains(reasons, id) {
				reasons = append(reasons, id)
			}
		}
	}
	addReasons(source.Reasons)
	if result != nil && !source.Excluded {
		route := result.SelectedRoute
		if route == nil {
			route = source.NormalRoute
			if p.state.includeEarly && source.EarlyRoute != nil {
				route = source.EarlyRoute
			}
		}
		addReasons(result.Reasons)
		if route != nil {
			if route.ReceiptOn != nil {
				values.expected.value = historyDay(view, *route.ReceiptOn)
			}
			if route.Kind == domain.RouteEarly {
				values.early = true
			}
			if route.FeeNative == nil || !route.FeeNative.IsZero() {
				details = append(details, fundsAmount(view, i18n.FundsRouteFee, route.FeeNative))
			}
			if result.SelectedRoute == nil && !source.DueUnconfirmed && (route.ReceiptOn == nil || *route.ReceiptOn <= day) {
				addReasons(route.MissingReasons)
			}
		}
		if result.SelectedRoute == nil && result.Status == domain.StatusComplete && !source.DueUnconfirmed {
			values.spendable.value = l.Text(i18n.FundsAtDateUnavailable)
		}
	}
	for _, reason := range reasons {
		if source.DueUnconfirmed && reason == i18n.FundsDue {
			continue
		}
		details = append(details, view.Message(l.Text(reason), view.Colors.SecondaryInk))
	}
	title := p.sourceTitle(view, source)
	buttons := p.sourceButtons(source.SourceKey)
	account := p.current.accounts[source.AccountID].Name
	name := p.sourceName(view, source)
	identity := fundsSourceIdentity(view, name, account, source)
	editable := p.policyLedger != nil && editableFundsPolicy(row)
	complete := editable && (source.DisplayState == domain.ProductDisplayNeedsInfo || result != nil && result.NeedsRules)
	var primary layout.Widget
	if complete {
		primary = p.policyButton(view, row, title, true)
	}
	if p.policyLedger != nil && !editable {
		reason := i18n.FundsPolicyReadOnly
		if source.ProductID != nil {
			reason = i18n.FundsPolicyManaged
		}
		details = append(details, view.Message(l.Text(reason), view.Colors.SecondaryInk))
	}
	actions := p.fundsSourceMenu(view, row, title, buttons, editable && !complete)
	return fundsSourceRow(view, title, buttons, identity, values, primary, actions, details)
}

func (p *availableFundsPage) sourceName(view ui.View, source domain.LiquiditySourceResult) string {
	if source.Ref.Kind == domain.SourceAccountCash {
		if name := p.current.accounts[source.AccountID].Name; name != "" {
			return fmt.Sprintf(view.Language.Text(i18n.FundsCashSource), name)
		}
	}
	return source.DisplayName
}

func (p *availableFundsPage) sourceTitle(view ui.View, source domain.LiquiditySourceResult) string {
	name := p.sourceName(view, source)
	if source.NativeCurrency != "" {
		name += " · " + source.NativeCurrency.String()
	}
	return name
}

func (p *availableFundsPage) fundsSourceMenu(view ui.View, row application.LiquidityReadSource, title string, buttons *fundsLinks, editRules bool) layout.Widget {
	source := row.Result
	l := view.Language
	entry := &buttons.menu.Trigger
	var items []ui.Choice
	commands := map[string]func(){}
	add := func(item ui.Choice, command func()) {
		items = append(items, item)
		commands[item.ID] = command
	}
	if editRules {
		add(ui.Choice{ID: "rules", Label: l.Text(i18n.FundsPolicyEdit)}, func() { p.startPolicy(row, title, entry) })
	}
	if p.reservationLedger != nil {
		add(ui.Choice{ID: "reservations", Label: l.Text(i18n.ReserveManageAll)}, func() { p.openReservations(source.Ref, entry) })
	}
	if source.ProductID != nil && p.openProduct != nil {
		add(ui.Choice{ID: "product", Label: fundsFigureLabel(l, i18n.ProductOpen)}, func() { p.openProduct(*source.ProductID, source.DisplayName, entry) })
	}
	if name := p.current.accounts[source.AccountID].Name; p.openAccount != nil && name != "" {
		add(ui.Choice{ID: "account", Label: fundsFigureLabel(l, i18n.HistoryOpenAccount)}, func() { p.openAccount(source.AccountID, name, entry) })
	}
	if source.ProductID == nil && row.InstrumentID != nil && p.openInstrument != nil {
		add(ui.Choice{ID: "instrument", Label: fundsFigureLabel(l, i18n.PortfolioOpenInstrument)}, func() {
			p.openInstrument(domain.InstrumentID(*row.InstrumentID), source.DisplayName, entry)
		})
	}
	if len(items) == 0 {
		return nil
	}
	label := fmt.Sprintf(l.Text(i18n.PortfolioActionsFor), title)
	return func(gtx layout.Context) layout.Dimensions {
		p.restoreReservationEntry(gtx, entry)
		size := gtx.Dp(ui.ControlCompact)
		origin := p.rowOrigin.Add(image.Pt(p.rowWidth-gtx.Dp(ui.Space12)-size, gtx.Dp(ui.Space8)))
		bounds := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(size, size))}
		buttons.menu.Menu(gtx, view, bounds, items, label, "", nil, nil, 208, func(id string) {
			if command := commands[id]; command != nil {
				command()
			}
		})
		view.Focus.Watch(gtx, entry)
		return view.ButtonOf(entry, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonCompact,
			Name: label, ShortName: label, Icon: lucide.Ellipsis})(gtx)
	}
}

func (p *availableFundsPage) sourceLinks(view ui.View, key string, account domain.AccountID, instrument domain.InstrumentID, name string) layout.Widget {
	return fundsWrappedActions(p.sourceNavigationActions(view, key, account, instrument, name))
}

func (p *availableFundsPage) sourceNavigationActions(view ui.View, key string, account domain.AccountID, instrument domain.InstrumentID, name string) []layout.Widget {
	l := view.Language
	var actions []layout.Widget
	if accountName := p.current.accounts[account].Name; p.openAccount != nil && accountName != "" {
		actions = append(actions, func(gtx layout.Context) layout.Dimensions {
			buttons := p.sourceButtons(key)
			if buttons.account.Clicked(gtx) {
				p.openAccount(account, accountName, &buttons.account)
				gtx.Execute(op.InvalidateCmd{})
			}
			view.Focus.Watch(gtx, &buttons.account)
			return view.ButtonOf(&buttons.account, ui.ButtonSpec{
				Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: fundsFigureLabel(l, i18n.HistoryOpenAccount),
				Name: fmt.Sprintf(l.Text(i18n.HistoryOpenAccount), accountName),
			})(gtx)
		})
	}
	if instrument != "" && p.openInstrument != nil {
		actions = append(actions, func(gtx layout.Context) layout.Dimensions {
			buttons := p.sourceButtons(key)
			if buttons.instrument.Clicked(gtx) {
				p.openInstrument(instrument, name, &buttons.instrument)
				gtx.Execute(op.InvalidateCmd{})
			}
			view.Focus.Watch(gtx, &buttons.instrument)
			return view.ButtonOf(&buttons.instrument, ui.ButtonSpec{
				Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: fundsFigureLabel(l, i18n.PortfolioOpenInstrument),
				Name: fmt.Sprintf(l.Text(i18n.PortfolioOpenInstrument), name),
			})(gtx)
		})
	}
	return actions
}

func fundsCard(view ui.View, title string, fields ...layout.Widget) layout.Widget {
	return fundsCardWithHeader(view, view.Body(ui.TextHeading, title, view.Colors.Ink), fields...)
}

func fundsCardWithHeader(view ui.View, header layout.Widget, fields ...layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, len(fields)+2)
			children = append(children,
				layout.Rigid(header),
				layout.Rigid(view.Hairline),
			)
			for _, field := range fields {
				children = append(children, layout.Rigid(field))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, children...)
		})
	}
}

// fundsAmount displays exact amounts for reviews and ledger evidence.
func fundsAmount(view ui.View, label i18n.MessageID, amount *domain.Money) layout.Widget {
	value := view.Language.Text(i18n.OverviewNoValue)
	if amount != nil {
		value = portfolioDecimal(view, amount.CanonicalAmount()) + " " + amount.Currency().String()
	}
	return labeledAmount(view, label, value, view.Colors.Ink)
}

// fundsAmountText formats a read-only projection at its currency's precision.
func fundsAmountText(view ui.View, amount *domain.Money) string {
	if amount == nil {
		return view.Language.Text(i18n.OverviewNoValue)
	}
	return valuationMoney(view, domain.MoneyView{Amount: amount.CanonicalAmount(), Currency: amount.Currency()}, false)
}

func fundsNativeAmountText(view ui.View, amount *decimal.Decimal, currency domain.CurrencyCode) string {
	if amount == nil {
		return view.Language.Text(i18n.OverviewNoValue)
	}
	return valuationMoney(view, domain.MoneyView{Amount: amount.String(), Currency: currency}, false)
}

func fundsStatus(status domain.CompletenessStatus) i18n.MessageID {
	switch status {
	case domain.StatusComplete:
		return i18n.FundsComplete
	case domain.StatusPartial:
		return i18n.FundsPartial
	default:
		return i18n.OverviewNoValue
	}
}

func fundsReason(value string) i18n.MessageID {
	switch value {
	case "base FX is missing", "missing base FX":
		return i18n.FundsMissingBaseFX
	case "source is archived, a liability, or has zero quantity":
		return i18n.FundsIneligible
	case "explicitly excluded from short-term access":
		return i18n.FundsExcluded
	case "due — receipt unconfirmed":
		return i18n.FundsDue
	case "managed product policy is missing", "access policy is unknown":
		return i18n.FundsMissingRules
	case "access timing is unknown", "receipt timing is unknown":
		return i18n.FundsMissingTiming
	case "amount is unknown", "early amount is unknown":
		return i18n.FundsMissingAmount
	case "exit fee is unknown", "early fee is unknown":
		return i18n.FundsMissingFee
	case "reservation currency does not match the source":
		return i18n.FundsReservationCurrency
	case "source is missing, archived, or no longer positive":
		return i18n.FundsUnresolvedReason
	default:
		return i18n.FundsUnknownRoute
	}
}
