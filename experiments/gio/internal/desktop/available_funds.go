package desktop

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

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
}

func readAvailableFunds(ctx context.Context, service availableFundsReader) (out availableFundsRead, err error) {
	err = service.WithRead(ctx, func(ctx context.Context) error {
		report, err := service.LiquidityRead(ctx, application.LiquidityOverviewQuery{})
		if err != nil {
			return err
		}
		at := slices.IndexFunc(report.Overview.Buckets, func(b domain.LiquidityBucket) bool { return b.HorizonOn == report.Overview.LocalDate })
		if at < 0 {
			return &domain.Error{Code: domain.ErrIntegrity}
		}
		accounts, err := service.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
		if err != nil {
			return err
		}
		out = availableFundsRead{report: report, today: report.Overview.Buckets[at], accounts: make(map[domain.AccountID]domain.Account, len(accounts))}
		for _, row := range accounts {
			out.accounts[row.Account.ID] = row.Account
		}
		return nil
	})
	return out, err
}

type fundsLinks struct {
	account, instrument, reserve, product widget.Clickable
}

type availableFundsPage struct {
	ctx               context.Context
	service           availableFundsReader
	invalidate        func()
	workers           sync.WaitGroup
	cancel            context.CancelFunc
	request           <-chan historyResult[availableFundsRead]
	current           *availableFundsRead
	reservationLedger fundsReservationLedger
	reservation       *fundsReservationSession
	productLedger     fundsProductLedger
	productClock      func() time.Time
	setup             *fundsProductSetup
	addProduct        widget.Clickable
	productSaved      bool
	restoreAddFocus   bool
	err               error

	list           widget.List
	reload         widget.Clickable
	links          map[string]*fundsLinks
	openAccount    func(domain.AccountID, string, event.Tag)
	openInstrument func(domain.InstrumentID, string, event.Tag)
	productPage    *productDetailPage
	openProduct    func(domain.ProductContractID, string, event.Tag)
}

func newAvailableFundsPage(ctx context.Context, service availableFundsReader, invalidate func()) *availableFundsPage {
	return &availableFundsPage{ctx: ctx, service: service, invalidate: invalidate,
		list: widget.List{List: layout.List{Axis: layout.Vertical}}, links: make(map[string]*fundsLinks)}
}

func (p *availableFundsPage) enter(position layout.Position) {
	p.list.Position, p.current = position, nil
	p.productSaved, p.restoreAddFocus = false, false
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
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel = cancel
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (availableFundsRead, error) {
		return readAvailableFunds(ctx, p.service)
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
		// Retain the button instances for surviving sources so Back can
		// restore focus even when its read replaces the report.
		active := make(map[string]bool)
		for _, row := range p.current.report.Sources {
			active[row.Result.SourceKey] = true
		}
		for _, row := range p.current.report.Overview.UnresolvedReservations {
			active[row.Reservation.ID.String()] = true
		}
		for key := range p.links {
			if !active[key] {
				delete(p.links, key)
			}
		}
	default:
	}
}

func (p *availableFundsPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.update()
	if d := p.setup; d != nil {
		if !d.poll() {
			overview := p.current.report.Overview
			return d.layout(gtx, view, productFormCalendar{today: overview.LocalDate, zone: overview.Timezone}, func() {
				p.setup, p.restoreAddFocus = nil, true
			})
		}
		p.setup, p.productSaved, p.restoreAddFocus = nil, true, true
		p.read()
		view.RestorePageScroll(0)
	}
	if p.reservation != nil {
		p.pollReservations()
		return p.layoutReservations(gtx, view)
	}
	if p.request == nil && p.reload.Clicked(gtx) {
		p.read()
		gtx.Execute(op.InvalidateCmd{})
	}
	l := view.Language
	accounts := p.productAccounts()
	if len(accounts) != 0 && p.request == nil && p.addProduct.Clicked(gtx) {
		p.productSaved = false
		run := productSetupRuntime{ctx: p.ctx, ledger: p.productLedger, clock: p.productClock, workers: &p.workers, invalidate: p.invalidate}
		p.setup = newProductSetup(run, accounts, l.Text(i18n.ProductAddDeposit))
		view.RestorePageScroll(0)
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	var status i18n.MessageID
	action := i18n.HistoryRefresh
	switch {
	case p.err != nil:
		status, action = i18n.FundsReadFailed, i18n.HistoryRetry
	case p.request != nil:
		status = i18n.FundsLoading
	}
	return ui.Column{Gap: 10}.Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
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
					if len(accounts) == 0 {
						return layout.Dimensions{}
					}
					if p.restoreAddFocus && p.request == nil {
						gtx.Execute(key.FocusCmd{Tag: &p.addProduct})
						p.restoreAddFocus = false
					}
					if p.request != nil {
						gtx = gtx.Disabled()
					}
					view.Focus.Watch(gtx, &p.addProduct)
					return view.PrimaryButton(&p.addProduct, l.Text(i18n.ProductAddAction))(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.request != nil {
						gtx = gtx.Disabled()
					}
					view.Focus.Watch(gtx, &p.reload)
					return view.Button(&p.reload, l.Text(action))(gtx)
				}))
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.productSaved {
				return view.Message(l.Text(i18n.ProductSetupSaved), view.Colors.Ink)(gtx)
			}
			return layout.Dimensions{}
		}),
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			if p.current == nil || p.request != nil {
				return layout.Dimensions{}
			}
			rows := p.rows(view)
			p.list.Position = restoredPosition(p.list.Position, len(rows))
			return pageListGap(gtx, view, &p.list, l.Text(i18n.NavAvailableFunds), len(rows), ui.Space12, func(gtx layout.Context, i int) layout.Dimensions {
				return rows[i](gtx)
			})
		}))
}

