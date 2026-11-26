package desktop

import (
	"context"
	"fmt"
	"image"
	"strings"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type analysisSelection struct {
	scope       domain.AnalysisScope
	valuation   domain.Valuation
	excludeCash bool
	from, to    domain.LocalDate
	account     string
	currency    string
	assetClass  string
	member      string
	instrument  string
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
	q.Filters.AssetClass = s.assetClass
	if s.member != "" {
		id := domain.MemberID(s.member)
		q.Filters.MemberID = &id
	}
	if s.instrument != "" {
		id := domain.InstrumentID(s.instrument)
		q.Filters.InstrumentID = &id
	}
	return q
}

func (s analysisSelection) clearAdditional() analysisSelection {
	s.account, s.currency, s.assetClass, s.member, s.instrument = "", "", "", "", ""
	return s
}

type analysisFilterControls struct {
	selection                analysisSelection
	scope, target, valuation ui.Select
	account, currency        ui.Select
	assetClass, member       ui.Select
	instrument               ui.Select
	from, to                 ui.DatePicker
	cash, more, reset, clear widget.Clickable
	chips                    [5]widget.Clickable
	shortcuts                analysisDateShortcuts
	expanded                 bool
}

type analysisReferences struct {
	accounts, instruments, members []ui.Choice
}

type analysisContextReader interface {
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	ListInstruments(context.Context, bool) ([]domain.Instrument, error)
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
	ListMembers(context.Context, bool) ([]domain.Member, error)
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
	members, err := reader.ListMembers(ctx, false)
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
	for _, member := range members {
		out.members = append(out.members, ui.Choice{ID: member.ID.String(), Label: member.Name})
	}
	return out, nil
}

func (c *analysisFilterControls) layout(gtx layout.Context, view ui.View, refs analysisReferences, origin *wealthHistoryOrigin, now time.Time, effective domain.AnalysisQuery, change func(analysisSelection, analysisSelection)) layout.Dimensions {
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
	if c.clear.Clicked(gtx) {
		set(selection.clearAdditional())
		selection = c.selection
		gtx.Execute(key.FocusCmd{Tag: &c.more})
	}
	if c.cash.Clicked(gtx) {
		selection.excludeCash = !selection.excludeCash
		set(selection)
	}
	if c.expanded {
		for index := range c.chips {
			if c.chips[index].Clicked(gtx) {
				fields := [...]*string{&selection.account, &selection.currency, &selection.assetClass, &selection.member, &selection.instrument}
				*fields[index] = ""
				set(selection)
				gtx.Execute(key.FocusCmd{Tag: &c.more})
			}
		}
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
	last := origin.lastClosed(now).Format(time.DateOnly)
	from, to := selection.from, selection.to
	if effective.From <= effective.To {
		if from == "" {
			from = effective.From
		}
		if to == "" {
			to = effective.To
		}
	}
	if last >= origin.first {
		dates := ui.DateOptions{Today: last, Min: origin.first, Max: last, Format: view.DateFormat, WeekStart: time.Monday}
		if view.WeekStart == "sunday" {
			dates.WeekStart = time.Sunday
		}
		fromOptions, toOptions := dates, dates
		if selection.to != "" {
			fromOptions.Max = min(dates.Max, selection.to)
		}
		if selection.from != "" {
			toOptions.Min = max(dates.Min, selection.from)
		}
		fields = append(fields,
			analysisDate(&c.from, i18n.HistoryFilterFrom, fromOptions, from, func(value string) {
				next := c.selection
				next.from, next.to = value, to
				set(next)
			}),
			analysisDate(&c.to, i18n.HistoryFilterTo, toOptions, to, func(value string) {
				next := c.selection
				next.from, next.to = from, value
				set(next)
			}))
	}
	var additional []analysisFilterField
	if c.expanded {
		accounts := append([]ui.Choice{{Label: l.Text(i18n.HistoryAllAccounts)}}, refs.accounts...)
		currencies := append([]ui.Choice{{Label: l.Text(i18n.AnalysisAllCurrencies)}}, supportedCurrencyChoices...)
		members := append([]ui.Choice{{Label: l.Text(i18n.AnalysisAllMembers)}}, refs.members...)
		instruments := append([]ui.Choice{{Label: l.Text(i18n.AnalysisAllInstruments)}}, refs.instruments...)
		additional = append(additional,
			analysisSelect(&c.valuation, i18n.AnalysisValuation, values, string(val), false, func(value string) {
				next := c.selection
				next.valuation = domain.Valuation(value)
				set(next)
			}),
			analysisSelect(&c.account, i18n.AnalysisAccountFilter, accounts, selection.account, true, func(value string) {
				next := c.selection
				next.account = value
				set(next)
			}),
			analysisSelect(&c.currency, i18n.AnalysisCurrency, currencies, selection.currency, true, func(value string) {
				next := c.selection
				next.currency = value
				set(next)
			}),
			analysisSelect(&c.assetClass, i18n.AnalysisAssetClass, analysisAssetClasses(l), selection.assetClass, false, func(value string) {
				next := c.selection
				next.assetClass = value
				set(next)
			}),
			analysisSelect(&c.member, i18n.AnalysisMember, members, selection.member, false, func(value string) {
				next := c.selection
				next.member = value
				set(next)
			}),
			analysisSelect(&c.instrument, i18n.AnalysisInstrumentFilter, instruments, selection.instrument, true, func(value string) {
				next := c.selection
				next.instrument = value
				set(next)
			}))
	}
	return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			view.Origin = view.Origin.Add(image.Pt(gtx.Dp(12), gtx.Dp(12)))
			width := gtx.Constraints.Max.X
			drawFields := func(fields []analysisFilterField, offset int) layout.Dimensions {
				return ui.Place(gtx, view, image.Pt(0, offset), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
					return (ui.Grid{MinColumn: 155, Gap: 10, MaxColumns: 4, Balanced: true}).Layout(gtx, view, len(fields),
						func(gtx layout.Context, view ui.View, i int) layout.Dimensions { return fields[i](gtx, view) })
				})
			}
			gap := gtx.Dp(10)
			primary := drawFields(fields, 0)
			offset := primary.Size.Y + gap
			if origin.first <= last {
				shortcuts := ui.Place(gtx, view, image.Pt(0, offset), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
					return c.shortcuts.layout(gtx, view, analysisDateWindow{from, to}, origin.first, last, func(window analysisDateWindow) {
						next := c.selection
						next.from, next.to = window.from, window.to
						set(next)
					})
				})
				offset += shortcuts.Size.Y + gap
			}
			widgets := []layout.Widget{
				view.ButtonOf(&c.reset, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Title: l.Text(i18n.AnalysisReset)}),
				analysisToggle(view, &c.more, l.Text(i18n.AnalysisMoreFilters), c.expanded),
			}
			if c.expanded {
				widgets = append(widgets, analysisToggle(view, &c.cash, l.Text(i18n.AnalysisIncludeCash), !selection.excludeCash))
				if selection != selection.clearAdditional() {
					widgets = append(widgets, view.Button(&c.clear, l.Text(i18n.AnalysisClearAdditional)))
				}
			}
			buttons := ui.Place(gtx, view, image.Pt(0, offset), width, func(gtx layout.Context, _ ui.View) layout.Dimensions {
				return (ui.Wrap{Gap: 8}).Layout(gtx, len(widgets), func(gtx layout.Context, i int) layout.Dimensions {
					return widgets[i](gtx)
				})
			})
			offset += buttons.Size.Y + gap
			summary := ui.Place(gtx, view, image.Pt(0, offset), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
				return view.Message(analysisFilterSummary(l, refs, selection), view.Colors.SecondaryInk)(gtx)
			})
			offset += summary.Size.Y
			if len(additional) > 0 {
				chips := c.chipWidgets(view, refs, selection)
				if len(chips) > 0 {
					dims := ui.Place(gtx, view, image.Pt(0, offset+gap), width, func(gtx layout.Context, _ ui.View) layout.Dimensions {
						return (ui.Wrap{Gap: 8}).Layout(gtx, len(chips), func(gtx layout.Context, index int) layout.Dimensions {
							return chips[index](gtx)
						})
					})
					offset += dims.Size.Y + gap
				}
				offset += gap + drawFields(additional, offset+gap).Size.Y
			}
			return layout.Dimensions{Size: image.Pt(width, offset)}
		})
	})
}

