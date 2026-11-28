package desktop

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
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

func fundsComparisonMoney(view ui.View, value *domain.Money) string {
	if value == nil {
		return view.Language.Text(i18n.OverviewFundsUnknownAmount)
	}
	amount := value.Amount()
	return fundsComparisonAmount(view, &amount, value.Currency())
}

func fundsComparisonAmount(view ui.View, value *decimal.Decimal, currency domain.CurrencyCode) string {
	if value == nil {
		return view.Language.Text(i18n.OverviewFundsUnknownAmount)
	}
	if !value.IsZero() && value.RoundBank(int32(domain.CurrencyFractionDigits(currency))).IsZero() {
		return format.AuditMoney(value.String(), currency.String(), settings.Settings{
			DecimalSeparator: view.DecimalSeparator, GroupingSeparator: view.GroupingSeparator,
		})
	}
	return fundsNativeAmountText(view, value, currency)
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
		{beforeLabel, fundsComparisonMoney(view, before)},
		{reserveLabel, fundsComparisonMoney(view, bucket.AppliedReserveSubtotal)},
		{afterLabel, fundsComparisonMoney(view, after)},
	}
	title := fmt.Sprintf(view.Language.Text(i18n.FundsBase), code)
	return fundsCompactRow(view, true, true, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space4)}.Layout(gtx,
			layout.Rigid(fundsTitleWithStatus(view, title, title, bucket.Status)),
			layout.Rigid(fundsFigures(view, figures)),
		)
	})
}

func fundsBreakdown(view ui.View, horizon fundsHorizon, amounts map[domain.CurrencyCode]application.LiquidityNativeAmounts) layout.Widget {
	l := view.Language
	groups := horizon.bucket.NativeCurrencyGroups
	window := l.Text(horizon.label) + " · " + historyDay(view, horizon.bucket.HorizonOn)
	labels := [...]i18n.MessageID{i18n.FundsNativeCash, i18n.FundsNativeProceeds, i18n.FundsApplied, i18n.FundsAvailableAfter}
	return func(gtx layout.Context) layout.Dimensions {
		wide := gtx.Constraints.Max.X >= gtx.Dp(ui.BreakpointCompact)
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: ui.Space16, Bottom: ui.Space8, Left: ui.Space16, Right: ui.Space16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				rows := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Bottom: ui.Space8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Gap: gtx.Dp(ui.Space8), Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, view.Body(ui.TextLabel, l.Text(i18n.FundsNativeBreakdown), view.Colors.Ink)),
							layout.Rigid(view.Body(ui.TextCaption, window, view.Colors.SecondaryInk)))
					})
				})}
				if len(groups) == 0 {
					rows = append(rows, layout.Rigid(view.Message(l.Text(i18n.OverviewFundsUnknownAmount), view.Colors.SecondaryInk)))
				} else if wide {
					rows = append(rows, layout.Rigid(fundsBreakdownHeadings(view, labels)), layout.Rigid(view.Hairline))
				}
				for i, group := range groups {
					if i != 0 {
						rows = append(rows, layout.Rigid(view.Hairline))
					}
					rows = append(rows, layout.Rigid(fundsBreakdownCurrency(view, group, amounts[group.Currency], wide)))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
			})
		})
	}
}

func fundsBreakdownHeadings(view ui.View, labels [4]i18n.MessageID) layout.Widget {
	l := view.Language
	var cells [5]layout.Widget
	cells[0] = view.Body(ui.TextCaptionStrong, l.Text(i18n.FundsBreakdownCurrency), view.Colors.SecondaryInk)
	for i, id := range labels {
		caption := view.Body(ui.TextCaptionStrong, fundsFigureLabel(l, id), view.Colors.SecondaryInk)
		cells[i+1] = func(gtx layout.Context) layout.Dimensions { return layout.E.Layout(gtx, caption) }
	}
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: ui.Space6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return fundsBreakdownColumns(gtx, cells)
		})
	}
}

