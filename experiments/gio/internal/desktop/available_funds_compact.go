package desktop

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type fundsFigure struct {
	pattern i18n.MessageID
	value   string
}

func fundsMoneyFigure(view ui.View, pattern i18n.MessageID, value *domain.Money) fundsFigure {
	return fundsFigure{pattern, fundsAmountText(view, value)}
}

func fundsFigureLabel(language i18n.Language, pattern i18n.MessageID) string {
	return strings.TrimRight(fmt.Sprintf(language.Text(pattern), ""), " ·:：")
}

func (f fundsFigure) layout(view ui.View, caption bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return semanticRow(gtx, fmt.Sprintf(view.Language.Text(f.pattern), f.value), func(gtx layout.Context) layout.Dimensions {
			value := view.Body(ui.TextLabel, f.value, view.Colors.Ink)
			if !caption {
				return layout.E.Layout(gtx, value)
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(view.Body(ui.TextCaption, fundsFigureLabel(view.Language, f.pattern), view.Colors.SecondaryInk)),
				layout.Rigid(value),
			)
		})
	}
}

func fundsFigures(view ui.View, figures []fundsFigure) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Grid{MinColumn: 105, MaxColumns: 4, Gap: ui.Space8, Balanced: true}).Layout(gtx, view, len(figures), func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
			return figures[i].layout(view, true)(gtx)
		})
	}
}

func fundsBaseSummary(view ui.View, code domain.CurrencyCode, bucket *domain.LiquidityBucket) layout.Widget {
	beforeLabel, reserveLabel, afterLabel := i18n.FundsAvailableBefore, i18n.FundsApplied, i18n.FundsAvailableAfter
	before, after := bucket.FullAvailable, bucket.FullUnreserved
	if bucket.Status != domain.StatusComplete && bucket.AppliedReserveSubtotal != nil {
		reserveLabel = i18n.FundsAppliedSubtotal
	}
	if before == nil && bucket.KnownAvailableSubtotal != nil {
		before, beforeLabel = bucket.KnownAvailableSubtotal, i18n.FundsBeforeSubtotal
	}
	if after == nil && bucket.KnownUnreservedSubtotal != nil {
		after, afterLabel = bucket.KnownUnreservedSubtotal, i18n.FundsAfterSubtotal
	}
	figures := []fundsFigure{
		fundsMoneyFigure(view, beforeLabel, before),
		fundsMoneyFigure(view, reserveLabel, bucket.AppliedReserveSubtotal),
		fundsMoneyFigure(view, afterLabel, after),
	}
	title := fmt.Sprintf(view.Language.Text(i18n.FundsBase), code)
	return fundsCompactRow(view, true, true, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space4)}.Layout(gtx,
			layout.Rigid(fundsTitleWithStatus(view, title, title, bucket.Status)),
			layout.Rigid(fundsFigures(view, figures)),
		)
	})
}

