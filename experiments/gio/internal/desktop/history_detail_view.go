package desktop

import (
	"errors"
	"fmt"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *historyPage) layoutDetail(gtx layout.Context, view ui.View) layout.Dimensions {
	r, l := p.reads, view.Language
	d := r.detail
	if d.back.Clicked(gtx) {
		r.back()
		gtx.Execute(op.InvalidateCmd{})
	}
	if d.err != nil && d.retry.Clicked(gtx) {
		r.loadDetail()
		gtx.Execute(op.InvalidateCmd{})
	}
	title := i18n.HistoryDetailTitle
	known := d.record
	if known != nil {
		title = historyKindLabel(known.Activity.Kind, known.Activity.Reason)
		r.rename(d.id, l.Text(title)+" · "+historyDay(view, known.Activity.EffectiveLocalDate))
	} else if d.id == r.posted.ID {
		title = historyKindLabel(r.posted.Kind, r.posted.Reason)
	}

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(10)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					view.Focus.Watch(gtx, &d.back)
					return view.Button(&d.back, l.Text(i18n.ContributionBack))(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Label(gtx, l.Text(title), 19, font.Medium, view.Colors.Ink)
				}),
			)
		}),
	}
	if d.id == r.posted.ID {
		if movement, supported := history.CashMovementOf(r.posted.Kind, r.posted.Reason); supported {
			children = append(children, layout.Rigid(view.Message(l.Text(cashMovementLabels[movement].posted), view.Colors.Ink)))
		}
	}
	switch {
	case d.pending != nil:
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
	case errors.Is(d.err, history.ErrActivityUnavailable):
		children = append(children, layout.Rigid(view.Message(l.Text(i18n.HistoryActivityUnavailable), view.Colors.SecondaryInk)))
	case d.err != nil:
		children = append(children,
			layout.Rigid(view.Message(l.Text(i18n.ContributionDetailFailed), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&d.retry, l.Text(i18n.HistoryRetry))),
		)
	case known != nil:
		facts := activityFacts(view, *known)
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(view.Theme, &d.scroll).Layout(gtx, len(facts), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Bottom: 8}.Layout(gtx, facts[i])
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(9)}.Layout(ui.LimitWidth(gtx, 720), children...)
}

var effectClasses = map[domain.ActivityClassification]i18n.MessageID{
	domain.ClassificationExternalInflow:   i18n.HistoryExternalInflow,
	domain.ClassificationExternalOutflow:  i18n.HistoryExternalOutflow,
	domain.ClassificationIncome:           i18n.HistoryIncome,
	domain.ClassificationFee:              i18n.HistoryFee,
	domain.ClassificationInternalTransfer: i18n.HistoryInternalTransfer,
	domain.ClassificationTradePrincipal:   i18n.HistoryTradePrincipal,
	domain.ClassificationDebtPrincipal:    i18n.HistoryDebtPrincipal,
	domain.ClassificationRemeasurement:    i18n.HistoryRemeasurement,
}

