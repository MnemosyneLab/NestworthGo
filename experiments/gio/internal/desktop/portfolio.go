package desktop

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type portfolioReader interface {
	history.PositionImportAccountReader
	WithRead(context.Context, func(context.Context) error) error
	Portfolio(context.Context, domain.AccountFilter) (domain.PortfolioValuation, error)
	InstrumentHoldings(context.Context) ([]domain.InstrumentHoldingsView, error)
	InstrumentHoldingGroups(context.Context) ([]domain.InstrumentHoldingGroupView, error)
	AccountHoldingDetails(context.Context, ...domain.AccountID) ([]domain.InstrumentHoldingsView, error)
	InstrumentProductOwnership(context.Context) (map[string]string, error)
	Product(context.Context, domain.ProductContractID) (application.ProductDetail, error)
	Bootstrap(context.Context) (application.Bootstrap, error)
	ListInstruments(context.Context, bool) ([]domain.Instrument, error)
	CurrentInstrumentQuote(context.Context, domain.InstrumentID) (*domain.InstrumentQuote, error)
	CurrentFXQuote(context.Context, domain.CurrencyCode, domain.CurrencyCode) (*domain.FXQuote, error)
}

type portfolioRead struct {
	importAccounts []domain.Account
	summary        domain.PortfolioValuation
	groups         []domain.InstrumentHoldingGroupView
	positions      []domain.InstrumentHoldingsView
	instrument     *domain.Instrument
	price          *domain.InstrumentQuote
	fx             *domain.FXQuote
	base           domain.CurrencyCode
	managed        bool
	product        *application.ProductDetail
	catalog        *portfolioCatalogRead
	allocation     *portfolioAllocationRead
}

func readPortfolio(ctx context.Context, service portfolioReader, id domain.InstrumentID, account domain.AccountID) (out portfolioRead, err error) {
	err = service.WithRead(ctx, func(ctx context.Context) error {
		if id == "" {
			var err error
			out.summary, err = service.Portfolio(ctx, domain.AccountFilter{})
			if err != nil {
				return err
			}
			out.importAccounts, err = history.PositionImportAccounts(ctx, service)
			if err != nil {
				return err
			}
			out.groups, err = service.InstrumentHoldingGroups(ctx)
			return err
		}
		ownership, err := service.InstrumentProductOwnership(ctx)
		if err != nil {
			return err
		}
		if contractID := ownership[id.String()]; contractID != "" {
			out.managed = true
			detail, err := service.Product(ctx, domain.ProductContractID(contractID))
			if err == nil {
				out.product = &detail
			}
			return err
		}
		instruments, err := service.ListInstruments(ctx, true)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(instruments, func(row domain.Instrument) bool { return row.ID == id })
		if i < 0 {
			return nil
		}
		out.instrument = &instruments[i]
		boot, err := service.Bootstrap(ctx)
		if err != nil || boot.Household == nil {
			return err
		}
		out.base = boot.Household.BaseCurrency
		out.price, err = service.CurrentInstrumentQuote(ctx, id)
		if err != nil {
			return err
		}
		if out.instrument.QuoteCurrency != out.base {
			out.fx, err = service.CurrentFXQuote(ctx, out.instrument.QuoteCurrency, out.base)
			if err != nil {
				return err
			}
		}
		if account == "" {
			out.positions, err = service.InstrumentHoldings(ctx)
		} else {
			out.positions, err = service.AccountHoldingDetails(ctx, account)
		}
		if err == nil {
			out.positions = slices.DeleteFunc(out.positions, func(row domain.InstrumentHoldingsView) bool {
				return row.InstrumentID != id
			})
		}
		return err
	})
	return out, err
}

type portfolioPage struct {
	ctx          context.Context
	service      portfolioReader
	invalidate   func()
	workers      sync.WaitGroup
	id           domain.InstrumentID
	accountScope domain.AccountID
	cancel       context.CancelFunc
	request      <-chan historyResult[portfolioRead]
	current      *portfolioRead
	err          error
	investment   *accountInvestment
	clock        func() time.Time
	holdings     portfolioHoldings
	catalog      portfolioCatalog
	tabs         portfolioTabs
	allocation   portfolioAllocation
	detail       portfolioInstrumentDetail

	search                ui.SearchField
	list                  widget.List
	reload                widget.Clickable
	instruments           map[domain.InstrumentID]*widget.Clickable
	accounts              map[domain.AccountID]*widget.Clickable
	openInstrument        func(domain.InstrumentID, string, event.Tag)
	openHoldingInstrument func(domain.InstrumentID, domain.AccountID, string, event.Tag)
	openAccount           func(domain.AccountID, string, event.Tag)
	openProduct           func(domain.ProductContractID, string, event.Tag)
	openHoldingHistory    func(domain.InstrumentID, domain.AccountID, string, event.Tag)
	openHoldingAnalysis   func(routeID, domain.InstrumentID, domain.AccountID, string, event.Tag)
	productButtons        map[domain.ProductContractID]*widget.Clickable
}

