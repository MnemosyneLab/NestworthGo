package desktop

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type portfolioCatalogLedger interface {
	portfolioReader
	WithWrite(context.Context, func(context.Context) error) error
	CreateInstrument(context.Context, application.InstrumentInput) (domain.Instrument, error)
	ArchiveInstrument(context.Context, domain.InstrumentID, bool) error
	PatchInstrument(context.Context, domain.InstrumentID, application.InstrumentPatch) (domain.Instrument, error)
	SearchInstruments(context.Context, string, string) ([]application.InstrumentSearchHit, error)
	PreviewMetalQuote(context.Context, string, string, string) (application.LatestInstrumentQuote, string, error)
}

type portfolioCatalogRead struct {
	instruments []domain.Instrument
	managed     map[string]string
	quotes      map[domain.InstrumentID]*domain.InstrumentQuote
	quoteErrors map[domain.InstrumentID]bool
}

type portfolioCatalogVisit struct {
	query string
}

type portfolioCatalog struct {
	ledger          portfolioCatalogLedger
	search          ui.SearchField
	add             widget.Clickable
	links           map[domain.InstrumentID]*portfolioCatalogActions
	draft           *portfolioCatalogDraft
	notice          i18n.MessageID
	focus           *widget.Clickable
	requestedEditor *domain.InstrumentID
	finishExternal  func(i18n.MessageID)
}

type portfolioCatalogActions struct {
	open, manage widget.Clickable
}

type portfolioCatalogDraft struct {
	instrument domain.Instrument
	returnTo   *widget.Clickable
	sheet      analysisSheet
	overlay    ui.Overlay
	archive    widget.Clickable
	cancel     widget.Clickable
	confirm    widget.Clickable
	save       widget.Clickable
	fields     [portfolioIdentityFieldCount]ui.TextField
	config     portfolioInstrumentConfig
	confirming bool
	external   bool
	focusName  bool
	opening    bool
	issue      i18n.MessageID
	pending    <-chan historyResult[i18n.MessageID]
}

func readPortfolioCatalog(ctx context.Context, service portfolioReader) (out portfolioRead, err error) {
	err = service.WithRead(ctx, func(ctx context.Context) error {
		catalog := new(portfolioCatalogRead)
		catalog.instruments, err = service.ListInstruments(ctx, true)
		if err != nil {
			return err
		}
		catalog.managed, err = service.InstrumentProductOwnership(ctx)
		if err != nil {
			return err
		}
		catalog.quotes = make(map[domain.InstrumentID]*domain.InstrumentQuote, len(catalog.instruments))
		for _, item := range catalog.instruments {
			quote, quoteErr := service.CurrentInstrumentQuote(ctx, item.ID)
			if quoteErr != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if catalog.quoteErrors == nil {
					catalog.quoteErrors = make(map[domain.InstrumentID]bool)
				}
				catalog.quoteErrors[item.ID] = true
			} else if quote != nil {
				catalog.quotes[item.ID] = quote
			}
		}
		out.summary, err = service.Portfolio(ctx, domain.AccountFilter{})
		if err == nil {
			out.catalog = catalog
		}
		return err
	})
	return out, err
}

func (c *portfolioCatalog) visit() portfolioCatalogVisit {
	return portfolioCatalogVisit{query: c.search.Field.Text()}
}

func (c *portfolioCatalog) restore(visit portfolioCatalogVisit) {
	c.draft, c.notice, c.focus, c.requestedEditor = nil, i18n.NoMessage, nil, nil
	c.search.Field.Editor.SetText(visit.query)
}

func (p *portfolioPage) requestCatalogEditor(item domain.Instrument) {
	id := item.ID
	p.catalog.requestedEditor = &id
}

func (p *portfolioPage) beginRequestedCatalogEditor(gtx layout.Context) {
	c := &p.catalog
	if c.requestedEditor == nil || p.id != "" || p.tabs.active != portfolioInstruments ||
		p.request != nil || p.err != nil || p.current == nil || p.current.catalog == nil {
		return
	}
	id := *c.requestedEditor
	c.requestedEditor = nil
	if c.ledger == nil {
		return
	}
	item, trigger := domain.Instrument{QuoteCurrency: p.current.summary.Currency}, &c.add
	if id != "" {
		catalog := p.current.catalog
		index := slices.IndexFunc(catalog.instruments, func(item domain.Instrument) bool { return item.ID == id })
		if index < 0 || catalog.managed[id.String()] != "" {
			c.notice = i18n.PortfolioInstrumentUnavailable
			return
		}
		item, trigger = catalog.instruments[index], &c.rowActions(id).manage
	}
	p.beginCatalogEditor(gtx, item, trigger)
	c.draft.external = true
}