func analysisAssetClasses(l i18n.Language) []ui.Choice {
	choices := []ui.Choice{{Label: l.Text(i18n.AnalysisAllClasses)}, {ID: "cash", Label: l.Text(i18n.AnalysisCash)}}
	for _, kind := range domain.AllInstrumentTypes() {
		choices = append(choices, ui.Choice{ID: string(kind), Label: l.Text(marketInstrumentTypes[kind])})
	}
	return choices
}

func analysisFilterSummary(l i18n.Language, refs analysisReferences, s analysisSelection) string {
	valuation := i18n.AnalysisBase
	if s.valuation == domain.ValuationNative {
		valuation = i18n.AnalysisNative
	}
	cash := i18n.AnalysisCashIncluded
	if s.excludeCash {
		cash = i18n.AnalysisCashExcluded
	}
	labels := []string{l.Text(valuation), l.Text(cash)}
	for _, label := range analysisChipLabels(l, refs, s) {
		if label != "" {
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, " · ")
}

func analysisChipLabels(l i18n.Language, refs analysisReferences, s analysisSelection) [5]string {
	var labels [5]string
	labels[1] = s.currency
	for _, filter := range [...]struct {
		index   int
		id      string
		options []ui.Choice
	}{{0, s.account, refs.accounts}, {2, s.assetClass, analysisAssetClasses(l)}, {3, s.member, refs.members}, {4, s.instrument, refs.instruments}} {
		if filter.id == "" {
			continue
		}
		labels[filter.index] = l.Text(i18n.OverviewEvidenceUnavailable)
		for _, option := range filter.options {
			if option.ID == filter.id {
				labels[filter.index] = option.Label
				break
			}
		}
	}
	return labels
}

func (c *analysisFilterControls) chipWidgets(view ui.View, refs analysisReferences, s analysisSelection) []layout.Widget {
	var chips []layout.Widget
	for index, label := range analysisChipLabels(view.Language, refs, s) {
		if label != "" {
			name := fmt.Sprintf(view.Language.Text(i18n.AnalysisRemoveFilter), label)
			chips = append(chips, view.ButtonOf(&c.chips[index], ui.ButtonSpec{
				Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: label, Name: name, Icon: lucide.X,
			}))
		}
	}
	return chips
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

// analysisDisclosure is a quiet expander for supporting data, such as the
// table behind a chart. It reads as a chevron and a label, not a full button.
func analysisDisclosure(view ui.View, button *widget.Clickable, text string, open bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		icon := lucide.ChevronRight
		if open {
			icon = lucide.ChevronDown
		}
		colors := view.Colors
		gtx.Constraints.Min = image.Point{}
		return layout.W.Layout(gtx, view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Name: text, Toggle: true, Selected: open,
			Content: func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space4)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.LayoutGlyph(gtx, icon, 16, colors.SecondaryInk)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return view.Line(gtx, ui.TextLabel, text, colors.SecondaryInk)
					}))
			}}))
	}
}