func (p *availableFundsPage) rows(view ui.View) []layout.Widget {
	data := p.current
	overview, today := data.report.Overview, data.today
	groups := today.NativeCurrencyGroups
	var rows []layout.Widget
	// Across currencies the converted base total is a distinct projection.
	// A single native group in the base currency already shows those amounts.
	if len(groups) != 1 || groups[0].Currency != overview.BaseCurrency {
		rows = append(rows, fundsSummary(view, fmt.Sprintf(view.Language.Text(i18n.FundsBase), overview.BaseCurrency), today.Status,
			today.FullAvailable, today.KnownAvailableSubtotal, today.AppliedReserveSubtotal, today.FullUnreserved, today.KnownUnreservedSubtotal))
	}
	for _, group := range groups {
		rows = append(rows, fundsSummary(view, fmt.Sprintf(view.Language.Text(i18n.FundsNative), group.Currency), group.Status,
			group.FullAvailable, group.KnownAvailableSubtotal, group.AppliedReserveSubtotal, group.FullUnreserved, group.KnownUnreservedSubtotal))
	}
	for _, warning := range today.Warnings {
		rows = append(rows, view.Message(view.Language.Text(fundsReason(warning)), view.Colors.SecondaryInk))
	}
	if len(data.report.Sources) == 0 {
		rows = append(rows, view.Message(view.Language.Text(i18n.FundsEmpty), view.Colors.SecondaryInk))
	} else {
		rows = append(rows, tradeTitle(view, i18n.FundsSources))
	}
	for _, source := range data.report.Sources {
		rows = append(rows, p.source(view, source, today.HorizonOn))
	}
	for _, unresolved := range overview.UnresolvedReservations {
		reservation := unresolved.Reservation
		rows = append(rows, fundsCard(view, fmt.Sprintf(view.Language.Text(i18n.FundsUnresolved), reservation.Label),
			fundsAmount(view, i18n.FundsRequested, &reservation.Amount),
			view.Message(view.Language.Text(fundsReason(unresolved.Reason)), view.Colors.SecondaryInk),
			p.sourceLinks(view, reservation.ID.String(), reservation.Source.AccountID, "", "")))
	}
	return rows
}

func fundsSummary(view ui.View, title string, status domain.CompletenessStatus, full, known, applied, fullAfter, knownAfter *domain.Money) layout.Widget {
	beforeLabel, reserveLabel, afterLabel := i18n.FundsAvailableBefore, i18n.FundsApplied, i18n.FundsAvailableAfter
	if status != domain.StatusComplete && applied != nil {
		reserveLabel = i18n.FundsAppliedSubtotal
	}
	if full == nil && known != nil {
		full, beforeLabel = known, i18n.FundsBeforeSubtotal
	}
	if fullAfter == nil && knownAfter != nil {
		fullAfter, afterLabel = knownAfter, i18n.FundsAfterSubtotal
	}
	return fundsCard(view, title,
		func(gtx layout.Context) layout.Dimensions {
			tone := ui.BadgeWarning
			if status == domain.StatusComplete {
				tone = ui.BadgeSuccess
			}
			return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return view.Badge(gtx, tone, view.Language.Text(fundsStatus(status)))
			})
		},
		fundsAmount(view, beforeLabel, full),
		fundsAmount(view, reserveLabel, applied),
		fundsAmount(view, afterLabel, fullAfter))
}

