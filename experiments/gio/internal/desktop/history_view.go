package desktop

import (
	"fmt"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type historyPage struct {
	model                                 *history.Model
	list                                  widget.List
	reloadButton, retryButton, moreButton widget.Clickable
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
	if p.model.Update() {
		p.list.Position = layout.Position{}
	}
	if p.reloadButton.Clicked(gtx) {
		p.model.Reload()
	}
	if p.retryButton.Clicked(gtx) {
		p.model.Retry()
	}
	if p.moreButton.Clicked(gtx) {
		p.model.LoadMore()
		gtx.Execute(key.FocusCmd{Tag: &p.list})
	}

	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(12)}.Layout(ui.LimitWidth(gtx, 780),
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
			if len(p.model.Rows()) == 0 {
				return layout.Dimensions{}
			}
			return p.activityList(gtx, view)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !p.model.HasMore() || p.model.Err() != nil {
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
	case p.model.Loading():
		text = view.Language.Text(i18n.HistoryLoading)
	case p.model.Err() != nil:
		color = view.Colors.ErrorInk
		text = view.Language.Text(i18n.HistoryLoadError)
	case len(p.model.Rows()) == 0:
		text = view.Language.Text(i18n.HistoryEmpty)
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
			return material.List(view.Theme, &p.list).Layout(gtx, len(p.model.Rows()), func(gtx layout.Context, i int) layout.Dimensions {
				return historyEntry(gtx, view, p.model.Rows()[i])
			})
		})
	})
}

func historyEntry(gtx layout.Context, view ui.View, row history.Row) layout.Dimensions {
	day := row.EffectiveLocalDate
	if parsed, ok := ui.ParseCivilDay(day); ok {
		day = ui.FormatDay(parsed, view.Language, view.DateFormat)
	}
	kind, ok := historyKindLabels[row.Kind]
	if !ok {
		kind = i18n.HistoryActivity
	}
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