func (p *portfolioPage) beginCatalogEditor(gtx layout.Context, item domain.Instrument, trigger *widget.Clickable) {
	p.catalog.draft, p.catalog.notice = newPortfolioCatalogDraft(item, trigger), i18n.NoMessage
	gtx.Execute(op.InvalidateCmd{})
}

func (c *portfolioCatalog) rowActions(id domain.InstrumentID) *portfolioCatalogActions {
	actions := c.links[id]
	if actions == nil {
		actions = new(portfolioCatalogActions)
		c.links[id] = actions
	}
	return actions
}

func (p *portfolioPage) catalogControls(gtx layout.Context, view ui.View) layout.Dimensions {
	c := &p.catalog
	if gtx.Enabled() && p.current.catalog != nil && p.request == nil && p.err == nil && c.focus != nil && c.focus != &c.add {
		query := normalizeDirectoryQuery(c.search.Field.Text())
		found := slices.ContainsFunc(p.current.catalog.instruments, func(item domain.Instrument) bool {
			actions := c.links[item.ID]
			return actions != nil && c.focus == &actions.manage && catalogMatches(item, query)
		})
		if !found {
			c.search.Field.Focus(gtx)
			c.focus = nil
		}
	}
	return (ui.Column{Gap: ui.Space8}).Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return c.search.Layout(gtx, view, i18n.MarketSearch, i18n.MarketSearchHint, func(string) {
				p.list.Position = layout.Position{}
				gtx.Execute(op.InvalidateCmd{})
			})
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if c.notice == i18n.NoMessage {
				return layout.Dimensions{}
			}
			return view.Message(view.Language.Text(c.notice), view.Colors.Ink)(gtx)
		}))
}

func (p *portfolioPage) catalogRows(view ui.View) []layout.Widget {
	c, l := &p.catalog, view.Language
	catalog := p.current.catalog
	if catalog == nil {
		return nil
	}
	query := normalizeDirectoryQuery(c.search.Field.Text())
	grouped := make(map[domain.InstrumentType][]domain.Instrument)
	for _, item := range sortPortfolioCatalogInstruments(catalog.instruments, l) {
		if catalogMatches(item, query) {
			grouped[item.Type] = append(grouped[item.Type], item)
		}
	}
	var rows []layout.Widget
	for _, kind := range domain.AllInstrumentTypes() {
		items := grouped[kind]
		if len(items) == 0 {
			continue
		}
		if len(rows) > 0 {
			rows = append(rows, portfolioSpacer(ui.Space12))
		}
		rows = append(rows, view.Body(ui.TextHeading, l.Text(marketInstrumentTypes[kind]), view.Colors.Ink))
		rows = append(rows, portfolioSpacer(ui.Space8))
		for i, item := range items {
			rows = append(rows, p.catalogRow(view, item, catalog, i == 0, i == len(items)-1))
		}
	}
	if len(rows) == 0 {
		label := i18n.MarketNoInstruments
		if len(catalog.instruments) > 0 {
			label = i18n.MarketNoMatches
		}
		return []layout.Widget{emptyState(view, l.Text(label))}
	}
	return rows
}

func portfolioSpacer(height unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Spacer{Height: height}.Layout(gtx)
	}
}

