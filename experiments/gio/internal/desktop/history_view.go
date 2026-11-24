package desktop

import (
	"fmt"
	"slices"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type historyPage struct {
	model                                 *history.Model
	list                                  widget.List
	reloadButton, retryButton, moreButton widget.Clickable
	filter                                historyFilterControls
	cashMovements                         *historyCashMovements
	reads                                 *historyReads
	bound, revision                       uint64
	onPosted                              func()
}

func newHistoryPage(model *history.Model) *historyPage {
	return &historyPage{model: model, list: widget.List{List: layout.List{Axis: layout.Vertical}}}
}

var historyKindLabels = map[domain.ActivityKind]i18n.MessageID{
	domain.ActivityCashIn:           i18n.HistoryCashIn,
	domain.ActivityCashOut:          i18n.HistoryCashOut,
	domain.ActivityCashDividend:     i18n.HistoryCashDividend,
	domain.ActivityCashTransfer:     i18n.HistoryCashTransfer,
	domain.ActivityFXConversion:     i18n.HistoryFXConversion,
	domain.ActivityPositionTransfer: i18n.HistoryPositionTransfer,
	domain.ActivityCostAdjustment:   i18n.HistoryCostAdjustment,
	domain.ActivityBuy:              i18n.HistoryBuy,
	domain.ActivitySell:             i18n.HistorySell,
	domain.ActivityValueUpdate:      i18n.HistoryValueUpdate,
	domain.ActivityDebtDraw:         i18n.HistoryDebtDraw,
	domain.ActivityDebtPayment:      i18n.HistoryDebtPayment,
	domain.ActivityReversal:         i18n.HistoryReversal,
}

func (p *historyPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	oldRows, pos := p.model.Rows(), p.list.Position
	if p.model.Update() {
		p.list.Position = layout.Position{}
		if (pos.First > 0 || pos.Offset > 0) && pos.First >= 0 && pos.First < len(oldRows) {
			at := slices.IndexFunc(p.model.Rows(), func(row history.Row) bool { return row.ID == oldRows[pos.First].ID })
			if at >= 0 {
				pos.First = at
				p.list.Position = restoredPosition(pos, len(p.model.Rows()))
			}
		}
		if p.reads != nil {
			p.pruneDetailButtons()
		}
	}
	if p.reads != nil {
		p.reads.update()
		if p.reads.detail != nil {
			return p.layoutDetail(gtx, view)
		}
	}
	if c := p.cashMovements; c != nil {
		if c.draft == nil {
			for i := range c.actions {
				if c.actions[i].Clicked(gtx) {
					c.start(history.CashMovement(i))
					break
				}
			}
		}
		if c.draft != nil {
			return c.layoutForm(gtx, view)
		}
	}
	if p.reloadButton.Clicked(gtx) {
		p.model.Reload()
		if p.reads != nil {
			p.reads.loadReferences()
		}
	}
	if p.retryButton.Clicked(gtx) {
		p.model.Retry()
	}
	if p.moreButton.Clicked(gtx) {
		p.model.LoadMore()
		gtx.Execute(key.FocusCmd{Tag: &p.list})
	}

	filterView := view
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(12)}.Layout(ui.LimitWidth(gtx, 780),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.cashMovements == nil {
				return layout.Dimensions{}
			}
			var actions []layout.FlexChild
			for i := range p.cashMovements.actions {
				actions = append(actions, layout.Rigid(view.Button(&p.cashMovements.actions[i], view.Language.Text(cashMovementLabels[i].action))))
			}
			dims := layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, actions...)
			filterView.Origin.Y += dims.Size.Y + gtx.Dp(12)
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutFilters(gtx, filterView) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.status(gtx, view) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.model.Err() != nil {
						return view.Button(&p.retryButton, view.Language.Text(i18n.HistoryRetry))(gtx)
					}
					return layout.Dimensions{}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.model.Err() != nil && len(p.model.Rows()) == 0 {
						return layout.Dimensions{}
					}
					if p.model.Loading() {
						gtx = gtx.Disabled()
					}
					return view.Button(&p.reloadButton, view.Language.Text(i18n.HistoryRefresh))(gtx)
				}),
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(p.model.Rows()) == 0 || p.noHistory() {
				return layout.Dimensions{}
			}
			return p.activityList(gtx, view)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !p.model.HasMore() || p.model.Err() != nil || p.noHistory() {
				return layout.Dimensions{}
			}
			if p.model.Loading() {
				gtx = gtx.Disabled()
			}
			return view.Button(&p.moreButton, view.Language.Text(i18n.HistoryMore))(gtx)
		}),
	)
}

