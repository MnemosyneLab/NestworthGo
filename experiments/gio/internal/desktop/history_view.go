package desktop

import (
	"fmt"
	"image"
	"slices"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
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
	investment                            *accountInvestment
	record                                historyRecordSheet
	startPoint                            *historyStart
	reads                                 *historyReads
	bound, revision                       uint64
	onPosted                              func()
}

func newHistoryPage(model *history.Model) *historyPage {
	return &historyPage{model: model, list: widget.List{List: layout.List{Axis: layout.Vertical}}}
}

// historyListMinHeight keeps a few rows visible. When the controls above plus
// this minimum exceed the window, the stage scrolls instead of squeezing the
// list.
const historyListMinHeight unit.Dp = 220

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
	if start := p.startPoint; start != nil {
		if p.noHistory() && start.start.Clicked(gtx) {
			p.beginStartingPoint(start)
		}
		if start.open {
			return p.layoutStartingPoint(gtx, view, start)
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

	return p.layoutActivity(gtx, view)
}

// layoutActivity stacks the filters, status and activity list. The
// list takes the height left in the window, never less than its minimum.
func (p *historyPage) layoutActivity(gtx layout.Context, view ui.View) layout.Dimensions {
	return ui.Column{Gap: 12, MinFill: historyListMinHeight}.Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutFilters(gtx, view) }),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if p.startPoint == nil || !p.startPoint.ready || p.reads.references == nil || p.reads.references.Origin == nil {
						return p.status(gtx, view)
					}
					return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
						layout.Rigid(view.Message(view.Language.Text(i18n.HistoryStartSaved), view.Colors.Ink)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.status(gtx, view) }),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.startPoint == nil || !p.noHistory() {
						return layout.Dimensions{}
					}
					return view.Button(&p.startPoint.start, view.Language.Text(i18n.HistoryStartAction))(gtx)
				}),
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
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			if len(p.model.Rows()) == 0 || p.noHistory() {
				return layout.Dimensions{}
			}
			return p.activityList(gtx, view)
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
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
		if !p.model.Filters().Empty() {
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
	rows := p.model.Rows()
	return pageListGap(gtx, view, &p.list, view.Language.Text(i18n.NavHistory), len(rows), 0, func(gtx layout.Context, i int) layout.Dimensions {
		row := rows[i]
		return view.RowCard(gtx, i == 0, i == len(rows)-1, func(gtx layout.Context) layout.Dimensions {
			return accessibility.Row(gtx, row.ID, func(gtx layout.Context) layout.Dimensions {
				// The day heads the first row of its group, as the Wails
				// list heads each day, so rows need not repeat it.
				heading := i == 0 || rows[i-1].EffectiveLocalDate != row.EffectiveLocalDate
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !heading {
							return layout.Dimensions{}
						}
						return historyDayHeading(gtx, view, row.EffectiveLocalDate, i != 0)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutActivityRow(gtx, view, row) }),
				)
			})
		})
	})
}

func (p *historyPage) layoutActivityRow(gtx layout.Context, view ui.View, row history.Row) layout.Dimensions {
	r := p.reads
	if r == nil {
		summary := historyRowSummary(view, row.Activity, historyRowNames{})
		return historyEntry(gtx, view, summary, row.Note)
	}
	button := p.detailButton(row.ID)
	if button.Clicked(gtx) {
		r.open(row.ID, button)
		gtx.Execute(op.InvalidateCmd{})
	}

	var toggle *widget.Clickable
	if r.hasPotentialRowActions(row.Activity) {
		toggle = p.historyActionButton(row.ID)
		if toggle.Clicked(gtx) {
			if r.rowActions != nil && r.rowActions.id == row.ID {
				r.closeRowActions()
			} else {
				r.openRowActions(row.ID)
			}
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	menu := r.rowActions
	expanded := toggle != nil && menu != nil && menu.id == row.ID
	names := r.rowNames
	if names.holdingAccounts == nil && expanded && menu.record != nil {
		names.holdingAccounts = menu.record.HoldingAccounts
	}
	summary := historyRowSummary(view, row.Activity, names)
	header := func(gtx layout.Context) layout.Dimensions {
		kind := view.Language.Text(historyKindLabel(row.Kind, row.Reason))
		day := historyDay(view, row.EffectiveLocalDate)
		name := fmt.Sprintf(view.Language.Text(i18n.HistoryViewOn), kind, day)
		controls := []layout.FlexChild{
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return historyEntry(gtx, view, summary, row.Note) }),
			layout.Rigid(view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Title: view.Language.Text(i18n.DirectoryView), Name: name})),
		}
		if toggle != nil {
			controls = append(controls, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.layoutRowActionToggle(gtx, view, toggle, kind+" · "+day, expanded)
			}))
		}
		return layout.Inset{Right: ui.Space8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space4)}.Layout(gtx, controls...)
		})
	}
	if !expanded {
		return header(gtx)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(header), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: ui.Space16, Right: ui.Space8, Bottom: ui.Space8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return p.layoutRowActionChoices(gtx, view, menu, toggle)
		})
	}))
}

// historyDayHeading is the muted band that opens a day of activities.
func historyDayHeading(gtx layout.Context, view ui.View, day string, divider bool) layout.Dimensions {
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Min
		paint.FillShape(gtx.Ops, ui.WithAlpha(view.Colors.HoverSurface, 0.6), clip.Rect(image.Rectangle{Max: size}).Op())
		return layout.Dimensions{Size: size}
	}, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		return layout.Inset{Top: 6, Bottom: 6, Left: ui.Space16, Right: ui.Space16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return view.Line(gtx, ui.TextCaption, historyDay(view, day), view.Colors.SecondaryInk)
		})
	})
}

func historyDay(view ui.View, day string) string {
	if parsed, ok := ui.ParseCivilDay(day); ok {
		return ui.FormatDay(parsed, view.Language, view.DateFormat)
	}
	return day
}

func historyEntry(gtx layout.Context, view ui.View, summary string, note *string) layout.Dimensions {
	return layout.Inset{Top: ui.Space12, Bottom: ui.Space12, Left: ui.Space16, Right: ui.Space8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space4)}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.Body(ui.TextLabel, summary, view.Colors.Ink)(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if note == nil || *note == "" {
					return layout.Dimensions{}
				}
				return view.Body(ui.TextCaption, *note, view.Colors.SecondaryInk)(gtx)
			}),
		)
	})
}