// Wails groups by domain type, then sorts each group by quote currency and
// display label. Sorting the flattened slice first produces the same ordering
// while retaining the existing type-section layout.
func sortPortfolioCatalogInstruments(items []domain.Instrument, locale i18n.Language) []domain.Instrument {
	result := slices.Clone(items)
	types := domain.AllInstrumentTypes()
	col := collate.New(language.Make(string(locale)))
	slices.SortStableFunc(result, func(a, b domain.Instrument) int {
		aType, bType := slices.Index(types, a.Type), slices.Index(types, b.Type)
		if aType < 0 {
			aType = len(types)
		}
		if bType < 0 {
			bType = len(types)
		}
		if order := cmp.Compare(aType, bType); order != 0 {
			return order
		}
		if a.Type != b.Type {
			return col.CompareString(string(a.Type), string(b.Type))
		}
		if order := col.CompareString(a.QuoteCurrency.String(), b.QuoteCurrency.String()); order != 0 {
			return order
		}
		return col.CompareString(a.DisplayLabel(), b.DisplayLabel())
	})
	return result
}

func catalogMatches(item domain.Instrument, query string) bool {
	var fields []string
	for _, value := range []string{item.ShortName, item.Name, productString(item.Symbol), productString(item.ISIN),
		productString(item.MarketCode), productString(item.ProviderSymbol), item.QuoteCurrency.String()} {
		if value = strings.TrimSpace(value); value != "" {
			fields = append(fields, value)
		}
	}
	facts := strings.Join(fields, " ")
	return strings.Contains(normalizeDirectoryQuery(facts), query)
}

func (p *portfolioPage) catalogRow(view ui.View, item domain.Instrument, catalog *portfolioCatalogRead, first, last bool) layout.Widget {
	c, l := &p.catalog, view.Language
	actions := c.rowActions(item.ID)
	title := item.DisplayLabel()
	caption := ""
	if item.Name != title {
		caption = item.Name
	}
	managed := catalog.managed[item.ID.String()] != ""
	badges := marketBadges(view, item.QuoteCurrency.String(), item.QuoteSource)
	if source := instrumentQuoteSources[item.QuoteSource]; source != i18n.NoMessage && len(badges) > 0 {
		badges[len(badges)-1].text = l.Text(source)
	}
	if item.ArchivedAt != nil {
		badges = append(badges, marketBadge{ui.BadgeNeutral, l.Text(i18n.DirectoryArchived)})
	}
	if managed {
		badges = append(badges, marketBadge{ui.BadgeNeutral, l.Text(i18n.ManagedProducts)})
	}
	icon := directoryGlyph(productString(item.IconKey), domain.DefaultInstrumentIcon(item.Type))
	if icon == nil {
		icon = lucide.TrendingUp
	}
	value, evidence := l.Text(i18n.MarketNoCurrentPrice), ""
	if q := catalog.quotes[item.ID]; q != nil {
		value = fmt.Sprintf(l.Text(i18n.MarketLatestPrice), instrumentCurrentQuotePrice(view, item, *q))
		evidence = instrumentCatalogQuoteEvidence(view, item, *q)
	} else if catalog.quoteErrors[item.ID] {
		value = l.Text(i18n.PortfolioCatalogQuoteUnavailable)
	}
	return func(gtx layout.Context) layout.Dimensions {
		if p.request != nil || p.err != nil {
			gtx = gtx.Disabled()
		}
		var buttons []layout.Widget
		var actionLabels []string
		if p.openInstrument != nil {
			if actions.open.Clicked(gtx) {
				p.openInstrument(item.ID, title, &actions.open)
				gtx.Execute(op.InvalidateCmd{})
			}
			label := fmt.Sprintf(l.Text(i18n.PortfolioCatalogOpen), title)
			buttons = append(buttons, view.ButtonOf(&actions.open, ui.ButtonSpec{
				Variant: ui.ButtonLink, Size: ui.ButtonSmall, Title: l.Text(i18n.PortfolioCatalogDetails), Name: label,
			}))
			actionLabels = append(actionLabels, l.Text(i18n.PortfolioCatalogDetails))
		}
		if c.ledger != nil && !managed {
			if c.focus == &actions.manage && gtx.Enabled() {
				gtx.Execute(key.FocusCmd{Tag: &actions.manage})
				c.focus = nil
			}
			if actions.manage.Clicked(gtx) {
				p.beginCatalogEditor(gtx, item, &actions.manage)
			}
			label := fmt.Sprintf(l.Text(i18n.PortfolioCatalogEditNamed), title)
			buttons = append(buttons, view.ButtonOf(&actions.manage, ui.ButtonSpec{
				Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: l.Text(i18n.InstitutionsEdit), Name: label,
			}))
			actionLabels = append(actionLabels, l.Text(i18n.InstitutionsEdit))
		}
		return marketQuoteRow(view, first, last, marketRowSpec{
			icon: icon, name: title, subtitle: caption, badges: badges, value: value, evidence: evidence,
			catalogActions: true, actionLabels: actionLabels,
		}, buttons)(gtx)
	}
}

