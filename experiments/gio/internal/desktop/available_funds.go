package desktop

import (
	"context"
	"fmt"
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
	var headerHeight int
	return ui.Column{Gap: ui.Space8}.Layout(gtx,
		ui.Rigid(func(gtx layout.Context) (dims layout.Dimensions) {
			defer func() { headerHeight = dims.Size.Y }()
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
			if p.localDate == "" || p.current != nil && len(p.current.report.Sources) == 0 {
				return layout.Dimensions{}
			}
			view.Origin.Y += headerHeight + gtx.Dp(ui.Space8)
			return p.layoutFundsControls(gtx, view)
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
			rows, sourcesIndex := p.rows(view)
			if p.state.focusSources && p.request == nil {
				if sourcesIndex >= 0 {
					p.list.Position = layout.Position{First: sourcesIndex}
				} else {
					p.state.focusSources = false
				}
			}
			p.list.Position = restoredPosition(p.list.Position, len(rows))
			return pageListGap(gtx, view, &p.list, l.Text(i18n.NavAvailableFunds), len(rows), ui.Space12, func(gtx layout.Context, i int) layout.Dimensions {
				if p.request != nil {
					gtx = gtx.Disabled()
				}
				return rows[i](gtx)
			})
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
	// Across currencies the converted base total is a distinct projection.
	// A single native group in the base currency already shows those amounts.
	if len(groups) != 1 || groups[0].Currency != overview.BaseCurrency {
		rows = append(rows, fundsBaseSummary(view, overview.BaseCurrency, bucket))
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
	sourcesIndex := -1
	if len(data.report.Sources) == 0 {
		for _, warning := range bucket.Warnings {
			rows = append(rows, view.Message(view.Language.Text(fundsReason(warning)), view.Colors.SecondaryInk))
		}
		rows = append(rows, view.Message(view.Language.Text(i18n.FundsEmpty), view.Colors.SecondaryInk))
	} else {
		counts := countFundsSources(data.report.Sources, bucket.HorizonOn)
		sourcesIndex = len(rows)
		rows = append(rows, p.fundsSourcesHeader(view, bucket, counts))
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
	var kind domain.ProductKind
	if source.ProductID != nil && row.ProductKind != nil {
		kind = domain.ProductKind(*row.ProductKind)
	}
	l := view.Language
	values := fundsSourceValues{
		current:   fundsMoneyFigure(view, i18n.FundsCurrent, source.CurrentNativeValue),
		expected:  fundsFigure{i18n.FundsLater, l.Text(i18n.OverviewNoValue)},
		spendable: fundsMoneyFigure(view, i18n.FundsAvailableAfter, nil),
	}
	var notes, details, actions []layout.Widget
	result := fundsResultForHorizon(source, day)
	if result != nil && !source.Excluded {
		values.spendable = fundsMoneyFigure(view, i18n.FundsAvailableAfter, result.UnreservedNative)
		if result.NetNative != nil {
			before := fundsMoneyFigure(view, i18n.FundsAvailableBefore, result.NetNative)
			details = append(details, labeledAmount(view, before.pattern, before.value, view.Colors.Ink))
		}
		if result.AppliedReserveNative != nil && !result.AppliedReserveNative.IsZero() {
			reserve := fundsMoneyFigure(view, i18n.FundsApplied, result.AppliedReserveNative)
			values.applied = &reserve
		}
		if result.ReserveShortfallNative != nil && !result.ReserveShortfallNative.IsZero() {
			notes = append(notes, fundsAmount(view, i18n.FundsShortfall, result.ReserveShortfallNative))
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
		if kind != "" && reason == i18n.FundsDue && source.DisplayState == domain.ProductDisplayDueUnconfirmed {
			continue // The product state below already explains the unconfirmed receipt.
		}
		notes = append(notes, view.Message(l.Text(reason), view.Colors.SecondaryInk))
	}
	var instrument domain.InstrumentID
	if source.ProductID == nil && row.InstrumentID != nil {
		instrument = domain.InstrumentID(*row.InstrumentID)
	}
	if kind == domain.ProductTermDeposit || kind == domain.ProductLockedProduct {
		notes = append(notes, productState(view, kind, source.DisplayState))
		if p.openProduct != nil {
			actions = append(actions, productLink(view, *source.ProductID, source.DisplayName, &p.sourceButtons(source.SourceKey).product, p.openProduct))
		}
	}
	title := p.sourceTitle(view, source)
	if p.policyLedger != nil {
		switch {
		case editableFundsPolicy(row):
			actions = append(actions, p.policyButton(view, row, title, result != nil && result.NeedsRules))
		case source.ProductID != nil:
			details = append(details, view.Message(l.Text(i18n.FundsPolicyManaged), view.Colors.SecondaryInk))
		default:
			details = append(details, view.Message(l.Text(i18n.FundsPolicyReadOnly), view.Colors.SecondaryInk))
		}
	}
	if p.reservationLedger != nil {
		name := source.DisplayName
		if source.Ref.Kind != domain.SourceAccountValue {
			name = reservationSourceName(view, &source, p.current.accounts[source.AccountID].Name)
		}
		actions = append(actions, p.reservationButton(view, name, source.Ref))
	}
	actions = append(actions, p.sourceNavigationActions(view, source.SourceKey, source.AccountID, instrument, source.DisplayName)...)
	return func(gtx layout.Context) layout.Dimensions {
		return fundsSourceRow(view, title, p.sourceButtons(source.SourceKey), values, notes, actions, details)(gtx)
	}
}

func (p *availableFundsPage) sourceTitle(view ui.View, source domain.LiquiditySourceResult) string {
	title := source.DisplayName
	if source.Ref.Kind == domain.SourceAccountCash {
		if name := p.current.accounts[source.AccountID].Name; name != "" {
			title = fmt.Sprintf(view.Language.Text(i18n.FundsCashSource), name)
		}
	}
	if source.NativeCurrency != "" {
		title += " · " + source.NativeCurrency.String()
	}
	return title
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