func (p *historyPage) status(gtx layout.Context, view ui.View) layout.Dimensions {
	color := view.Colors.SecondaryInk
	var text string
	switch {
	case p.noHistory():
		text = view.Language.Text(i18n.HistoryUnavailable)
	case p.model.Loading():
		text = view.Language.Text(i18n.HistoryLoading)
	case p.model.Err() != nil:
		color = view.Colors.ErrorInk
		text = view.Language.Text(i18n.HistoryLoadError)
	case len(p.model.Rows()) == 0:
		if p.model.Filters() != (history.Filters{}) {
			text = view.Language.Text(i18n.HistoryNoMatches)
		} else {
			text = view.Language.Text(i18n.HistoryEmpty)
		}
	default:
		text = fmt.Sprintf(view.Language.Text(i18n.HistoryCount), len(p.model.Rows()))
	}
	return view.Message(text, color)(gtx)
}

func (p *historyPage) activityList(gtx layout.Context, view ui.View) layout.Dimensions {
	border := view.Colors.Divider
	if gtx.Focused(&p.list) {
		border = view.Colors.Accent
	}
	return view.Panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
			view.Focus.Watch(gtx, &p.list)
			semantic.DescriptionOp(view.Language.Text(i18n.NavHistory)).Add(gtx.Ops)
			event.Op(gtx.Ops, &p.list)
			for _, pressed := range ui.PressedKeys(gtx, &p.list,
				key.NameUpArrow, key.NameDownArrow, key.NamePageUp, key.NamePageDown, key.NameHome, key.NameEnd) {
				page := float32(max(1, p.list.Position.Count-1))
				switch pressed.Name {
				case key.NameUpArrow:
					p.list.ScrollBy(-1)
				case key.NameDownArrow:
					p.list.ScrollBy(1)
				case key.NamePageUp:
					p.list.ScrollBy(-page)
				case key.NamePageDown:
					p.list.ScrollBy(page)
				case key.NameHome:
					p.list.ScrollTo(0)
				case key.NameEnd:
					p.list.ScrollTo(len(p.model.Rows()) - 1)
				}
			}
			return accessibility.List(gtx, &p.list, view.Language.Text(i18n.NavHistory), func(gtx layout.Context) layout.Dimensions {
				return material.List(view.Theme, &p.list).Layout(gtx, len(p.model.Rows()), func(gtx layout.Context, i int) layout.Dimensions {
					row := p.model.Rows()[i]
					return accessibility.Row(gtx, row.ID, func(gtx layout.Context) layout.Dimensions {
						return p.layoutActivityRow(gtx, view, row)
					})
				})
			})
		})
	})
}

func (p *historyPage) layoutActivityRow(gtx layout.Context, view ui.View, row history.Row) layout.Dimensions {
	if p.reads == nil {
		return historyEntry(gtx, view, row)
	}
	button := p.detailButton(row.ID)
	if button.Clicked(gtx) {
		p.reads.open(row.ID, button)
		gtx.Execute(op.InvalidateCmd{})
	}
	name := fmt.Sprintf(view.Language.Text(i18n.HistoryViewOn), view.Language.Text(historyKindLabel(row.Kind, row.Reason)), historyDay(view, row.EffectiveLocalDate))
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return historyEntry(gtx, view, row) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			view.Focus.Watch(gtx, button)
			return view.NamedButton(button, view.Language.Text(i18n.DirectoryView), name)(gtx)
		}),
	)
}

func historyDay(view ui.View, day string) string {
	if parsed, ok := ui.ParseCivilDay(day); ok {
		return ui.FormatDay(parsed, view.Language, view.DateFormat)
	}
	return day
}

func historyEntry(gtx layout.Context, view ui.View, row history.Row) layout.Dimensions {
	day := historyDay(view, row.EffectiveLocalDate)
	kind := historyKindLabel(row.Kind, row.Reason)
	return layout.Inset{Top: 9, Bottom: 9, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(3)}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return view.Label(gtx, view.Language.Text(kind), 15, font.Medium, view.Colors.Ink)
					}),
					layout.Rigid(view.Message(day, view.Colors.SecondaryInk)),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if row.Note == nil || *row.Note == "" {
					return layout.Dimensions{}
				}
				return view.Message(*row.Note, view.Colors.SecondaryInk)(gtx)
			}),
		)
	})
}