func activityFacts(view ui.View, detail history.Detail) []layout.Widget {
	activity, l := detail.Activity, view.Language
	ink := func(text string) layout.Widget { return view.Message(text, view.Colors.Ink) }
	muted := func(text string) layout.Widget { return view.Message(text, view.Colors.SecondaryInk) }
	movement, supported := history.CashMovementOf(activity.Kind, activity.Reason)
	paired := supported && (movement == history.CashTransfer || movement == history.CashConversion)
	account := func(id *domain.AccountID) string {
		if id != nil && detail.Accounts[*id] != "" {
			return detail.Accounts[*id]
		}
		return l.Text(i18n.AccountReferenceUnavailable)
	}
	instrument := func(id domain.InstrumentID) string {
		if name := detail.Instruments[id]; name != "" {
			return name
		}
		return l.Text(i18n.HistoryInstrumentUnavailable)
	}
	var facts []layout.Widget
	if detail.Location != nil {
		facts = append(facts, muted(contributionTimestamp(view, activity.EffectiveAt, detail.Location)))
	} else {
		facts = append(facts, muted(historyDay(view, activity.EffectiveLocalDate)), muted(contributionTimestamp(view, activity.EffectiveAt, time.UTC)))
	}
	if activity.Note != nil && *activity.Note != "" {
		facts = append(facts, ink(*activity.Note))
	}
	if activity.ReversesActivityID != nil || activity.CorrectionGroupID != nil {
		badge := i18n.HistoryCorrectionRecord
		if activity.ReversesActivityID != nil {
			badge = i18n.HistoryReversal
		}
		facts = append(facts, muted(l.Text(badge)))
	}
	if trade := activity.TradeDetail; trade != nil {
		facts = append(facts, ink(instrument(trade.InstrumentID)),
			muted(fmt.Sprintf(l.Text(i18n.HistoryUnitPrice), trade.UnitPrice.Canonical()+" "+trade.Gross.Currency().String())))
	} else if dividend := activity.DividendDetail; dividend != nil {
		facts = append(facts, ink(instrument(dividend.InstrumentID)))
	}
	for _, effect := range activity.Effects {
		caption := effectCaption(l, effect, paired, supported, movement, account, instrument)
		class := l.Text(effectClasses[effect.Classification])
		if caption == "" {
			continue
		}
		if supported && !paired && effect.AccountID != nil {
			facts = append(facts, ink(account(effect.AccountID)))
		}
		facts = append(facts, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(14)}.Layout(gtx,
				layout.Flexed(1, ink(caption)), layout.Rigid(muted(class)))
		})
	}
	if paired {
		from, to := transferCurrencies(activity.Effects)
		if rate := transferRateLabel(view, activity.TransactionFXRate, from, to); rate != "" {
			facts = append(facts, ink(rate))
		}
	}
	for _, result := range activity.Resulting {
		if result.Quantity != "" {
			if result.Name != "" {
				facts = append(facts, ink(fmt.Sprintf(l.Text(i18n.HistoryResultQuantity), result.Name, result.Quantity)))
			}
			continue
		}
		if result.Amount == "" || result.Currency == "" {
			continue
		}
		balance := result.Amount + " " + result.Currency.String()
		if paired {
			facts = append(facts, ink(fmt.Sprintf(l.Text(i18n.TransferAccountBalance), account(result.AccountID), balance)))
		} else {
			facts = append(facts, ink(fmt.Sprintf(l.Text(i18n.ContributionAfter), balance)))
		}
	}
	return facts
}

func effectCaption(l i18n.Language, effect domain.ActivityEffect, paired, supported bool, movement history.CashMovement, account func(*domain.AccountID) string, instrument func(domain.InstrumentID) string) string {
	if effect.Money != nil {
		amount := movementMoney(effect)
		if paired {
			label := map[domain.EffectRole]i18n.MessageID{
				domain.EffectRoleTransferFrom: i18n.TransferSent,
				domain.EffectRoleTransferTo:   i18n.TransferReceived,
				domain.EffectRoleFee:          i18n.TransferFeePaid,
			}[effect.Role]
			if label != i18n.NoMessage {
				return fmt.Sprintf(l.Text(label), account(effect.AccountID), amount)
			}
		}
		if supported && !paired {
			return fmt.Sprintf(l.Text(cashMovementLabels[movement].delta), amount)
		}
		label := i18n.HistoryAdded
		if effect.Direction == domain.EffectRemoved {
			label = i18n.HistoryRemoved
		}
		return fmt.Sprintf(l.Text(label), account(effect.AccountID), amount)
	}
	if effect.InstrumentID != nil {
		name := instrument(*effect.InstrumentID)
		if effect.Quantity != nil {
			label := i18n.HistoryAdded
			if effect.Direction == domain.EffectRemoved {
				label = i18n.HistoryRemoved
			}
			return fmt.Sprintf(l.Text(label), name, effect.Quantity.Canonical())
		}
		if effect.CostUnitPrice != nil {
			return name + " · " + fmt.Sprintf(l.Text(i18n.HistoryUnitCost), effect.CostUnitPrice.Canonical())
		}
	}
	return ""
}
