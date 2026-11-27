package desktop

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type historyRecordSheet struct {
	trigger      widget.Clickable
	actions      [len(cashMovementLabels)]widget.Clickable
	sheet        analysisSheet
	overlay      ui.Overlay
	open         bool
	restoreFocus bool
}

func (s *shell) historyRecordOpen() bool {
	return s.activeRoute() == historyRoute && s.historyPage != nil && s.historyPage.record.open
}

func (p *historyPage) canRecord() bool {
	return p != nil && p.cashMovements != nil && p.reads != nil && p.reads.detail == nil
}

func (p *historyPage) updateRecordTrigger(gtx layout.Context) {
	if !p.canRecord() || p.record.open || !p.record.trigger.Clicked(gtx) {
		return
	}
	p.record.open = true
	p.record.sheet = newAnalysisSheet()
	p.record.sheet.width, p.record.sheet.modal = 680, true
	p.cashMovements.start(history.CashContribution)
	// Opening the sheet focuses Close. A later account read must not steal focus.
	p.cashMovements.draft.focusAmount = false
}

func (p *historyPage) layoutRecordTrigger(gtx layout.Context, view ui.View) layout.Dimensions {
	if p.record.restoreFocus && gtx.Enabled() {
		p.record.restoreFocus = false
		gtx.Execute(key.FocusCmd{Tag: &p.record.trigger})
	}
	return view.PrimaryButton(&p.record.trigger, view.Language.Text(i18n.HistoryRecordChange))(gtx)
}

func (p *historyPage) closeRecord() {
	p.record.open = false
	p.record.overlay = ui.Overlay{}
	if p.cashMovements.draft != nil {
		p.cashMovements.discard()
	}
	if p.investment != nil {
		p.investment.draft, p.investment.restore = nil, nil
	}
}

func (p *historyPage) recordPosted(activity domain.Activity, notice i18n.MessageID, focus event.Tag) {
	if p.record.open {
		p.closeRecord()
		focus = &p.record.trigger
	}
	p.showPosted(activity, notice, focus)
}

func (p *historyPage) layoutRecord(gtx layout.Context, view ui.View) {
	r := &p.record
	cash, investments := p.cashMovements, p.investment
	view.Overlay = &r.overlay
	r.overlay.BeginFrame(gtx)
	canClose := !cash.writing() && !investments.unresolved()
	if r.overlay.Trigger == nil {
		dismiss := r.sheet.dismissed(gtx)
		for {
			e, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				break
			}
			dismiss = dismiss || e.(key.Event).State == key.Press
		}
		if dismiss && canClose {
			p.dismissRecord(gtx)
			return
		}
	}
	title := view.Language.Text(i18n.HistoryRecordChange)
	r.sheet.layoutPanel(gtx, view, title, func(gtx layout.Context, v ui.View) layout.Dimensions {
		bounds := image.Rectangle{Min: v.Origin, Max: v.Origin.Add(gtx.Constraints.Max)}
		v = v.WithScrollAnchors(&r.sheet.list)
		r.overlay.ScrollOutsidePanel(gtx, &r.sheet.list, bounds)
		dims := v.List(&r.sheet.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			return p.layoutRecordForm(gtx, v)
		})
		r.overlay.OffsetScrollAnchor(&r.sheet.list, bounds)
		return dims
	}, nil, canClose)
	if cash.draft == nil && (investments == nil || investments.draft == nil) {
		p.dismissRecord(gtx)
		return
	}
	r.overlay.Layout(gtx, view)
}

func (p *historyPage) dismissRecord(gtx layout.Context) {
	p.closeRecord()
	p.record.restoreFocus = true
	gtx.Execute(op.InvalidateCmd{})
}

func (p *historyPage) layoutRecordForm(gtx layout.Context, view ui.View) layout.Dimensions {
	cash, investments := p.cashMovements, p.investment
	if cash.draft != nil && cash.draft.review != nil {
		return cash.layoutForm(gtx, view)
	}
	if investments != nil && investments.draft != nil && investments.draft.review != nil {
		return investments.layoutForm(gtx, view, p.investmentRuntime())
	}
	var choicesHeight int
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(16)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := p.layoutRecordChoices(gtx, view)
			choicesHeight = dims.Size.Y
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			view.Origin.Y += choicesHeight + gtx.Dp(16)
			if cash.draft != nil {
				return cash.layoutForm(gtx, view)
			}
			return investments.layoutForm(gtx, view, p.investmentRuntime())
		}),
	)
}

func (p *historyPage) layoutRecordChoices(gtx layout.Context, view ui.View) layout.Dimensions {
	cash, investments := p.cashMovements, p.investment
	group := func(title i18n.MessageID, buttons []layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
				layout.Rigid(view.Body(ui.TextLabel, view.Language.Text(title), view.Colors.SecondaryInk)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.Wrap{Gap: ui.Space8}.Layout(gtx, len(buttons), func(gtx layout.Context, i int) layout.Dimensions {
						return buttons[i](gtx)
					})
				}),
			)
		})
	}
	movement := func(kind history.CashMovement) layout.Widget {
		button := &p.record.actions[kind]
		return func(gtx layout.Context) layout.Dimensions {
			selected := cash.draft != nil && cash.draft.movement == kind
			if button.Clicked(gtx) && !selected {
				p.replaceRecordDraft(gtx)
				cash.start(kind)
				selected = true
			}
			return view.TabButton(button, view.Language.Text(cashMovementLabels[kind].action), selected)(gtx)
		}
	}
	groups := []layout.FlexChild{group(i18n.HistoryEntryCash, []layout.Widget{
		movement(history.CashContribution), movement(history.CashWithdrawal), movement(history.CashTransfer), movement(history.CashConversion),
	})}
	if p.positionActionsAvailable() {
		position := func(button *widget.Clickable, label i18n.MessageID, start func(investmentRuntime, positionScope)) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				selected := investments.draft != nil && investments.action(investments.draft) == button
				if button.Clicked(gtx) && !selected {
					p.replaceRecordDraft(gtx)
					start(p.investmentRuntime(), positionScope{})
					selected = true
				}
				return view.TabButton(button, view.Language.Text(label), selected)(gtx)
			}
		}
		groups = append(groups, group(i18n.HistoryEntryInvestments, []layout.Widget{
			position(&investments.custodyOpen, i18n.CustodyOpen, investments.startCustody),
			position(&investments.adjustmentOpen, i18n.QuantityAdjustOpen, investments.startAdjustment),
		}))
	}
	groups = append(groups, group(i18n.HistoryEntryDebt, []layout.Widget{
		movement(history.CashDebtDraw), movement(history.CashDebtPayment),
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, groups...)
}

func (p *historyPage) replaceRecordDraft(gtx layout.Context) {
	if p.cashMovements.draft != nil {
		p.cashMovements.discard()
	}
	if p.investment != nil {
		p.investment.draft, p.investment.restore = nil, nil
	}
	p.record.overlay.Close()
	p.record.sheet.list.Position = layout.Position{}
	gtx.Execute(op.InvalidateCmd{})
}