func fundsBreakdownCurrency(view ui.View, group domain.NativeCurrencyGroup, parts application.LiquidityNativeAmounts, wide bool) layout.Widget {
	l := view.Language
	after, afterLabel := group.FullUnreserved, i18n.FundsAvailableAfter
	partial := after == nil && group.KnownUnreservedSubtotal != nil
	if partial {
		after, afterLabel = group.KnownUnreservedSubtotal, i18n.FundsAfterSubtotal
	}
	reserveLabel := i18n.FundsApplied
	if group.Status != domain.StatusComplete && group.AppliedReserveSubtotal != nil {
		reserveLabel = i18n.FundsAppliedSubtotal
	}
	figures := [...]fundsFigure{
		{i18n.FundsNativeCash, fundsComparisonAmount(view, parts.Cash, group.Currency)},
		{i18n.FundsNativeProceeds, fundsComparisonAmount(view, parts.Proceeds, group.Currency)},
		{reserveLabel, fundsComparisonMoney(view, group.AppliedReserveSubtotal)},
		{afterLabel, fundsComparisonMoney(view, after)},
	}
	name := group.Currency.String()
	currency := func(gtx layout.Context) layout.Dimensions {
		return semanticRow(gtx, fmt.Sprintf(l.Text(i18n.FundsNative), name), view.Body(ui.TextLabel, name, view.Colors.Ink))
	}
	return func(gtx layout.Context) layout.Dimensions {
		if wide {
			cells := [5]layout.Widget{fundsBreakdownInset(currency)}
			for i, value := range figures {
				cells[i+1] = fundsBreakdownCell(view, value, i == len(figures)-1, partial && i == len(figures)-1)
			}
			return fundsBreakdownColumns(gtx, cells)
		}
		rows := []layout.FlexChild{layout.Rigid(currency)}
		for i, value := range figures {
			amount := fundsBreakdownCell(view, value, i == len(figures)-1, partial && i == len(figures)-1)
			label := view.Body(ui.TextCaption, fundsFigureLabel(l, value.pattern), view.Colors.SecondaryInk)
			rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Gap: gtx.Dp(ui.Space8), Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, label), layout.Flexed(1, amount))
			}))
		}
		return layout.Inset{Top: ui.Space8, Bottom: ui.Space4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
		})
	}
}

func fundsBreakdownInset(content layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: ui.Space6, Bottom: ui.Space6}.Layout(gtx, content)
	}
}

func fundsBreakdownCell(view ui.View, figure fundsFigure, featured, partial bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return semanticRow(gtx, fmt.Sprintf(view.Language.Text(figure.pattern), figure.value), func(gtx layout.Context) layout.Dimensions {
			style := ui.TextBody
			if featured {
				style = ui.TextLabel
			}
			amount := fundsBreakdownInset(func(gtx layout.Context) layout.Dimensions {
				return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					value := view.Body(style, figure.value, view.Colors.Ink)
					if !partial {
						return value(gtx)
					}
					return (ui.Wrap{Gap: ui.Space4, Middle: true}).Layout(gtx, 2, func(gtx layout.Context, i int) layout.Dimensions {
						if i == 0 {
							return value(gtx)
						}
						return view.Badge(gtx, ui.BadgeWarning, view.Language.Text(i18n.FundsPartialShort))
					})
				})
			})
			if !featured {
				return amount(gtx)
			}
			return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				ui.FillRoundRect(gtx, image.Rectangle{Max: gtx.Constraints.Min}, 0, ui.Mix(view.Colors.Surface, view.Colors.Accent, 0.08))
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: ui.Space8, Right: ui.Space8}.Layout(gtx, amount)
			})
		})
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

type fundsSourceValues struct {
	current, expected, spendable fundsFigure
	applied                      *fundsFigure
	early                        bool
}

const fundsSourceTableWidth = 800

// Use the space inside the list body. At the compact window size the source
// identity, three values and scoped controls need the labelled row layout.
func wideFundsSources(gtx layout.Context) bool {
	return gtx.Constraints.Max.X >= gtx.Dp(fundsSourceTableWidth)
}

func fundsSourceColumns(gtx layout.Context, cells [5]layout.Widget) layout.Dimensions {
	return layout.Flex{Alignment: layout.Start, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
		layout.Flexed(1.3, cells[0]), layout.Flexed(0.8, cells[1]), layout.Flexed(1.05, cells[2]),
		layout.Flexed(1.15, cells[3]), layout.Flexed(1.3, cells[4]))
}

func fundsSourceHeadings(view ui.View) layout.Widget {
	l := view.Language
	return fundsCompactRow(view, true, true, func(gtx layout.Context) layout.Dimensions {
		labels := [5]string{l.Text(i18n.FundsAssetColumn), fundsFigureLabel(l, i18n.FundsCurrent),
			fundsFigureLabel(l, i18n.FundsLater), l.Text(i18n.FundsSpendableColumn), l.Text(i18n.FundsActionColumn)}
		var cells [5]layout.Widget
		for i, label := range labels {
			cell := view.Message(label, view.Colors.SecondaryInk)
			if i > 0 && i < len(cells)-1 {
				cell = func(gtx layout.Context) layout.Dimensions {
					return layout.E.Layout(gtx, view.Message(label, view.Colors.SecondaryInk))
				}
			}
			cells[i] = cell
		}
		return fundsSourceColumns(gtx, cells)
	})
}

func (v fundsSourceValues) expectedCell(view ui.View) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if !v.early {
			return v.expected.layout(view, false)(gtx)
		}
		return (ui.Column{Gap: ui.Space4}).Layout(gtx, ui.Rigid(v.expected.layout(view, false)), ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.E.Layout(gtx, view.Message(view.Language.Text(i18n.FundsEarlyRoute), view.Colors.WarningInk))
		}))
	}
}

