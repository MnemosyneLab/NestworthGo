package desktop

import (
	"context"
	"image"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type analysisSelection struct {
	scope       domain.AnalysisScope
	valuation   domain.Valuation
	excludeCash bool
	from, to    domain.LocalDate
	account     string
	currency    string
}

func (s analysisSelection) ready() bool {
	return (s.scope.Kind == "" || s.scope.Kind == domain.ScopeHousehold || s.scope.ID != "") &&
		(s.from == "" || s.to == "" || s.from <= s.to)
}

func (s analysisSelection) query(q domain.AnalysisQuery) domain.AnalysisQuery {
	if s.scope.Kind != "" {
		q.Scope = s.scope
	}
	if s.valuation != "" {
		q.Valuation = s.valuation
	}
	q.IncludeCash = !s.excludeCash
	if s.account != "" {
		id := domain.AccountID(s.account)
		q.Filters.AccountID = &id
	}
	if s.currency != "" {
		code := domain.CurrencyCode(s.currency)
		q.Filters.Currency = &code
	}
	return q
}

type analysisFilterControls struct {
	selection                analysisSelection
	scope, target, valuation ui.Select
	account, currency        ui.Select
	from, to                 ui.DatePicker
	cash, more, reset        widget.Clickable
	expanded                 bool
}

type analysisReferences struct {
	accounts, instruments []ui.Choice
}

type analysisContextReader interface {
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	ListInstruments(context.Context, bool) ([]domain.Instrument, error)
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
}

type analysisContext struct {
	origin *wealthHistoryOrigin
	names  map[string]string
	analysisReferences
}

func readAnalysisContext(ctx context.Context, reader analysisContextReader) (analysisContext, error) {
	origin, err := reader.HistoryOrigin(ctx)
	if err != nil || origin == nil {
		return analysisContext{}, err
	}
	location, err := time.LoadLocation(origin.Timezone)
	if err != nil {
		return analysisContext{}, err
	}
	instruments, err := reader.ListInstruments(ctx, true)
	if err != nil {
		return analysisContext{}, err
	}
	accounts, err := reader.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return analysisContext{}, err
	}
	out := analysisContext{
		origin: &wealthHistoryOrigin{first: origin.StartedAt.In(location).Format(time.DateOnly), location: location},
		names:  make(map[string]string, len(instruments)+len(accounts)),
	}
	for _, instrument := range instruments {
		out.names[instrument.ID.String()] = instrument.DisplayLabel()
		if instrument.ArchivedAt == nil {
			out.instruments = append(out.instruments, ui.Choice{ID: instrument.ID.String(), Label: instrument.SelectionLabel()})
		}
	}
	for _, record := range accounts {
		if record.Account.ArchivedAt == nil {
			out.accounts = append(out.accounts, ui.Choice{ID: record.Account.ID.String(), Label: record.Account.Name})
		}
		out.names[record.Account.ID.String()] = record.Account.Name
	}
	return out, nil
}