func newPortfolioPage(ctx context.Context, service portfolioReader, invalidate func()) *portfolioPage {
	catalogLedger, _ := service.(portfolioCatalogLedger)
	return &portfolioPage{ctx: ctx, service: service, invalidate: invalidate,
		catalog: portfolioCatalog{ledger: catalogLedger, search: ui.SearchField{Field: ui.NewTextField(true)}, links: make(map[domain.InstrumentID]*portfolioCatalogActions)},
		search:  ui.SearchField{Field: ui.NewTextField(true)}, list: widget.List{List: layout.List{Axis: layout.Vertical}},
		instruments: make(map[domain.InstrumentID]*widget.Clickable), accounts: make(map[domain.AccountID]*widget.Clickable), productButtons: make(map[domain.ProductContractID]*widget.Clickable)}
}

func (p *portfolioPage) enterScoped(id domain.InstrumentID, account domain.AccountID, query string, position layout.Position) {
	p.id, p.current = id, nil
	p.accountScope = account
	p.search.Field.Editor.SetText(query)
	p.list.Position = position
	p.read()
}

func (p *portfolioPage) leave() {
	p.retireRead()
	p.allocation.retireTrend()
	p.detail.retireReads()
}

func (p *portfolioPage) retireRead() {
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel, p.request = nil, nil
}

func (p *portfolioPage) read() {
	p.leave()
	p.err = nil
	if p.id != "" {
		if _, err := domain.ParseInstrumentID(p.id.String()); err != nil {
			p.current = &portfolioRead{}
			return
		}
	}
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel = cancel
	id, account := p.id, p.accountScope
	tab := p.tabs.active
	p.allocation.stale = true
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (portfolioRead, error) {
		if id == "" {
			switch tab {
			case portfolioInstruments:
				return readPortfolioCatalog(ctx, p.service)
			case portfolioAllocationTab:
				return readPortfolioAllocation(ctx, p.service)
			}
		}
		return readPortfolio(ctx, p.service, id, account)
	})
}

func (p *portfolioPage) update() {
	select {
	case result := <-p.request:
		p.retireRead()
		p.err = result.err
		if p.err == nil {
			p.current = &result.value
		}
	default:
	}
}

func (p *portfolioPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.update()
	p.updateAllocationTrend(gtx, view)
	if f := p.investment; f != nil {
		run := investmentRuntime{ctx: p.ctx, workers: &p.workers, clock: p.clock, invalidate: p.invalidate}
		f.poll(run)
		if p.canImportPosition() && f.draft == nil && f.importOpen.Clicked(gtx) {
			f.draft = newAccountInvestmentDraft(domain.ActivityPositionTransfer, domain.Account{})
			f.draft.pickAccount, f.draft.focusAccount = true, true
			f.draft.accounts = p.current.importAccounts
			view.RestorePageScroll(0)
			gtx.Execute(op.InvalidateCmd{})
		}
		if f.draft != nil {
			return f.layoutForm(gtx, view, run)
		}
	}
	if p.reload.Clicked(gtx) {
		p.read()
	}
	l := view.Language
	status, reload := i18n.NoMessage, i18n.HistoryRefresh
	switch {
	case p.err != nil:
		status, reload = i18n.PortfolioReadFailed, i18n.HistoryRetry
	case p.request != nil && p.current == nil:
		status = i18n.PortfolioLoading
	case p.request != nil:
		status = i18n.OverviewRefreshing
	}
	return ui.Column{Gap: 12}.Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					if status == i18n.NoMessage {
						return layout.Dimensions{Size: gtx.Constraints.Min}
					}
					color := view.Colors.SecondaryInk
					if p.err != nil {
						color = view.Colors.ErrorInk
					}
					return view.Message(l.Text(status), color)(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !p.canImportPosition() {
						return layout.Dimensions{}
					}
					f := p.investment
					view.Focus.Watch(gtx, &f.importOpen)
					if f.restore == &f.importOpen && gtx.Enabled() {
						gtx.Execute(key.FocusCmd{Tag: &f.importOpen})
						f.restore = nil
					}
					return view.PrimaryButton(&f.importOpen, l.Text(i18n.PositionImportPortfolioAction))(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.request != nil {
						gtx = gtx.Disabled()
					}
					return view.Button(&p.reload, l.Text(reload))(gtx)
				}),
			)
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.id != "" {
				return p.layoutInstrumentDetailHeader(gtx, view)
			}
			if p.current == nil || p.current.summary.Currency == "" {
				return p.layoutPortfolioTabs(gtx, view)
			}
			return (ui.Column{Gap: ui.Space12}).Layout(gtx,
				ui.Rigid(portfolioSummary(view, p.current.summary)),
				ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutPortfolioTabs(gtx, view) }),
			)
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.id != "" || p.current == nil {
				return layout.Dimensions{}
			}
			switch p.tabs.active {
			case portfolioInstruments:
				return p.catalogControls(gtx, view)
			case portfolioAllocationTab:
				return layout.Dimensions{}
			default:
				return p.holdingsControls(gtx, view)
			}
		}),
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			if p.current == nil {
				return layout.Dimensions{}
			}
			rows := p.rows(view)
			gap := ui.Space12
			if p.id == "" && p.tabs.active == portfolioHoldingsTab && ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, holdingTableWidth) {
				gap = ui.Space4
			}
			return pageListGap(gtx, view, &p.list, l.Text(i18n.NavPortfolio), len(rows), gap, func(gtx layout.Context, i int) layout.Dimensions {
				return rows[i](gtx)
			})
		}),
	)
}

