package desktop

import (
	"context"
	"fmt"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type historyRowAction uint8

const (
	historyRowView historyRowAction = iota
	historyRowRepeat
	historyRowCorrect
	historyRowReverse
)

type historyRowActions struct {
	id                              domain.ActivityID
	cancel                          context.CancelFunc
	read                            <-chan historyResult[history.Detail]
	record                          *history.Detail
	err                             error
	repeat, correct, reverse, retry widget.Clickable
	product                         widget.Clickable
}

func (p *historyPage) historyActionButton(id domain.ActivityID) *widget.Clickable {
	button := p.reads.actionButtons[id]
	if button == nil {
		button = new(widget.Clickable)
		p.reads.actionButtons[id] = button
	}
	return button
}

func (r *historyReads) hasPotentialRowActions(activity domain.Activity) bool {
	if r.openAction == nil || activity.ReversesActivityID != nil {
		return false
	}
	if product := activity.ProductContext; product != nil {
		return r.openProduct != nil && product.ProductKind == domain.ProductTermDeposit
	}
	if _, ok := history.CashAmendment(activity); ok {
		return true
	}
	if _, ok := history.CustodyAmendment(activity); ok {
		return true
	}
	// Shape alone is enough to offer the disclosure. The shared detail read
	// decides whether this exact record is currently eligible.
	return r.repeatAPI != nil && (activity.Kind == domain.ActivityCashIn || activity.Kind == domain.ActivityCashOut) &&
		activity.TradeDetail == nil && activity.DividendDetail == nil && activity.CorrectionGroupID == nil
}

func (r *historyReads) openRowActions(id domain.ActivityID) {
	r.closeRowActions()
	ctx, cancel := context.WithCancel(r.ctx)
	r.rowActions = &historyRowActions{id: id, cancel: cancel, read: r.readDetail(ctx, id)}
}

func (r *historyReads) closeRowActions() {
	if menu := r.rowActions; menu != nil {
		menu.cancel()
		r.rowActions = nil
	}
}

func (r *historyReads) updateRowActions() {
	menu := r.rowActions
	if menu == nil {
		return
	}
	select {
	case result := <-menu.read:
		menu.cancel()
		menu.read, menu.err = nil, result.err
		if result.err == nil {
			menu.record = &result.value
		}
	default:
	}
}

func (r *historyReads) allowsRowAction(record *history.Detail, action historyRowAction) bool {
	switch action {
	case historyRowRepeat:
		return r.repeatAPI != nil && record.Repeatable
	case historyRowCorrect:
		return record.Amendable
	case historyRowReverse:
		return record.Amendable && record.Reversible
	default:
		return false
	}
}

func (r *historyReads) startRequestedRowAction(visit *historyDetailVisit) {
	action := visit.requested
	visit.requested = historyRowView
	if action == historyRowView {
		return
	}
	if !r.allowsRowAction(visit.record, action) {
		visit.actionUnavailable = true
		return
	}
	visit.fromRowAction = true
	switch action {
	case historyRowRepeat:
		r.startRepeat()
	case historyRowCorrect:
		r.startAmendment(history.CorrectActivity)
	case historyRowReverse:
		r.startAmendment(history.ReverseActivity)
	}
}

func historyCorrectLabel(activity domain.Activity) i18n.MessageID {
	if _, custody := history.CustodyAmendment(activity); custody {
		return i18n.CustodyAmendmentCorrect
	}
	return i18n.HistoryCorrectAction
}

func (p *historyPage) layoutRowActionToggle(gtx layout.Context, view ui.View, button *widget.Clickable, summary string, open bool) layout.Dimensions {
	label := view.Language.Text(i18n.MarketMoreActions)
	name := fmt.Sprintf(view.Language.Text(i18n.HistoryRowMoreActions), summary)
	return view.ButtonOf(button, ui.ButtonSpec{
		Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Icon: ui.Glyph(lucide.Ellipsis),
		Name: name, ShortName: label, Toggle: true, Selected: open,
	})(gtx)
}

func (p *historyPage) layoutRowActionChoices(gtx layout.Context, view ui.View, menu *historyRowActions, trigger *widget.Clickable) layout.Dimensions {
	r, l := p.reads, view.Language
	dismiss := func(button *widget.Clickable) bool {
		if len(ui.PressedKeys(gtx, button, key.NameEscape)) == 0 {
			return false
		}
		r.closeRowActions()
		gtx.Execute(key.FocusCmd{Tag: trigger})
		gtx.Execute(op.InvalidateCmd{})
		return true
	}
	if dismiss(trigger) {
		return layout.Dimensions{}
	}
	if menu.read != nil {
		return view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)(gtx)
	}
	if menu.err != nil {
		if dismiss(&menu.retry) {
			return layout.Dimensions{}
		}
		if menu.retry.Clicked(gtx) {
			r.openRowActions(menu.id)
			gtx.Execute(key.FocusCmd{Tag: trigger})
			gtx.Execute(op.InvalidateCmd{})
		}
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
			layout.Flexed(1, view.Message(l.Text(i18n.ContributionDetailFailed), view.Colors.ErrorInk)),
			layout.Rigid(returnMonthButton(view, &menu.retry, i18n.HistoryRetry, nil, true)))
	}
	record := menu.record
	var controls []layout.Widget
	for _, item := range []struct {
		action historyRowAction
		label  i18n.MessageID
		button *widget.Clickable
	}{
		{historyRowRepeat, i18n.HistoryRepeatAction, &menu.repeat},
		{historyRowCorrect, historyCorrectLabel(record.Activity), &menu.correct},
		{historyRowReverse, i18n.HistoryReverseAction, &menu.reverse},
	} {
		if !r.allowsRowAction(record, item.action) {
			continue
		}
		if dismiss(item.button) {
			return layout.Dimensions{}
		}
		if item.button.Clicked(gtx) {
			r.openAction(menu.id, item.action, trigger)
			gtx.Execute(op.InvalidateCmd{})
		}
		controls = append(controls, returnMonthButton(view, item.button, item.label, nil, true))
	}
	if product := record.Activity.ProductContext; product != nil && product.ProductKind == domain.ProductTermDeposit && r.openProduct != nil {
		if name := record.Instruments[product.InstrumentID]; name != "" {
			if dismiss(&menu.product) {
				return layout.Dimensions{}
			}
			controls = append(controls, productLink(view, product.ProductID, name, &menu.product, func(id domain.ProductContractID, name string, _ event.Tag) {
				r.openProduct(id, name, trigger)
			}))
		}
	}
	if len(controls) == 0 {
		return view.Message(l.Text(i18n.HistoryAmendmentUnavailable), view.Colors.SecondaryInk)(gtx)
	}
	return (ui.Wrap{Gap: ui.Space8}).Layout(gtx, len(controls), func(gtx layout.Context, index int) layout.Dimensions {
		return controls[index](gtx)
	})
}
