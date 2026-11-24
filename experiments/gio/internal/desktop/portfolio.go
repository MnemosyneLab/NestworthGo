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
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type portfolioReader interface {
	WithRead(context.Context, func(context.Context) error) error
	Portfolio(context.Context, domain.AccountFilter) (domain.PortfolioValuation, error)
	InstrumentHoldings(context.Context) ([]domain.InstrumentHoldingsView, error)
	Bootstrap(context.Context) (application.Bootstrap, error)
	ListInstruments(context.Context, bool) ([]domain.Instrument, error)
	CurrentInstrumentQuote(context.Context, domain.InstrumentID) (*domain.InstrumentQuote, error)
	CurrentFXQuote(context.Context, domain.CurrencyCode, domain.CurrencyCode) (*domain.FXQuote, error)
}

type portfolioRead struct {
	summary    domain.PortfolioValuation
	positions  []domain.InstrumentHoldingsView
	instrument *domain.Instrument
	price      *domain.InstrumentQuote
	fx         *domain.FXQuote
	base       domain.CurrencyCode
}

func readPortfolio(ctx context.Context, service portfolioReader, id domain.InstrumentID) (out portfolioRead, err error) {
	err = service.WithRead(ctx, func(ctx context.Context) error {
		var err error
		if id == "" {
			out.summary, err = service.Portfolio(ctx, domain.AccountFilter{})
			if err != nil {
				return err
			}
		} else {
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
		}
		out.positions, err = service.InstrumentHoldings(ctx)
		if err == nil && id != "" {
			out.positions = slices.DeleteFunc(out.positions, func(row domain.InstrumentHoldingsView) bool { return row.InstrumentID != id })
		}
		return err
	})
	return out, err
}

type portfolioPage struct {
	ctx        context.Context
	service    portfolioReader
	invalidate func()
	workers    sync.WaitGroup
	id         domain.InstrumentID
	cancel     context.CancelFunc
	request    <-chan historyResult[portfolioRead]
	current    *portfolioRead
	err        error

	search         ui.SearchField
	list           widget.List
	reload         widget.Clickable
	history        widget.Clickable
	instruments    map[domain.InstrumentID]*widget.Clickable
	accounts       map[domain.AccountID]*widget.Clickable
	openInstrument func(domain.InstrumentID, string, event.Tag)
	openAccount    func(domain.AccountID, string, event.Tag)
	openHistory    func(domain.InstrumentID, string, event.Tag)
}

func newPortfolioPage(ctx context.Context, service portfolioReader, invalidate func()) *portfolioPage {
	return &portfolioPage{ctx: ctx, service: service, invalidate: invalidate,
		search: ui.SearchField{Field: ui.NewTextField(true)}, list: widget.List{List: layout.List{Axis: layout.Vertical}},
		instruments: make(map[domain.InstrumentID]*widget.Clickable), accounts: make(map[domain.AccountID]*widget.Clickable)}
}