func (v fundsSourceValues) spendableCell(view ui.View) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if v.applied == nil {
			return v.spendable.layout(view, false)(gtx)
		}
		reserved := fmt.Sprintf(view.Language.Text(v.applied.pattern), v.applied.value)
		return (ui.Column{Gap: ui.Space4}).Layout(gtx, ui.Rigid(v.spendable.layout(view, false)), ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.E.Layout(gtx, view.Message(reserved, view.Colors.SecondaryInk))
		}))
	}
}

func fundsSourceIdentity(view ui.View, name, account string, source domain.LiquiditySourceResult) layout.Widget {
	l := view.Language
	var status i18n.MessageID
	tone := ui.BadgeNeutral
	switch {
	case source.Excluded:
		status = i18n.FundsFilterExcluded
	case source.DisplayState == domain.ProductDisplayNeedsInfo:
		status, tone = i18n.FundsFilterNeedsInfo, ui.BadgeWarning
	case source.DisplayState == domain.ProductDisplayLocked:
		status, tone = i18n.ProductLocked, ui.BadgeOutline
	case source.DisplayState == domain.ProductDisplayRedeemable:
		status, tone = i18n.ProductRedeemable, ui.BadgeSuccess
		if source.ProductID == nil {
			status = i18n.FundsCashReady
			if source.Ref.Kind == domain.SourceHolding {
				status = i18n.FundsHoldingRedeemable
			}
		}
	case source.DisplayState == domain.ProductDisplaySettled:
		status = i18n.ProductSettled
	case source.DisplayState == domain.ProductDisplayCancelled:
		status = i18n.ProductCancelled
	}
	rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextLabel, name, view.Colors.Ink))}
	if account != "" && account != name {
		rows = append(rows, ui.Rigid(view.Body(ui.TextCaption, account, view.Colors.SecondaryInk)))
	}
	if status != i18n.NoMessage {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Badge(gtx, tone, l.Text(status))
		}))
	}
	if source.DueUnconfirmed && !source.Excluded {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Badge(gtx, ui.BadgeWarning, l.Text(i18n.FundsDue))
		}))
	}
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: ui.Space4}).Layout(gtx, rows...)
	}
}

func fundsSourceRow(view ui.View, title string, buttons *fundsLinks, identity layout.Widget, values fundsSourceValues, primary, menu layout.Widget, details []layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		wide := wideFundsSources(gtx)
		return fundsCompactRow(view, true, true, func(gtx layout.Context) layout.Dimensions {
			var rows []layout.FlexChild
			actions := fundsSourceActions(primary, menu)
			if wide {
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return fundsSourceColumns(gtx, [5]layout.Widget{identity, values.current.layout(view, false),
						values.expectedCell(view), values.spendableCell(view), actions})
				}))
			} else {
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Start, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
						layout.Flexed(1, identity), layout.Rigid(actions))
				}))
				figures := []fundsFigure{values.current, values.expected, values.spendable}
				if values.applied != nil {
					figures = append(figures, *values.applied)
				}
				rows = append(rows, layout.Rigid(fundsFigures(view, figures)))
				if values.early {
					rows = append(rows, layout.Rigid(view.Message(view.Language.Text(i18n.FundsEarlyRoute), view.Colors.WarningInk)))
				}
			}
			if len(details) != 0 {
				l := view.Language
				if buttons.details.Clicked(gtx) {
					buttons.showDetails = !buttons.showDetails
					gtx.Execute(op.InvalidateCmd{})
				}
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					view.Focus.Watch(gtx, &buttons.details)
					return view.ButtonOf(&buttons.details, ui.ButtonSpec{
						Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Toggle: true, Selected: buttons.showDetails,
						Title: l.Text(i18n.FundsSourceDetails), Name: fmt.Sprintf(l.Text(i18n.FundsNamedSourceDetails), title),
					})(gtx)
				}))
				if buttons.showDetails {
					for _, detail := range details {
						rows = append(rows, layout.Rigid(detail))
					}
				}
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space4)}.Layout(gtx, rows...)
		})(gtx)
	}
}

func fundsSourceActions(primary, menu layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		var children []layout.FlexChild
		if primary != nil {
			children = append(children, layout.Rigid(primary))
		}
		if menu != nil {
			children = append(children, layout.Rigid(menu))
		}
		return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Start, Gap: gtx.Dp(ui.Space4)}.Layout(gtx, children...)
		})
	}
}

func fundsWrappedActions(actions []layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Wrap{Gap: ui.Space6, Middle: true}).Layout(gtx, len(actions), func(gtx layout.Context, i int) layout.Dimensions {
			return actions[i](gtx)
		})
	}
}