func (p *portfolioPage) canImportPosition() bool {
	return p.id == "" && p.tabs.active == portfolioHoldingsTab && p.investment != nil && p.investment.onPosted != nil && p.current != nil && len(p.current.importAccounts) != 0
}

func (p *portfolioPage) rows(view ui.View) []layout.Widget {
	if p.id != "" {
		return p.instrumentRows(view)
	}
	switch p.tabs.active {
	case portfolioInstruments:
		return p.catalogRows(view)
	case portfolioAllocationTab:
		return p.allocationRows(view, p.current.summary)
	default:
		return p.holdingsRows(view)
	}
}

func portfolioSummary(view ui.View, summary domain.PortfolioValuation) layout.Widget {
	l := view.Language
	quality := summary.DisplayValuation.Quality
	status, tone := i18n.OverviewCurrentEstimate, ui.BadgeSuccess
	if !summary.Complete || quality.Availability != "" && quality.Availability != domain.ValuationFull {
		status, tone = i18n.OverviewIncomplete, ui.BadgeWarning
	}
	caption, amount := overviewMoneyText(view, i18n.PortfolioCurrentValue, summary.DisplayValuation)
	if quality.Availability == "" && summary.ValuedSubtotal != nil {
		amount = valuationMoney(view, *summary.ValuedSubtotal, false)
	}
	value := func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: ui.Space2}).Layout(gtx,
			ui.Rigid(view.Body(ui.TextCaption, caption, view.Colors.SecondaryInk)),
			ui.Rigid(view.Body(ui.TextMoneyLG, amount, view.Colors.Ink)),
		)
	}
	evidence := func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: ui.Space4}).Layout(gtx,
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				base := fmt.Sprintf(l.Text(i18n.OverviewBaseCurrency), summary.Currency)
				return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return view.Badge(gtx, tone, l.Text(status)) }),
					layout.Rigid(view.Body(ui.TextCaption, base, view.Colors.SecondaryInk)),
				)
			}),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return overviewEvidence(gtx, view, quality, i18n.OverviewQuoteDates)
			}),
		)
	}
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: ui.Space12, Bottom: ui.Space12, Left: ui.Space16, Right: ui.Space16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				rows := []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions {
					if ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, 465) {
						return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
							layout.Flexed(1, value), layout.Rigid(evidence))
					}
					return (ui.Column{Gap: ui.Space8}).Layout(gtx, ui.Rigid(value), ui.Rigid(evidence))
				})}
				if !summary.Complete && len(summary.MissingInputs) != 0 {
					note := fmt.Sprintf(l.Text(i18n.PortfolioSummaryCoverage), len(summary.MissingInputs))
					rows = append(rows, ui.Rigid(view.Body(ui.TextCaption, note, view.Colors.WarningInk)))
				}
				return (ui.Column{Gap: ui.Space4}).Layout(gtx, rows...)
			})
		})
	}
}

