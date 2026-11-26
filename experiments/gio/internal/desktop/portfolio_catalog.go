package desktop

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type portfolioCatalogLedger interface {
	portfolioReader
	WithWrite(context.Context, func(context.Context) error) error
	ArchiveInstrument(context.Context, domain.InstrumentID, bool) error
	PatchInstrument(context.Context, domain.InstrumentID, application.InstrumentPatch) (domain.Instrument, error)
}

type portfolioCatalogRead struct {
	instruments []domain.Instrument
	managed     map[string]string
}

type portfolioCatalogVisit struct {
	active bool
	query  string
	other  layout.Position
}

type portfolioCatalog struct {
	ledger   portfolioCatalogLedger
	active   bool
	other    layout.Position
	search   ui.SearchField
	holdings widget.Clickable
	tab      widget.Clickable
	links    map[domain.InstrumentID]*portfolioCatalogActions
	draft    *portfolioCatalogDraft
	notice   i18n.MessageID
	focus    *widget.Clickable
}

type portfolioCatalogActions struct {
	open, manage widget.Clickable
}

type portfolioCatalogDraft struct {
	instrument            domain.Instrument
	returnTo              *widget.Clickable
	archive, back, cancel widget.Clickable
	confirm               widget.Clickable
	save                  widget.Clickable
	fields                [portfolioIdentityFieldCount]ui.TextField
	confirming            bool
	issue                 i18n.MessageID
	pending               <-chan historyResult[i18n.MessageID]
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
		out.summary, err = service.Portfolio(ctx, domain.AccountFilter{})
		if err == nil {
			out.catalog = catalog
		}
		return err
	})
	return out, err
}

func (c *portfolioCatalog) visit() portfolioCatalogVisit {
	return portfolioCatalogVisit{active: c.active, query: c.search.Field.Text(), other: c.other}
}

func (c *portfolioCatalog) restore(visit portfolioCatalogVisit) {
	c.active, c.other, c.draft, c.notice, c.focus = visit.active, visit.other, nil, i18n.NoMessage, nil
	c.search.Field.Editor.SetText(visit.query)
}

func (p *portfolioPage) catalogTabs(gtx layout.Context, view ui.View) layout.Dimensions {
	c, l := &p.catalog, view.Language
	if c.holdings.Clicked(gtx) && c.active || c.tab.Clicked(gtx) && !c.active {
		c.active = !c.active
		p.list.Position, c.other = c.other, p.list.Position
		p.current = nil
		p.read()
		view.RestorePageScroll(0)
		gtx.Execute(op.InvalidateCmd{})
	}
	return view.TabTrack(gtx, 2, func(gtx layout.Context, i int) layout.Dimensions {
		if i == 0 {
			return view.TabSegment(&c.holdings, l.Text(i18n.PortfolioMyHoldings), !c.active)(gtx)
		}
		return view.TabSegment(&c.tab, l.Text(i18n.MarketInstruments), c.active)(gtx)
	})
}

