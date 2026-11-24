package desktop

import (
	"image"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type historyFilterControls struct {
	kind, account, instrument ui.Select
	from, to                  ui.DatePicker
	clear, retry              widget.Clickable
}

var filterActivityKinds = []domain.ActivityKind{
	domain.ActivityCashIn, domain.ActivityCashOut, domain.ActivityCashTransfer, domain.ActivityFXConversion,
	domain.ActivityValueUpdate, domain.ActivityBuy, domain.ActivitySell, domain.ActivityCashDividend,
	domain.ActivityPositionTransfer, domain.ActivityCostAdjustment, domain.ActivityDebtDraw,
	domain.ActivityDebtPayment, domain.ActivityReversal,
}

func (p *historyPage) noHistory() bool {
	return p.reads != nil && p.reads.refsErr == nil && p.reads.references != nil && p.reads.references.Origin == nil
}

func (p *historyPage) applyFilters(gtx layout.Context, filters history.Filters) {
	if p.model.Filters() == filters {
		return
	}
	p.model.SetFilters(filters)
	p.list.Position = layout.Position{}
	p.pruneDetailButtons()
	gtx.Execute(op.InvalidateCmd{})
}

func (p *historyPage) layoutFilters(gtx layout.Context, view ui.View) layout.Dimensions {
	r, c, l := p.reads, &p.filter, view.Language
	if r == nil || p.noHistory() {
		return layout.Dimensions{}
	}
	if r.refsErr != nil {
		if c.retry.Clicked(gtx) {
			r.loadReferences()
			gtx.Execute(op.InvalidateCmd{})
		}
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
			layout.Flexed(1, view.Message(l.Text(i18n.HistoryFilterLoadFailed), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&c.retry, l.Text(i18n.HistoryRetryFilters))))
	}
	if r.references == nil || r.references.Origin == nil {
		return layout.Dimensions{}
	}

	refs, filters := r.references, p.model.Filters()
	if c.clear.Clicked(gtx) {
		p.applyFilters(gtx, history.Filters{})
		filters = history.Filters{}
		gtx.Execute(key.FocusCmd{Tag: &c.kind.Trigger})
	}
	accountChoices := []ui.Choice{{Label: l.Text(i18n.HistoryAllAccounts)}}
	instrumentChoices := []ui.Choice{{Label: l.Text(i18n.HistoryAllInstruments)}}
	kindChoices := []ui.Choice{{Label: l.Text(i18n.HistoryAllTypes)}}
	archived := func(name string, isArchived bool) string {
		if isArchived {
			return name + " · " + l.Text(i18n.DirectoryArchived)
		}
		return name
	}
	for _, row := range refs.Accounts {
		account := row.Account
		accountChoices = append(accountChoices, ui.Choice{ID: account.ID.String(), Label: archived(account.Name, account.ArchivedAt != nil)})
	}
	for _, instrument := range refs.Instruments {
		instrumentChoices = append(instrumentChoices, ui.Choice{ID: instrument.ID.String(), Label: archived(instrument.SelectionLabel(), instrument.ArchivedAt != nil)})
	}
	for _, kind := range filterActivityKinds {
		kindChoices = append(kindChoices, ui.Choice{ID: kind.String(), Label: l.Text(historyKindLabel(kind, ""))})
	}

	layoutPicker := func(picker *ui.Select, label, fallback i18n.MessageID, choices []ui.Choice, id string, searchable bool, change func(string)) func(layout.Context, image.Point) layout.Dimensions {
		return func(gtx layout.Context, at image.Point) layout.Dimensions {
			field := ui.PickerField{Label: label, Origin: at}
			options := ui.SelectOptions{Items: choices, Searchable: searchable}
			if id != "" {
				found := false
				for _, option := range choices {
					found = found || option.ID == id
				}
				if !found {
					options.ValueLabel = l.Text(fallback)
				}
			}
			picker.Layout(gtx, view, field, options, id, change)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}
	}

	today := r.clock().In(refs.Location).Format(time.DateOnly)
	first := refs.Origin.StartedAt.In(refs.Location).Format(time.DateOnly)
	weekStart := time.Monday
	if view.WeekStart == "sunday" {
		weekStart = time.Sunday
	}
	layoutDate := func(picker *ui.DatePicker, label i18n.MessageID, value, minDay, maxDay string, change func(string)) func(layout.Context, image.Point) layout.Dimensions {
		return func(gtx layout.Context, at image.Point) layout.Dimensions {
			options := ui.DateOptions{Today: today, Min: minDay, Max: maxDay, Clearable: true, WeekStart: weekStart, Format: view.DateFormat}
			picker.Layout(gtx, view, ui.PickerField{Label: label, Origin: at}, options, value, change)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}
	}
	fromMax, toMin := today, first
	if filters.To != "" {
		fromMax = filters.To
	}
	if filters.From != "" {
		toMin = filters.From
	}
	origin := view.Origin
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := historyFilterRow(gtx, origin,
				layoutPicker(&c.kind, i18n.HistoryFilterType, i18n.HistoryActivity, kindChoices, filters.Kind.String(), false, func(value string) {
					next := p.model.Filters()
					next.Kind = domain.ActivityKind(value)
					p.applyFilters(gtx, next)
				}),
				layoutPicker(&c.account, i18n.HistoryFilterAccount, i18n.AccountReferenceUnavailable, accountChoices, filters.Account.String(), true, func(value string) {
					next := p.model.Filters()
					next.Account = domain.AccountID(value)
					p.applyFilters(gtx, next)
				}),
				layoutPicker(&c.instrument, i18n.HistoryFilterInstrument, i18n.HistoryInstrumentUnavailable, instrumentChoices, filters.Instrument.String(), true, func(value string) {
					next := p.model.Filters()
					next.Instrument = domain.InstrumentID(value)
					p.applyFilters(gtx, next)
				}),
			)
			origin.Y += dims.Size.Y + gtx.Dp(8)
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return historyFilterRow(gtx, origin,
				layoutDate(&c.from, i18n.HistoryFilterFrom, filters.From, first, fromMax, func(day string) {
					next := p.model.Filters()
					next.From = day
					p.applyFilters(gtx, next)
				}),
				layoutDate(&c.to, i18n.HistoryFilterTo, filters.To, toMin, today, func(day string) {
					next := p.model.Filters()
					next.To = day
					p.applyFilters(gtx, next)
				}),
				func(gtx layout.Context, _ image.Point) layout.Dimensions {
					if filters == (history.Filters{}) {
						return layout.Dimensions{}
					}
					return view.Button(&c.clear, l.Text(i18n.HistoryClearFilters))(gtx)
				},
			)
		}),
	)
}

// The shared pickers register overlays in window coordinates.
func historyFilterRow(gtx layout.Context, origin image.Point, fields ...func(layout.Context, image.Point) layout.Dimensions) layout.Dimensions {
	gap := gtx.Dp(8)
	width := (gtx.Constraints.Max.X - gap*(len(fields)-1)) / len(fields)
	children := make([]layout.FlexChild, 0, len(fields))
	for i, field := range fields {
		at := origin.Add(image.Pt(i*(width+gap), 0))
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			return field(gtx, at)
		}))
	}
	return layout.Flex{Alignment: layout.Middle, Gap: gap}.Layout(gtx, children...)
}