func portfolioButton[ID comparable](buttons map[ID]*widget.Clickable, id ID) *widget.Clickable {
	if buttons[id] == nil {
		buttons[id] = new(widget.Clickable)
	}
	return buttons[id]
}

func (p *portfolioPage) instrumentRows(view ui.View) []layout.Widget {
	r, l := p.current, view.Language
	if r.managed {
		if r.product == nil {
			return []layout.Widget{view.Message(l.Text(i18n.ProductUnavailable), view.Colors.SecondaryInk)}
		}
		product := r.product
		name := productName(*product)
		fields := []layout.Widget{productState(view, product.Contract.Kind, product.DisplayState), fundsAmount(view, i18n.ProductValue, product.CurrentValue)}
		if p.openProduct != nil {
			fields = append(fields, productLink(view, product.Contract.ID, name, portfolioButton(p.productButtons, product.Contract.ID), p.openProduct))
		}
		return []layout.Widget{fundsCard(view, name, fields...)}
	}
	i := r.instrument
	if i == nil {
		return []layout.Widget{view.Message(l.Text(i18n.PortfolioInstrumentUnavailable), view.Colors.SecondaryInk)}
	}
	switch p.detail.visit.section {
	case instrumentDetailPerformance:
		return p.instrumentPerformanceRows(view, *i)
	case instrumentDetailData:
		return p.instrumentDataRows(view, *i)
	}
	var rows []layout.Widget
	if actions := p.instrumentRelatedActions(view, *i); actions != nil {
		rows = append(rows, actions)
	}
	if len(r.positions) != 0 {
		rows = append(rows, holdingAmounts(view, r.positions[0].Amounts, i.QuantityUnit))
	} else {
		rows = append(rows, view.Message(l.Text(i18n.PortfolioEmpty), view.Colors.SecondaryInk))
	}
	price := l.Text(i18n.PortfolioPriceMissing)
	if q := r.price; q != nil {
		price = l.Text(i18n.PortfolioInstrumentLatestQuote) + " · " + instrumentCurrentQuotePrice(view, *i, *q) + " · " +
			instrumentCurrentQuoteEvidence(view, *q)
	}
	rows = append(rows, view.Message(price, view.Colors.Ink))
	if i.QuoteCurrency != r.base {
		fx := fmt.Sprintf(l.Text(i18n.PortfolioFXMissing), i.QuoteCurrency, r.base)
		if q := r.fx; q != nil {
			fx = fmt.Sprintf(l.Text(i18n.PortfolioMarketFX), q.BaseCurrency, portfolioDecimal(view, q.Rate.Canonical()), q.QuoteCurrency) +
				portfolioQuoteStamp(view, q.SourceKey, q.EffectiveDate, q.QuotedAt)
		}
		rows = append(rows, view.Message(fx, view.Colors.Ink))
	}
	if len(r.positions) != 0 {
		for _, holding := range r.positions[0].Holdings {
			rows = append(rows, func(gtx layout.Context) layout.Dimensions {
				button := portfolioButton(p.accounts, holding.AccountID)
				if p.openAccount == nil {
					return view.Message(holding.AccountName, view.Colors.Ink)(gtx)
				}
				if button.Clicked(gtx) {
					p.openAccount(holding.AccountID, holding.AccountName, button)
					gtx.Execute(op.InvalidateCmd{})
				}
				label := fmt.Sprintf(l.Text(i18n.HistoryOpenAccount), holding.AccountName)
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.W.Layout(gtx, view.Button(button, label)) }),
					layout.Rigid(holdingAmounts(view, holding.Amounts, i.QuantityUnit)),
				)
			})
		}
	}
	return rows
}

func portfolioDecimal(view ui.View, canonical string) string {
	if view.DecimalSeparator == settings.DecimalComma {
		return strings.Replace(canonical, ".", ",", 1)
	}
	return canonical
}

func portfolioQuoteStamp(view ui.View, source, effectiveDate string, at time.Time) string {
	stamp := ""
	if day, ok := ui.ParseCivilDay(effectiveDate); ok {
		stamp = ui.FormatDay(day, view.Language, view.DateFormat)
	} else if !at.IsZero() {
		stamp = format.LocalDateTime(at.In(view.TimeLocation), settings.Settings{
			DateFormat: view.DateFormat, TimeFormat: view.TimeFormat,
		}, settings.Language(view.Language))
	}
	if source != "" {
		source = " · " + source
	}
	if stamp != "" {
		return source + " · " + stamp
	}
	return source
}