func (p *portfolioPage) catalogControls(gtx layout.Context, view ui.View) layout.Dimensions {
	c := &p.catalog
	return (ui.Column{Gap: ui.Space8}).Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.search.Layout(ui.LimitWidth(gtx, 440), view, i18n.MarketSearch, i18n.MarketSearchHint, func(string) {
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
	for _, item := range catalog.instruments {
		facts := strings.Join([]string{item.Name, item.ShortName, productString(item.Symbol), productString(item.ISIN),
			productString(item.MarketCode), productString(item.ProviderSymbol), item.QuoteCurrency.String()}, " ")
		if strings.Contains(normalizeDirectoryQuery(facts), query) {
			grouped[item.Type] = append(grouped[item.Type], item)
		}
	}
	var rows []layout.Widget
	for _, kind := range domain.AllInstrumentTypes() {
		items := grouped[kind]
		if len(items) == 0 {
			continue
		}
		rows = append(rows, view.Body(ui.TextHeading, l.Text(marketInstrumentTypes[kind]), view.Colors.Ink))
		for _, item := range items {
			rows = append(rows, p.catalogRow(view, item, catalog.managed[item.ID.String()] != ""))
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

func (p *portfolioPage) catalogRow(view ui.View, item domain.Instrument, managed bool) layout.Widget {
	c, l := &p.catalog, view.Language
	actions := c.links[item.ID]
	if actions == nil {
		actions = new(portfolioCatalogActions)
		c.links[item.ID] = actions
	}
	title := item.DisplayLabel()
	fields := []layout.Widget{}
	if item.Name != title {
		fields = append(fields, view.Message(item.Name, view.Colors.SecondaryInk))
	}
	badges := []string{item.QuoteCurrency.String()}
	if item.ArchivedAt != nil {
		badges = append(badges, l.Text(i18n.DirectoryArchived))
	}
	if managed {
		badges = append(badges, l.Text(i18n.ManagedProducts))
	}
	fields = append(fields, view.Message(strings.Join(badges, " · "), view.Colors.SecondaryInk))
	fields = append(fields, func(gtx layout.Context) layout.Dimensions {
		if p.request != nil || p.err != nil {
			gtx = gtx.Disabled()
		}
		var buttons []layout.Widget
		if p.openInstrument != nil {
			if actions.open.Clicked(gtx) {
				p.openInstrument(item.ID, title, &actions.open)
				gtx.Execute(op.InvalidateCmd{})
			}
			label := fmt.Sprintf(l.Text(i18n.PortfolioCatalogOpen), title)
			buttons = append(buttons, view.ButtonOf(&actions.open, ui.ButtonSpec{
				Variant: ui.ButtonLink, Size: ui.ButtonSmall, Title: l.Text(i18n.PortfolioCatalogDetails), Name: label,
			}))
		}
		if c.ledger != nil && !managed {
			if c.focus == &actions.manage && gtx.Enabled() {
				gtx.Execute(key.FocusCmd{Tag: &actions.manage})
				c.focus = nil
			}
			if actions.manage.Clicked(gtx) {
				c.draft = newPortfolioCatalogDraft(item, &actions.manage)
				c.draft.fields[portfolioInstrumentName].Focus(gtx)
				view.RestorePageScroll(0)
				gtx.Execute(op.InvalidateCmd{})
			}
			label := fmt.Sprintf(l.Text(i18n.PortfolioCatalogManageNamed), title)
			buttons = append(buttons, view.ButtonOf(&actions.manage, ui.ButtonSpec{
				Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: l.Text(i18n.PortfolioCatalogManage), Name: label,
			}))
		}
		return (ui.Wrap{Gap: ui.Space8}).Layout(gtx, len(buttons), func(gtx layout.Context, i int) layout.Dimensions {
			return buttons[i](gtx)
		})
	})
	return fundsCard(view, title, fields...)
}

func (p *portfolioPage) saveCatalogInstrument(archive bool) {
	c, d := &p.catalog, p.catalog.draft
	d.issue = i18n.NoMessage
	input := d.instrument
	patch := d.identityPatch()
	d.pending = runHistoryRead(&p.workers, p.invalidate, func() (i18n.MessageID, error) {
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
			p.read()
			return
		}
		d.issue = i18n.PortfolioCatalogWriteFailed
		var issue *domain.Error
		if errors.As(result.err, &issue) {
			switch {
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

func (p *portfolioPage) layoutCatalogDraft(gtx layout.Context, view ui.View) layout.Dimensions {
	c, d, l := &p.catalog, p.catalog.draft, view.Language
	if d.pending == nil {
		switch {
		case d.cancel.Clicked(gtx):
			d.confirming, d.issue = false, i18n.NoMessage
			gtx.Execute(key.FocusCmd{Tag: &d.archive})
			gtx.Execute(op.InvalidateCmd{})
		case d.back.Clicked(gtx):
			c.draft, c.focus = nil, d.returnTo
			gtx.Execute(op.InvalidateCmd{})
			return layout.Dimensions{}
		case d.archive.Clicked(gtx):
			d.confirming = true
			gtx.Execute(key.FocusCmd{Tag: &d.cancel})
			gtx.Execute(op.InvalidateCmd{})
		case d.confirm.Clicked(gtx) && d.issue != i18n.PortfolioCatalogChanged:
			p.saveCatalogInstrument(true)
			gtx.Execute(op.InvalidateCmd{})
		case d.save.Clicked(gtx) && d.issue != i18n.PortfolioCatalogChanged:
			p.saveCatalogInstrument(false)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	gtx, view = view.Measure(gtx, 620)
	title, action, prompt := i18n.PortfolioCatalogManage, i18n.InstitutionsArchive, i18n.PortfolioCatalogArchive
	if d.instrument.ArchivedAt != nil {
		action, prompt = i18n.InstitutionsRestore, i18n.PortfolioCatalogRestore
	}
	if d.confirming {
		title = prompt
	}
	rows := []ui.ColumnChild{
		ui.Rigid(tradeTitle(view, title)),
		ui.Rigid(view.Message(d.instrument.DisplayLabel(), view.Colors.Ink)),
	}
	if d.issue != i18n.NoMessage {
		rows = append(rows, ui.Rigid(view.Message(l.Text(d.issue), view.Colors.ErrorInk)))
	}
	if d.pending != nil {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)))
	} else {
		buttons := []layout.Widget{view.Button(&d.back, l.Text(i18n.ReserveBack))}
		switch {
		case d.issue == i18n.PortfolioCatalogChanged:
		case d.confirming:
			buttons = []layout.Widget{view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel))}
			buttons = append(buttons, view.PrimaryButton(&d.confirm, l.Text(action)))
		default:
			rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return (ui.Grid{MinColumn: 215, MaxColumns: 2, Gap: ui.Space8}).Layout(gtx, view, len(catalogIdentityLabels),
					func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
						return d.fields[i].Layout(gtx, view, ui.FieldOptions{Label: catalogIdentityLabels[i]})
					})
			}))
			buttons = append(buttons, view.Button(&d.archive, l.Text(action)), view.PrimaryButton(&d.save, l.Text(i18n.InstitutionsSave)))
		}
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: ui.Space8}).Layout(gtx, len(buttons), func(gtx layout.Context, i int) layout.Dimensions {
				return buttons[i](gtx)
			})
		}))
	}
	return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
}