func (p *availableFundsPage) source(view ui.View, row application.LiquidityReadSource, day string) layout.Widget {
	source := row.Result
	var kind domain.ProductKind
	if source.ProductID != nil && row.ProductKind != nil {
		kind = domain.ProductKind(*row.ProductKind)
	}
	l := view.Language
	fields := []layout.Widget{fundsAmount(view, i18n.FundsCurrent, source.CurrentNativeValue)}
	at := slices.IndexFunc(source.BucketResults, func(r domain.LiquidityBucketResult) bool { return r.HorizonOn == day })
	if at >= 0 && !source.Excluded {
		result := source.BucketResults[at]
		fields = append(fields, fundsAmount(view, i18n.FundsAvailableBefore, result.NetNative),
			fundsAmount(view, i18n.FundsApplied, result.AppliedReserveNative), fundsAmount(view, i18n.FundsAvailableAfter, result.UnreservedNative))
		if result.ReserveShortfallNative != nil && !result.ReserveShortfallNative.IsZero() {
			fields = append(fields, fundsAmount(view, i18n.FundsShortfall, result.ReserveShortfallNative))
		}
	}
	if !source.ReservationRequested.IsZero() {
		fields = append(fields, fundsAmount(view, i18n.FundsRequested, &source.ReservationRequested))
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
	if at >= 0 && !source.Excluded {
		result := source.BucketResults[at]
		route := source.NormalRoute
		addReasons(result.Reasons)
		if route != nil && result.SelectedRoute == nil && !source.DueUnconfirmed {
			if route.ReceiptOn != nil && *route.ReceiptOn > day {
				fields = append(fields, view.Message(fmt.Sprintf(l.Text(i18n.FundsLater), historyDay(view, *route.ReceiptOn)), view.Colors.SecondaryInk))
			} else {
				addReasons(route.MissingReasons)
			}
		}
		if result.SelectedRoute == nil && result.Status == domain.StatusComplete && len(reasons) == 0 &&
			(route == nil || route.ReceiptOn == nil || *route.ReceiptOn <= day) {
			reasons = append(reasons, i18n.FundsNoRoute)
		}
	}
	for _, reason := range reasons {
		if kind != "" && reason == i18n.FundsDue && source.DisplayState == domain.ProductDisplayDueUnconfirmed {
			continue // The product state below already explains the unconfirmed receipt.
		}
		fields = append(fields, view.Message(l.Text(reason), view.Colors.SecondaryInk))
	}
	var instrument domain.InstrumentID
	if source.ProductID == nil && row.InstrumentID != nil {
		instrument = domain.InstrumentID(*row.InstrumentID)
	}
	if kind == domain.ProductTermDeposit || kind == domain.ProductLockedProduct {
		fields = append(fields, productState(view, kind, source.DisplayState))
		if p.openProduct != nil {
			fields = append(fields, productLink(view, *source.ProductID, source.DisplayName, &p.sourceButtons(source.SourceKey).product, p.openProduct))
		}
	}
	fields = append(fields, p.sourceLinks(view, source.SourceKey, source.AccountID, instrument, source.DisplayName))
	account := p.current.accounts[source.AccountID]
	if p.reservationLedger != nil && fundsReservationSource(account, source) {
		fields = append(fields, p.reservationButton(view, account.Name, source.Ref))
	}
	title := source.DisplayName
	if source.Ref.Kind == domain.SourceAccountCash {
		if name := p.current.accounts[source.AccountID].Name; name != "" {
			title = fmt.Sprintf(l.Text(i18n.FundsCashSource), name)
		}
	}
	if source.NativeCurrency != "" {
		title += " · " + source.NativeCurrency.String()
	}
	return fundsCard(view, title, fields...)
}

func (p *availableFundsPage) sourceLinks(view ui.View, key string, account domain.AccountID, instrument domain.InstrumentID, name string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := view.Language
		buttons := p.sourceButtons(key)
		var actions []layout.FlexChild
		if accountName := p.current.accounts[account].Name; p.openAccount != nil && accountName != "" {
			if buttons.account.Clicked(gtx) {
				p.openAccount(account, accountName, &buttons.account)
				gtx.Execute(op.InvalidateCmd{})
			}
			actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					view.Focus.Watch(gtx, &buttons.account)
					return view.Button(&buttons.account, fmt.Sprintf(l.Text(i18n.HistoryOpenAccount), accountName))(gtx)
				})
			}))
		}
		if instrument != "" && p.openInstrument != nil {
			if buttons.instrument.Clicked(gtx) {
				p.openInstrument(instrument, name, &buttons.instrument)
				gtx.Execute(op.InvalidateCmd{})
			}
			actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					view.Focus.Watch(gtx, &buttons.instrument)
					return view.Button(&buttons.instrument, fmt.Sprintf(l.Text(i18n.PortfolioOpenInstrument), name))(gtx)
				})
			}))
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx, actions...)
	}
}

func fundsCard(view ui.View, title string, fields ...layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, len(fields)+2)
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Body(ui.TextHeading, title, view.Colors.Ink)(gtx)
				}),
				layout.Rigid(view.Hairline),
			)
			for _, field := range fields {
				children = append(children, layout.Rigid(field))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, children...)
		})
	}
}

// fundsAmount shows a labelled amount as a name on the left and the figure on
// the right.
func fundsAmount(view ui.View, label i18n.MessageID, amount *domain.Money) layout.Widget {
	value := view.Language.Text(i18n.OverviewNoValue)
	if amount != nil {
		value = portfolioDecimal(view, amount.CanonicalAmount()) + " " + amount.Currency().String()
	}
	return labeledAmount(view, label, value, view.Colors.Ink)
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
	case "amount is unknown":
		return i18n.FundsMissingAmount
	case "exit fee is unknown":
		return i18n.FundsMissingFee
	case "reservation currency does not match the source":
		return i18n.FundsReservationCurrency
	case "source is missing, archived, or no longer positive":
		return i18n.FundsUnresolvedReason
	default:
		return i18n.FundsUnknownRoute
	}
}