func (p *portfolioPage) enter(id domain.InstrumentID, query string, position layout.Position) {
	p.id, p.current = id, nil
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
	id := p.id
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (portfolioRead, error) {
		return readPortfolio(ctx, p.service, id)
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
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(12)}.Layout(ui.LimitWidth(gtx, 760),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if status == i18n.NoMessage {
						return layout.Dimensions{}
					}
					color := view.Colors.SecondaryInk
					if p.err != nil {
						color = view.Colors.ErrorInk
					}
					return view.Message(l.Text(status), color)(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.request != nil {
						gtx = gtx.Disabled()
					}
					return view.Button(&p.reload, l.Text(reload))(gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.id != "" {
				return layout.Dimensions{}
			}
			return p.search.Layout(ui.LimitWidth(gtx, 440), view, i18n.PortfolioSearch, i18n.PortfolioSearchHint, func(string) {
				p.list.Position = layout.Position{}
				gtx.Execute(op.InvalidateCmd{})
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if p.current == nil {
				return layout.Dimensions{}
			}
			rows := p.rows(view)
			return material.List(view.Theme, &p.list).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Bottom: 10}.Layout(gtx, rows[i])
			})
		}),
	)
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
		rows = append(rows, view.Message(l.Text(status), view.Colors.SecondaryInk),
			overviewAmount(view, i18n.PortfolioValue, summary.DisplayValuation, 25),
			view.Message(fmt.Sprintf(l.Text(i18n.OverviewBaseCurrency), summary.Currency), view.Colors.SecondaryInk),
			func(gtx layout.Context) layout.Dimensions {
				return overviewEvidence(gtx, view, summary.DisplayValuation.Quality)
			})
	}
	active := func(row domain.InstrumentHoldingsView) bool { return row.Amounts.Quantity != "0" }
	if !slices.ContainsFunc(p.current.positions, active) {
		return append(rows, view.Message(l.Text(i18n.PortfolioEmpty), view.Colors.SecondaryInk))
	}

	query := normalizeDirectoryQuery(p.search.Field.Text())
	matched := false
	for _, row := range p.current.positions {
		name := domain.InstrumentDisplayLabel(row.Name, row.Symbol, row.ShortName)
		if !active(row) || !strings.Contains(normalizeDirectoryQuery(strings.Join([]string{row.Name, row.Symbol, row.ShortName}, " ")), query) {
			continue
		}
		matched = true
		button := portfolioButton(p.instruments, row.InstrumentID)
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return p.positionCard(gtx, view, row, name, button)
		})
	}
	if !matched {
		rows = append(rows, view.Message(l.Text(i18n.PortfolioNoMatches), view.Colors.SecondaryInk))
	}
	return rows
}

func portfolioButton[ID comparable](buttons map[ID]*widget.Clickable, id ID) *widget.Clickable {
	if buttons[id] == nil {
		buttons[id] = new(widget.Clickable)
	}
	return buttons[id]
}

func (p *portfolioPage) positionCard(gtx layout.Context, view ui.View, row domain.InstrumentHoldingsView, name string, button *widget.Clickable) layout.Dimensions {
	if button.Clicked(gtx) && p.openInstrument != nil {
		p.openInstrument(row.InstrumentID, name, button)
		gtx.Execute(op.InvalidateCmd{})
	}
	fullName := strings.TrimSpace(row.Name)
	showFullName := fullName != "" && fullName != name
	if row.Archived {
		name += " · " + view.Language.Text(i18n.DirectoryArchived)
	}
	return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.W.Layout(gtx, view.Button(button, name)) }),
			}
			if showFullName {
				children = append(children, layout.Rigid(view.Message(fullName, view.Colors.SecondaryInk)))
			}
			children = append(children, layout.Rigid(holdingAmounts(view, row.Amounts, row.QuantityUnit)))
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx, children...)
		})
	})
}

func (p *portfolioPage) instrumentRows(view ui.View) []layout.Widget {
	r, l := p.current, view.Language
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

func holdingAmounts(view ui.View, amounts domain.HoldingAmounts, unit string) layout.Widget {
	l := view.Language
	display := func(money *domain.MoneyView) string {
		if money == nil {
			return l.Text(i18n.OverviewNoValue)
		}
		formatted := valuationMoney(view, *money, false)
		code := money.Currency.String() + " "
		if strings.HasPrefix(formatted, code) {
			return formatted
		}
		return code + formatted
	}
	quantity := portfolioDecimal(view, amounts.Quantity)
	if unit != "" {
		quantity += " " + unit
	}
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(3)}.Layout(gtx,
			layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.AccountQuantity), quantity), view.Colors.Ink)),
			layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.PortfolioCost), display(amounts.TotalCost)), view.Colors.Ink)),
			layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.PortfolioNativeValue), display(amounts.CurrentValue)), view.Colors.Ink)),
		)
	}
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