func (c *analysisFilterControls) layout(gtx layout.Context, view ui.View, refs analysisReferences, origin *wealthHistoryOrigin, now time.Time, change func(analysisSelection, analysisSelection)) layout.Dimensions {
	l := view.Language
	if c.more.Clicked(gtx) {
		c.expanded = !c.expanded
		gtx.Execute(op.InvalidateCmd{})
	}
	selection := c.selection
	set := func(next analysisSelection) {
		if c.selection != next {
			previous := c.selection
			c.selection = next
			change(previous, next)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	if c.reset.Clicked(gtx) {
		set(analysisSelection{})
		selection = c.selection
	}
	if c.cash.Clicked(gtx) {
		selection.excludeCash = !selection.excludeCash
		set(selection)
	}
	scope := selection.scope.Kind
	if scope == "" {
		scope = domain.ScopeHousehold
	}
	val := selection.valuation
	if val == "" {
		val = domain.ValuationBase
	}
	scopes := []ui.Choice{
		{ID: string(domain.ScopeHousehold), Label: l.Text(i18n.AnalysisPortfolio)},
		{ID: string(domain.ScopeAccount), Label: l.Text(i18n.AnalysisAccount)},
		{ID: string(domain.ScopeInstrument), Label: l.Text(i18n.AnalysisInstrument)},
	}
	values := []ui.Choice{
		{ID: string(domain.ValuationBase), Label: l.Text(i18n.AnalysisBase)},
		{ID: string(domain.ValuationNative), Label: l.Text(i18n.AnalysisNative)},
	}
	fields := []analysisFilterField{
		analysisSelect(&c.scope, i18n.AnalysisScope, scopes, string(scope), false, func(value string) {
			next := c.selection
			next.scope = domain.AnalysisScope{Kind: domain.ScopeKind(value)}
			set(next)
		}),
	}
	if scope != domain.ScopeHousehold {
		choices, label := refs.accounts, i18n.AnalysisAccount
		if scope == domain.ScopeInstrument {
			choices, label = refs.instruments, i18n.AnalysisInstrument
		}
		fields = append(fields, analysisSelect(&c.target, label, choices, selection.scope.ID, true, func(value string) {
			next := c.selection
			next.scope.ID = value
			set(next)
		}))
	}
	fields = append(fields, analysisSelect(&c.valuation, i18n.AnalysisValuation, values, string(val), false, func(value string) {
		next := c.selection
		next.valuation = domain.Valuation(value)
		set(next)
	}))
	last := origin.lastClosed(now).Format(time.DateOnly)
	if last >= origin.first {
		dates := ui.DateOptions{Today: last, Min: origin.first, Max: last, Clearable: true, Format: view.DateFormat, WeekStart: time.Monday}
		if view.WeekStart == "sunday" {
			dates.WeekStart = time.Sunday
		}
		fields = append(fields,
			analysisDate(&c.from, i18n.HistoryFilterFrom, dates, selection.from, func(value string) {
				next := c.selection
				next.from = value
				set(next)
			}),
			analysisDate(&c.to, i18n.HistoryFilterTo, dates, selection.to, func(value string) {
				next := c.selection
				next.to = value
				set(next)
			}))
	}
	if c.expanded {
		accounts := append([]ui.Choice{{Label: l.Text(i18n.HistoryAllAccounts)}}, refs.accounts...)
		currencies := append([]ui.Choice{{Label: l.Text(i18n.AnalysisAllCurrencies)}}, supportedCurrencyChoices...)
		fields = append(fields,
			analysisSelect(&c.account, i18n.AnalysisAccountFilter, accounts, selection.account, true, func(value string) {
				next := c.selection
				next.account = value
				set(next)
			}),
			analysisSelect(&c.currency, i18n.AnalysisCurrency, currencies, selection.currency, true, func(value string) {
				next := c.selection
				next.currency = value
				set(next)
			}))
	}
	return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			view.Origin = view.Origin.Add(image.Pt(gtx.Dp(12), gtx.Dp(12)))
			grid := (ui.Grid{MinColumn: 155, Gap: 10, MaxColumns: 4, Balanced: true}).Layout(gtx, view, len(fields),
				func(gtx layout.Context, view ui.View, i int) layout.Dimensions { return fields[i](gtx, view) })
			defer op.Offset(image.Pt(0, grid.Size.Y+gtx.Dp(10))).Push(gtx.Ops).Pop()
			widgets := []layout.Widget{
				analysisToggle(view, &c.cash, l.Text(i18n.AnalysisIncludeCash), !selection.excludeCash),
				analysisToggle(view, &c.more, l.Text(i18n.AnalysisMoreFilters), c.expanded),
				view.ButtonOf(&c.reset, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Title: l.Text(i18n.AnalysisReset)}),
			}
			buttons := (ui.Wrap{Gap: 8}).Layout(gtx, len(widgets),
				func(gtx layout.Context, i int) layout.Dimensions { return widgets[i](gtx) })
			return layout.Dimensions{Size: image.Pt(grid.Size.X, grid.Size.Y+gtx.Dp(10)+buttons.Size.Y)}
		})
	})
}

type analysisFilterField func(layout.Context, ui.View) layout.Dimensions

func analysisSelect(control *ui.Select, label i18n.MessageID, choices []ui.Choice, value string, searchable bool, change func(string)) analysisFilterField {
	return func(gtx layout.Context, view ui.View) layout.Dimensions {
		control.Layout(gtx, view, ui.PickerField{Label: label, Origin: view.Origin}, ui.SelectOptions{Items: choices, Searchable: searchable}, value, change)
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
	}
}

func analysisDate(control *ui.DatePicker, label i18n.MessageID, options ui.DateOptions, value string, change func(string)) analysisFilterField {
	return func(gtx layout.Context, view ui.View) layout.Dimensions {
		control.Layout(gtx, view, ui.PickerField{Label: label, Origin: view.Origin}, options, value, change)
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
	}
}

func analysisToggle(view ui.View, button *widget.Clickable, text string, selected bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		view.Focus.Watch(gtx, button)
		return view.TabButton(button, text, selected)(gtx)
	}
}
