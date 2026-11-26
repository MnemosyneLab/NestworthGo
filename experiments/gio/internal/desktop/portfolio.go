package desktop

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"gioui.org/font"
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

	search                ui.SearchField
	list                  widget.List
	reload                widget.Clickable
	history               widget.Clickable
	instruments           map[domain.InstrumentID]*widget.Clickable
	accounts              map[domain.AccountID]*widget.Clickable
	openInstrument        func(domain.InstrumentID, string, event.Tag)
	openHoldingInstrument func(domain.InstrumentID, domain.AccountID, string, event.Tag)
	openAccount           func(domain.AccountID, string, event.Tag)
	openHistory           func(domain.InstrumentID, string, event.Tag)
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
	catalog := id == "" && p.catalog.active
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (portfolioRead, error) {
		if catalog {
			return readPortfolioCatalog(ctx, p.service)
		}
		return readPortfolio(ctx, p.service, id, account)
	})
}

func (p *portfolioPage) update() {
	select {
	case result := <-p.request:
		p.leave()
		p.err = result.err
		if p.err == nil {
			p.current = &result.value
		}
	default:
	}
}

func (p *portfolioPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.update()
	p.pollCatalogDraft()
	if p.catalog.draft != nil {
		return p.layoutCatalogDraft(gtx, view)
	}
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
				return layout.Dimensions{}
			}
			return p.catalogTabs(gtx, view)
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.id != "" || p.current == nil {
				return layout.Dimensions{}
			}
			if p.catalog.active {
				return p.catalogControls(gtx, view)
			}
			return p.holdingsControls(gtx, view)
		}),
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			if p.current == nil {
				return layout.Dimensions{}
			}
			rows := p.rows(view)
			return pageListGap(gtx, view, &p.list, l.Text(i18n.NavPortfolio), len(rows), ui.Space12, func(gtx layout.Context, i int) layout.Dimensions {
				return rows[i](gtx)
			})
		}),
	)
}

func (p *portfolioPage) canImportPosition() bool {
	return p.id == "" && !p.catalog.active && p.investment != nil && p.investment.onPosted != nil && p.current != nil && len(p.current.importAccounts) != 0
}

func (p *portfolioPage) rows(view ui.View) []layout.Widget {
	l := view.Language
	if p.id != "" {
		return p.instrumentRows(view)
	}
	summary := p.current.summary
	var rows []layout.Widget
	if summary.Currency != "" {
		status := i18n.OverviewCurrentEstimate
		if !summary.Complete || summary.DisplayValuation.Quality.Availability != domain.ValuationFull {
			status = i18n.OverviewIncomplete
		}
		tone := ui.BadgeSuccess
		if status == i18n.OverviewIncomplete {
			tone = ui.BadgeWarning
		}
		caption, display := overviewMoneyText(view, i18n.PortfolioValue, summary.DisplayValuation)
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space4)}.Layout(gtx,
									layout.Rigid(view.Body(ui.TextCaption, caption, view.Colors.SecondaryInk)),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return view.Line(gtx, ui.TextMoneyLG, display, view.Colors.Ink)
									}),
								)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return view.Badge(gtx, tone, l.Text(status)) }),
						)
					}),
					layout.Rigid(view.Body(ui.TextCaption, fmt.Sprintf(l.Text(i18n.OverviewBaseCurrency), summary.Currency), view.Colors.SecondaryInk)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return overviewEvidence(gtx, view, summary.DisplayValuation.Quality, i18n.OverviewQuoteDates)
					}),
				)
			})
		})
	}
	if p.catalog.active {
		return append(rows, p.catalogRows(view)...)
	}
	return append(rows, p.holdingsRows(view)...)
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
	name := i.DisplayLabel()
	rows := []layout.Widget{func(gtx layout.Context) layout.Dimensions {
		text := name
		if i.ArchivedAt != nil {
			text += " · " + l.Text(i18n.DirectoryArchived)
		}
		return view.Label(gtx, text, 23, font.Medium, view.Colors.Ink)
	}}
	if fullName := strings.TrimSpace(i.Name); fullName != "" && fullName != name {
		rows = append(rows, view.Message(fullName, view.Colors.SecondaryInk))
	}
	if p.openHistory != nil {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			if p.history.Clicked(gtx) {
				p.openHistory(i.ID, name, &p.history)
				gtx.Execute(op.InvalidateCmd{})
			}
			return layout.W.Layout(gtx, view.Button(&p.history, l.Text(i18n.PortfolioInstrumentHistory)))
		})
	}
	if len(r.positions) != 0 {
		rows = append(rows, holdingAmounts(view, r.positions[0].Amounts, i.QuantityUnit))
	} else {
		rows = append(rows, view.Message(l.Text(i18n.PortfolioEmpty), view.Colors.SecondaryInk))
	}
	price := l.Text(i18n.PortfolioPriceMissing)
	if q := r.price; q != nil {
		price = fmt.Sprintf(l.Text(i18n.PortfolioCurrentPrice), q.Currency, portfolioDecimal(view, q.UnitPrice.Canonical())) +
			portfolioQuoteStamp(view, q.SourceKey, q.EffectiveDate, q.QuotedAt)
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