func fundsBreakdown(view ui.View, groups []domain.NativeCurrencyGroup, amounts map[domain.CurrencyCode]application.LiquidityNativeAmounts) layout.Widget {
	l := view.Language
	labels := [...]i18n.MessageID{i18n.FundsNativeCash, i18n.FundsNativeProceeds, i18n.FundsApplied, i18n.FundsAvailableAfter}
	return func(gtx layout.Context) layout.Dimensions {
		wide := gtx.Constraints.Max.X >= gtx.Dp(ui.BreakpointCompact)
		rows := []layout.FlexChild{layout.Rigid(tradeTitle(view, i18n.FundsNativeBreakdown))}
		if wide {
			rows = append(rows, layout.Rigid(fundsCompactRow(view, true, false, func(gtx layout.Context) layout.Dimensions {
				var cells [5]layout.Widget
				cells[0] = view.Message(l.Text(i18n.FundsBreakdownCurrency), view.Colors.SecondaryInk)
				for i, label := range labels {
					cells[i+1] = func(gtx layout.Context) layout.Dimensions {
						return layout.E.Layout(gtx, view.Message(fundsFigureLabel(l, label), view.Colors.SecondaryInk))
					}
				}
				return fundsBreakdownColumns(gtx, cells)
			})))
		}
		for i, group := range groups {
			parts := amounts[group.Currency]
			after, afterLabel := group.FullUnreserved, i18n.FundsAvailableAfter
			if after == nil && group.KnownUnreservedSubtotal != nil {
				after, afterLabel = group.KnownUnreservedSubtotal, i18n.FundsAfterSubtotal
			}
			reserveLabel := i18n.FundsApplied
			if group.Status != domain.StatusComplete && group.AppliedReserveSubtotal != nil {
				reserveLabel = i18n.FundsAppliedSubtotal
			}
			figures := []fundsFigure{
				{labels[0], fundsNativeAmountText(view, parts.Cash, group.Currency)},
				{labels[1], fundsNativeAmountText(view, parts.Proceeds, group.Currency)},
				fundsMoneyFigure(view, reserveLabel, group.AppliedReserveSubtotal),
				fundsMoneyFigure(view, afterLabel, after),
			}
			name := group.Currency.String()
			currency := fundsTitleWithStatus(view, name, fmt.Sprintf(l.Text(i18n.FundsNative), name), group.Status)
			rows = append(rows, layout.Rigid(fundsCompactRow(view, !wide && i == 0, i == len(groups)-1, func(gtx layout.Context) layout.Dimensions {
				if !wide {
					return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space4)}.Layout(gtx,
						layout.Rigid(currency), layout.Rigid(fundsFigures(view, figures)))
				}
				cells := [5]layout.Widget{currency}
				for i, figure := range figures {
					cells[i+1] = figure.layout(view, false)
				}
				return fundsBreakdownColumns(gtx, cells)
			})))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	}
}

func fundsBreakdownColumns(gtx layout.Context, cells [5]layout.Widget) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
		layout.Flexed(0.65, cells[0]), layout.Flexed(1, cells[1]), layout.Flexed(1, cells[2]),
		layout.Flexed(1, cells[3]), layout.Flexed(1, cells[4]))
}

func fundsCompactRow(view ui.View, first, last bool, content layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.RowCard(gtx, first, last, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: ui.Space8, Bottom: ui.Space8, Left: ui.Space12, Right: ui.Space12}.Layout(gtx, content)
		})
	}
}

func fundsTitleWithStatus(view ui.View, title, accessibleTitle string, status domain.CompletenessStatus) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return semanticRow(gtx, accessibleTitle, view.Body(ui.TextLabel, title, view.Colors.Ink))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if status == domain.StatusComplete {
					return layout.Dimensions{}
				}
				return view.Message(view.Language.Text(fundsStatus(status)), view.Colors.SecondaryInk)(gtx)
			}),
		)
	}
}

func fundsSourceRow(view ui.View, title string, buttons *fundsLinks, figures []fundsFigure, notes, actions, details []layout.Widget) layout.Widget {
	return fundsCompactRow(view, true, true, func(gtx layout.Context) layout.Dimensions {
		rowActions := actions
		rows := []layout.FlexChild{layout.Rigid(view.Message(title, view.Colors.Ink)), layout.Rigid(fundsFigures(view, figures))}
		if len(details) != 0 {
			l := view.Language
			if buttons.details.Clicked(gtx) {
				buttons.showDetails = !buttons.showDetails
				gtx.Execute(op.InvalidateCmd{})
			}
			rowActions = append(rowActions, func(gtx layout.Context) layout.Dimensions {
				view.Focus.Watch(gtx, &buttons.details)
				return view.ButtonOf(&buttons.details, ui.ButtonSpec{
					Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Toggle: true, Selected: buttons.showDetails,
					Title: l.Text(i18n.FundsSourceDetails), Name: fmt.Sprintf(l.Text(i18n.FundsNamedSourceDetails), title),
				})(gtx)
			})
		}
		if len(notes) > 0 {
			rows = append(rows, layout.Rigid(fundsWrappedActions(notes)))
		}
		if len(rowActions) > 0 {
			rows = append(rows, layout.Rigid(fundsWrappedActions(rowActions)))
		}
		if buttons.showDetails {
			for _, detail := range details {
				rows = append(rows, layout.Rigid(detail))
			}
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space4)}.Layout(gtx, rows...)
	})
}

func fundsWrappedActions(actions []layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Wrap{Gap: ui.Space6, Middle: true}).Layout(gtx, len(actions), func(gtx layout.Context, i int) layout.Dimensions {
			return actions[i](gtx)
		})
	}
}