func instrumentCatalogQuoteEvidence(view ui.View, instrument domain.Instrument, quote domain.InstrumentQuote) string {
	l := view.Language
	var parts []string
	day := ""
	if quote.TimestampBasis == "date_label" {
		day = quote.EffectiveDate
	}
	if stamp := strings.TrimPrefix(portfolioQuoteStamp(view, "", day, quote.QuotedAt), " · "); stamp != "" {
		parts = append(parts, fmt.Sprintf(l.Text(i18n.PortfolioInstrumentQuoteAsOf), stamp))
	}
	if quote.SourceKey != "" && quote.SourceKey != string(quote.SourceKind) {
		source := l.Text(instrumentQuoteSources[quote.SourceKind])
		if source == "" {
			source = l.Text(i18n.MarketNoSource)
		}
		parts = append(parts, source+" · "+quote.SourceKey)
	} else if quote.SourceKind != instrument.QuoteSource {
		source := l.Text(instrumentQuoteSources[quote.SourceKind])
		if source == "" {
			source = l.Text(i18n.MarketNoSource)
		}
		parts = append(parts, source)
	}
	if quote.Delayed {
		parts = append(parts, l.Text(i18n.MarketDelayed))
	}
	return strings.Join(parts, " · ")
}

func (p *portfolioPage) saveCatalogInstrument(archive bool) {
	c, d := &p.catalog, p.catalog.draft
	d.issue = i18n.NoMessage
	input := d.instrument
	create := input.ID == ""
	values := d.createInput()
	patch := d.instrumentPatch()
	d.config.retireReads()
	d.pending = runHistoryRead(&p.workers, p.invalidate, func() (i18n.MessageID, error) {
		if create {
			_, err := c.ledger.CreateInstrument(p.ctx, values)
			return i18n.PortfolioCatalogCreated, err
		}
		notice := i18n.PortfolioCatalogSaved
		err := c.ledger.WithWrite(p.ctx, func(ctx context.Context) error {
			current, err := c.ledger.ListInstruments(ctx, true)
			if err != nil {
				return err
			}
			for _, item := range current {
				if item.ID != input.ID {
					continue
				}
				if !reflect.DeepEqual(item, input) {
					return &domain.Error{Code: domain.ErrStalePreview}
				}
				if archive {
					notice = i18n.DirectoryArchiveDone
					if input.ArchivedAt != nil {
						notice = i18n.DirectoryRestored
					}
					return c.ledger.ArchiveInstrument(ctx, input.ID, input.ArchivedAt == nil)
				}
				_, err := c.ledger.PatchInstrument(ctx, input.ID, patch)
				return err
			}
			return &domain.Error{Code: domain.ErrNotFound}
		})
		return notice, err
	})
}

func (p *portfolioPage) pollCatalogDraft() {
	c, d := &p.catalog, p.catalog.draft
	if d == nil {
		return
	}
	select {
	case result := <-d.pending:
		d.pending = nil
		if result.err == nil {
			c.notice = result.value
			c.draft, c.focus = nil, d.returnTo
			if d.external && c.finishExternal != nil {
				c.finishExternal(result.value)
				return
			}
			p.read()
			return
		}
		d.issue = i18n.PortfolioCatalogWriteFailed
		d.sheet.list.Position = layout.Position{}
		var issue *domain.Error
		if errors.As(result.err, &issue) {
			switch {
			case issue.Code == domain.ErrValidation:
				d.issue = catalogInstrumentValidation(issue.Field)
				d.focusName = d.issue == i18n.PortfolioCatalogNameRequired
			case issue.Code == domain.ErrConflict && issue.Field == "quantity":
				d.issue = i18n.PortfolioCatalogNonzero
			case issue.Code == domain.ErrStalePreview || issue.Code == domain.ErrNotFound:
				d.issue = i18n.PortfolioCatalogChanged
				p.read()
			}
		}
	default:
	}
}
