package desktop

import (
	"slices"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/shopspring/decimal"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type holdingSortKey uint8

const (
	holdingSortName holdingSortKey = iota
	holdingSortQuantity
	holdingSortCost
	holdingSortValue
	holdingSortGain
	holdingSortCount
)

var holdingSortLabels = [holdingSortCount]i18n.MessageID{
	i18n.AnalysisInstrument, i18n.AccountQuantity, i18n.PortfolioCost,
	i18n.PortfolioNativeValue, i18n.PortfolioUnrealized,
}

type holdingSort struct {
	key        holdingSortKey
	descending bool
}

type portfolioHoldingRow struct {
	instrument *domain.InstrumentHoldingsView
	member     *domain.InstrumentHoldingMember
}

type sortableHoldingRow struct {
	row             portfolioHoldingRow
	label, unit, id string
	value           decimal.Decimal
	hasValue        bool
}

// Sorting reorders displayed rows only. Quantities of unlike units and
// amounts in unlike currencies keep their own ordered groups in either
// direction. Incomplete values come last within their currency.
func sortPortfolioHoldingRows(rows []portfolioHoldingRow, sort holdingSort, locale i18n.Language) {
	col := collate.New(language.Make(string(locale)))
	items := make([]sortableHoldingRow, 0, len(rows))
	for _, row := range rows {
		item := sortableHoldingRow{row: row, label: holdingInstrumentName(*row.instrument), id: row.instrument.InstrumentID.String()}
		amounts := row.instrument.Amounts
		if row.member != nil {
			item.id = row.member.HoldingID.String()
			amounts = row.member.Amounts
		}
		unit, amount := holdingSortMeasure(*row.instrument, amounts, sort.key)
		item.unit = unit
		if amount != "" {
			value, err := decimal.NewFromString(amount)
			item.value, item.hasValue = value, err == nil
		}
		items = append(items, item)
	}
	slices.SortStableFunc(items, func(a, b sortableHoldingRow) int {
		if order := strings.Compare(a.unit, b.unit); order != 0 {
			return order
		}
		order := 0
		switch {
		case sort.key == holdingSortName:
			order = col.CompareString(a.label, b.label)
		case a.hasValue && b.hasValue:
			order = a.value.Cmp(b.value)
		case a.hasValue:
			return -1
		case b.hasValue:
			return 1
		}
		if order != 0 {
			if sort.descending {
				return -order
			}
			return order
		}
		if order := col.CompareString(a.label, b.label); order != 0 {
			return order
		}
		if order := col.CompareString(a.row.instrument.Name, b.row.instrument.Name); order != 0 {
			return order
		}
		return strings.Compare(a.id, b.id)
	})
	for i, item := range items {
		rows[i] = item.row
	}
}

func holdingSortMeasure(row domain.InstrumentHoldingsView, amount domain.HoldingAmounts, key holdingSortKey) (unit, value string) {
	if key == holdingSortName {
		return "", ""
	}
	if key == holdingSortQuantity {
		return row.QuantityUnit, amount.Quantity
	}
	switch key {
	case holdingSortCost:
		if amount.TotalCost != nil {
			value = amount.TotalCost.Amount
		}
	case holdingSortValue:
		if amount.CurrentValue != nil {
			value = amount.CurrentValue.Amount
		}
	case holdingSortGain:
		if amount.UnrealizedGain != nil {
			value = amount.UnrealizedGain.Amount
		}
	}
	return row.QuoteCurrency.String(), value
}

func (p *portfolioPage) holdingSortHeader(gtx layout.Context, view ui.View, group domain.HoldingGroupKey) layout.Dimensions {
	h := &p.holdings
	buttons := h.sortButtons[group]
	if buttons == nil {
		buttons = new([holdingSortCount]widget.Clickable)
		h.sortButtons[group] = buttons
	}
	wide := ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, holdingTableWidth)
	return layoutPortfolioHoldingSortControls(gtx, view, &h.visit.sort, buttons, wide, func() {
		if view.Overlay != nil {
			view.Overlay.Close()
		}
		gtx.Execute(op.InvalidateCmd{})
	})
}

func layoutHoldingSortControls(gtx layout.Context, view ui.View, sort *holdingSort, buttons *[holdingSortCount]widget.Clickable, wide bool, reset func()) layout.Dimensions {
	return layoutHoldingSortControlsWithLabels(gtx, view, sort, buttons, wide, reset, holdingSortLabels)
}

func layoutPortfolioHoldingSortControls(gtx layout.Context, view ui.View, sort *holdingSort, buttons *[holdingSortCount]widget.Clickable, wide bool, reset func()) layout.Dimensions {
	labels := holdingSortLabels
	labels[holdingSortCost] = i18n.PortfolioColumnCost
	labels[holdingSortValue] = i18n.PortfolioColumnValue
	labels[holdingSortGain] = i18n.PortfolioColumnGain
	return layoutHoldingSortControlsWithLabels(gtx, view, sort, buttons, wide, reset, labels)
}

func layoutHoldingSortControlsWithLabels(gtx layout.Context, view ui.View, sort *holdingSort, buttons *[holdingSortCount]widget.Clickable, wide bool, reset func(), labels [holdingSortCount]i18n.MessageID) layout.Dimensions {
	l := view.Language
	column := func(gtx layout.Context, i int) layout.Dimensions {
		key := holdingSortKey(i)
		button := &buttons[key]
		if button.Clicked(gtx) {
			*sort = holdingSort{key: key, descending: sort.key == key && !sort.descending}
			reset()
		}
		label := holdingMetricLabel(l, labels[key])
		name, title := label, label
		selected := sort.key == key
		if selected {
			direction, arrow := i18n.PortfolioSortAscending, " ↑"
			if sort.descending {
				direction, arrow = i18n.PortfolioSortDescending, " ↓"
			}
			title += arrow
			name += " · " + l.Text(direction)
		}
		if !wide {
			return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: title, Name: name, Selected: selected})(gtx)
		}
		align := layout.E
		if key == holdingSortName {
			align = layout.W
		}
		return align.Layout(gtx, view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonCompact, Title: title, Name: name, Selected: selected}))
	}
	if !wide {
		return (ui.Wrap{Gap: ui.Space8}).Layout(gtx, len(labels), column)
	}
	return holdingTableInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return holdingTableColumns(gtx, column, func(gtx layout.Context) layout.Dimensions {
			return view.Line(gtx, ui.TextCaption, l.Text(i18n.FundsActionColumn), view.Colors.SecondaryInk)
		})
	})
}
