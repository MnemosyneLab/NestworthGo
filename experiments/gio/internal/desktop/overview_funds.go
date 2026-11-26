package desktop

import (
	"context"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type overviewFunds struct {
	reader     availableFundsReader
	open       func(string, event.Tag)
	cancel     context.CancelFunc
	request    <-chan historyResult[application.LiquidityReadResult]
	current    *application.LiquidityReadResult
	err        error
	dirty      bool
	nextRead   time.Time
	retry      widget.Clickable
	detail     widget.Clickable
	rules      [2]widget.Clickable
	rowHeights ui.RowHeights
}

func (f *overviewFunds) retire() {
	if f.cancel != nil {
		f.cancel()
	}
	f.cancel, f.request = nil, nil
}

func (f *overviewFunds) invalidate() {
	f.retire()
	f.dirty = true
}

func (p *overviewPage) updateFunds(gtx layout.Context) {
	f := &p.funds
	if f.reader == nil || p.current == nil || p.current.value.AccountCount == 0 {
		return
	}
	select {
	case result := <-f.request:
		f.retire()
		f.err = result.err
		if result.err == nil {
			f.current = &result.value
			f.nextRead = gtx.Now.Add(time.Minute)
		}
	default:
	}
	if f.request != nil || p.ctx.Err() != nil {
		return
	}
	if f.dirty || f.err == nil && !f.nextRead.After(gtx.Now) {
		ctx, cancel := context.WithCancel(p.ctx)
		request := make(chan historyResult[application.LiquidityReadResult], 1)
		f.request, f.cancel, f.dirty = request, cancel, false
		reader := f.reader
		p.workers.Go(func() {
			value, err := reader.LiquidityRead(ctx, application.LiquidityOverviewQuery{})
			if ctx.Err() == nil {
				request <- historyResult[application.LiquidityReadResult]{value: value, err: err}
				p.invalidate()
			}
		})
	} else if f.err == nil {
		gtx.Execute(op.InvalidateCmd{At: f.nextRead})
	}
}

func (p *overviewPage) financialRow(gtx layout.Context, view ui.View) layout.Dimensions {
	if p.funds.reader == nil {
		return p.totals(gtx, view)
	}
	grid := ui.Grid{MinColumn: 392, MaxColumns: 2, Gap: ui.Space16, Rows: &p.funds.rowHeights}
	return grid.Layout(gtx, view, 2, func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
		if index == 0 {
			return p.totals(gtx, view)
		}
		return p.funds.card(view)(gtx)
	})
}

func (f *overviewFunds) card(view ui.View) layout.Widget {
	l := view.Language
	caption := func(id i18n.MessageID) layout.Widget {
		return view.Body(ui.TextCaption, l.Text(id), view.Colors.SecondaryInk)
	}
	var fields []layout.Widget
	switch {
	case f.request != nil && (f.current == nil || f.err != nil):
		fields = append(fields, view.Message(l.Text(i18n.FundsLoading), view.Colors.SecondaryInk))
	case f.err != nil:
		fields = append(fields, view.Message(l.Text(i18n.FundsReadFailed), view.Colors.ErrorInk), func(gtx layout.Context) layout.Dimensions {
			if f.retry.Clicked(gtx) {
				f.dirty = true
				gtx.Execute(op.InvalidateCmd{})
			}
			return returnMonthButton(view, &f.retry, i18n.HistoryRetry, nil, true)(gtx)
		})
	case f.current != nil:
		today := f.current.Overview.LocalDate
		first := f.horizon(view, today, i18n.OverviewFundsToday, &f.rules[0])
		month := f.horizon(view, fundsPresetDay(today, 30), i18n.FundsHorizonMonth, &f.rules[1])
		fields = append(fields, func(gtx layout.Context) layout.Dimensions {
			if ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, 345) {
				return (layout.Flex{Gap: gtx.Dp(ui.Space16)}).Layout(gtx, layout.Flexed(1, first), layout.Flexed(1, month))
			}
			return (ui.Column{Gap: ui.Space12}).Layout(gtx, ui.Rigid(first), ui.Rigid(month))
		}, caption(i18n.OverviewFundsPriceHint))
		if f.open != nil {
			label := l.Text(i18n.OverviewOpenFunds)
			fields = append(fields, view.Hairline, func(gtx layout.Context) layout.Dimensions {
				return layout.E.Layout(gtx, f.link(view, &f.detail, label, label, ""))
			})
		}
	}
	return fundsCard(view, l.Text(i18n.NavAvailableFunds), fields...)
}

func (f *overviewFunds) horizon(view ui.View, day string, title i18n.MessageID, rules *widget.Clickable) layout.Widget {
	l := view.Language
	bucket := fundsBucketForHorizon(f.current.Overview.Buckets, day)
	status, hint, tone := i18n.OverviewFundsUnavailable, i18n.FundsUnknownHint, ui.BadgeNeutral
	var amount *domain.Money
	if bucket != nil {
		status = fundsStatus(bucket.Status)
		switch bucket.Status {
		case domain.StatusComplete:
			tone = ui.BadgeSuccess
		case domain.StatusPartial:
			tone = ui.BadgeWarning
		default:
			status = i18n.OverviewFundsUnavailable
		}
		amount = bucket.FullUnreserved
		if amount != nil {
			hint = i18n.FundsSpendableHint
		} else if amount = bucket.KnownUnreservedSubtotal; amount != nil {
			hint = i18n.FundsSubtotalHint
		}
	}
	display := l.Text(i18n.OverviewFundsUnknownAmount)
	if amount != nil {
		display = fundsAmountText(view, amount)
	}
	titleText := l.Text(title)
	rulesLine := view.Body(ui.TextCaption, l.Text(i18n.FundsNoMissingRules), view.Colors.SecondaryInk)
	if count := fundsSourceCount(f.current.Sources, day, fundsNeedsRules); count > 0 {
		label := fundsMissingRuleText(l, count)
		rulesLine = view.Body(ui.TextCaption, label, view.Colors.SecondaryInk)
		if f.open != nil {
			rulesLine = f.link(view, rules, label, titleText+" · "+label, day)
		}
	}
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: ui.Space4}).Layout(gtx,
			ui.Rigid(view.Body(ui.TextLabel, titleText, view.Colors.SecondaryInk)),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions { return view.Badge(gtx, tone, l.Text(status)) }),
			ui.Rigid(view.Body(ui.TextMoneyMD, display, view.Colors.Ink)),
			ui.Rigid(view.Body(ui.TextCaption, l.Text(hint), view.Colors.SecondaryInk)),
			ui.Rigid(rulesLine))
	}
}

func (f *overviewFunds) link(view ui.View, button *widget.Clickable, title, name, day string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if button.Clicked(gtx) {
			f.open(day, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonLink, Size: ui.ButtonSmall, Title: title, Name: name,
			Content: view.Body(ui.TextCaption, title, view.Colors.Accent)})(gtx)
	}
}
