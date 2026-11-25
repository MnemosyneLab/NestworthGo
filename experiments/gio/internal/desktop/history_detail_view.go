package desktop

import (
	"errors"
	"fmt"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *historyPage) layoutDetail(gtx layout.Context, view ui.View) layout.Dimensions {
	r, l := p.reads, view.Language
	d := r.detail
	if d.repeatDraft != nil {
		return p.layoutRepeat(gtx, view)
	}
	if d.amendment != nil {
		return p.layoutAmendment(gtx, view)
	}
	if r.repeatAPI != nil && d.record != nil && d.err == nil && d.pending == nil && d.record.Repeatable && d.repeat.Clicked(gtx) {
		r.startRepeat()
		return p.layoutRepeat(gtx, view)
	}
	if d.record != nil && d.err == nil && d.pending == nil && d.record.Amendable {
		if d.correct.Clicked(gtx) {
			r.startAmendment(history.CorrectCash)
		} else if d.record.Reversible && d.reverse.Clicked(gtx) {
			r.startAmendment(history.ReverseCash)
		}
		if d.amendment != nil {
			return p.layoutAmendment(gtx, view)
		}
	}
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
		if known.LiabilityAccount != "" {
			title = liabilityKindLabel(known.Activity.Kind, known.Activity.Reason)
		}
		r.rename(d.id, l.Text(title)+" · "+historyDay(view, known.Activity.EffectiveLocalDate))
	} else if d.id == r.posted.ID {
		title = historyKindLabel(r.posted.Kind, r.posted.Reason)
	}
	backLabel := i18n.ContributionBack
	if r.backToAccount != nil && r.backToAccount() {
		backLabel = i18n.LiabilityBackToAccount
	}

	children := []ui.ColumnChild{
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(10)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					view.Focus.Watch(gtx, &d.back)
					return view.Button(&d.back, l.Text(backLabel))(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Label(gtx, l.Text(title), 19, font.Medium, view.Colors.Ink)
				}),
			)
		}),
	}
	if d.id == r.posted.ID {
		if r.postedNotice != i18n.NoMessage {
			children = append(children, ui.Rigid(view.Message(l.Text(r.postedNotice), view.Colors.Ink)))
		} else if movement, supported := history.CashMovementOf(r.posted.Kind, r.posted.Reason); supported {
			children = append(children, ui.Rigid(view.Message(l.Text(cashMovementLabels[movement].posted), view.Colors.Ink)))
		}
	}
	switch {
	case d.pending != nil:
		children = append(children, ui.Rigid(view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)))
	case errors.Is(d.err, history.ErrActivityUnavailable):
		children = append(children, ui.Rigid(view.Message(l.Text(i18n.HistoryActivityUnavailable), view.Colors.SecondaryInk)))
	case d.err != nil:
		children = append(children,
			ui.Rigid(view.Message(l.Text(i18n.ContributionDetailFailed), view.Colors.ErrorInk)),
			ui.Rigid(view.Button(&d.retry, l.Text(i18n.HistoryRetry))),
		)
	case known != nil:
		if r.repeatAPI != nil && known.Repeatable {
			children = append(children, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.W.Layout(gtx, view.Button(&d.repeat, l.Text(i18n.HistoryRepeatAction)))
			}))
		}
		if known.Amendable {
			buttons := []layout.FlexChild{layout.Rigid(view.Button(&d.correct, l.Text(i18n.HistoryCorrectAction)))}
			if known.Reversible {
				buttons = append(buttons, layout.Rigid(view.Button(&d.reverse, l.Text(i18n.HistoryReverseAction))))
			}
			children = append(children, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
			}))
			if !known.Reversible {
				children = append(children, ui.Rigid(view.Message(l.Text(i18n.HistoryReversalArchived), view.Colors.SecondaryInk)))
			}
		}
		if known.Links != (history.AmendmentLinks{}) {
			children = append(children, ui.Rigid(r.amendmentLinks(view)))
		}
		facts := append(r.accountLinks(view, *known), r.instrumentLinks(view, *known)...)
		facts = append(facts, activityFacts(view, *known)...)
		children = append(children, ui.Fill(func(gtx layout.Context) layout.Dimensions {
			return view.List(&d.scroll).Layout(gtx, len(facts), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Bottom: 8}.Layout(gtx, facts[i])
			})
		}))
	}
	gtx, view = view.Measure(gtx, 720)
	return ui.Column{Gap: 9}.Layout(gtx, children...)
}

func (r *historyReads) instrumentLinks(view ui.View, detail history.Detail) []layout.Widget {
	activity := detail.Activity
	if product := activity.ProductContext; product != nil && product.ProductKind == domain.ProductTermDeposit && r.openProduct != nil {
		if name := detail.Instruments[product.InstrumentID]; name != "" {
			button := portfolioButton(r.detail.instruments, product.InstrumentID)
			return []layout.Widget{productLink(view, product.ProductID, name, button, r.openProduct)}
		}
	}
	if r.openInstrument == nil {
		return nil
	}
	var ids []domain.InstrumentID
	if activity.TradeDetail != nil {
		ids = append(ids, activity.TradeDetail.InstrumentID)
	}
	if activity.DividendDetail != nil {
		ids = append(ids, activity.DividendDetail.InstrumentID)
	}
	for _, effect := range activity.Effects {
		if effect.InstrumentID != nil {
			ids = append(ids, *effect.InstrumentID)
		}
	}
	var links []layout.Widget
	seen := make(map[domain.InstrumentID]bool)
	for _, id := range ids {
		name := detail.Instruments[id]
		if name == "" || seen[id] {
			continue
		}
		seen[id] = true
		button := portfolioButton(r.detail.instruments, id)
		label := fmt.Sprintf(view.Language.Text(i18n.PortfolioOpenInstrument), name)
		links = append(links, func(gtx layout.Context) layout.Dimensions {
			if button.Clicked(gtx) {
				r.openInstrument(id, name, button)
				gtx.Execute(op.InvalidateCmd{})
			}
			return layout.W.Layout(gtx, view.Button(button, label))
		})
	}
	return links
}

func (r *historyReads) accountLinks(view ui.View, detail history.Detail) []layout.Widget {
	if r.openAccount == nil {
		return nil
	}
	var links []layout.Widget
	seen := make(map[domain.AccountID]bool)
	for _, effect := range detail.Activity.Effects {
		if effect.AccountID == nil {
			continue
		}
		id := *effect.AccountID
		name := detail.Accounts[id]
		if name == "" || seen[id] {
			continue
		}
		seen[id] = true
		button := r.detail.accountButton(id)
		label := fmt.Sprintf(view.Language.Text(i18n.HistoryOpenAccount), name)
		links = append(links, func(gtx layout.Context) layout.Dimensions {
			if button.Clicked(gtx) {
				r.openAccount(id, name, button)
				gtx.Execute(op.InvalidateCmd{})
			}
			return layout.W.Layout(gtx, view.Button(button, label))
		})
	}
	return links
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
	debt := activity.Kind == domain.ActivityDebtDraw || activity.Kind == domain.ActivityDebtPayment
	liabilityChange := detail.LiabilityAccount != "" && (activity.Kind == domain.ActivityCashIn ||
		activity.Kind == domain.ActivityCashOut || activity.Kind == domain.ActivityValueUpdate)
	var debtAccount domain.AccountID
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
	timestamp := contributionTimestamp
	if activity.Kind == domain.ActivityBuy || activity.Kind == domain.ActivitySell {
		timestamp = tradeTimestamp
	}
	var facts []layout.Widget
	if detail.Location != nil {
		facts = append(facts, muted(timestamp(view, activity.EffectiveAt, detail.Location)))
	} else {
		facts = append(facts, muted(historyDay(view, activity.EffectiveLocalDate)), muted(timestamp(view, activity.EffectiveAt, time.UTC)))
	}
	if activity.Note != nil && *activity.Note != "" {
		facts = append(facts, ink(*activity.Note))
	}
	if liabilityChange && liabilityReasonLabels[activity.Reason] != i18n.NoMessage {
		facts = append(facts, ink(liabilityReasonFact(l, activity.Reason)))
	}
	if detail.Links.Reversal != "" {
		badge := i18n.HistoryOriginalReversed
		if detail.Links.Replacement != "" {
			badge = i18n.HistoryOriginalCorrected
		}
		facts = append(facts, muted(l.Text(badge)))
	}
	if activity.ReversesActivityID == nil && activity.CorrectionGroupID != nil {
		facts = append(facts, muted(l.Text(i18n.HistoryCorrectionRecord)))
	}
	if trade := activity.TradeDetail; trade != nil {
		facts = append(facts, ink(instrument(trade.InstrumentID)),
			muted(fmt.Sprintf(l.Text(i18n.HistoryUnitPrice), trade.UnitPrice.Canonical()+" "+trade.Gross.Currency().String())))
	} else if dividend := activity.DividendDetail; dividend != nil {
		facts = append(facts, ink(instrument(dividend.InstrumentID)))
	}
	for _, effect := range activity.Effects {
		var caption string
		switch {
		case (liabilityChange || effect.Role == domain.EffectRoleDebt) && effect.Target == domain.EffectTargetAccountValue && effect.Money != nil:
			label := i18n.HistoryDebtIncreased
			if effect.Direction == domain.EffectRemoved {
				label = i18n.HistoryDebtReduced
			}
			caption = fmt.Sprintf(l.Text(label), account(effect.AccountID), movementMoney(effect))
			if effect.AccountID != nil {
				debtAccount = *effect.AccountID
			}
		case debt && effect.Role == domain.EffectRoleFee && effect.Money != nil:
			caption = fmt.Sprintf(l.Text(i18n.TransferFeePaid), account(effect.AccountID), movementMoney(effect))
		default:
			caption = effectCaption(l, effect, paired, supported, movement, account, instrument)
		}
		class := l.Text(effectClasses[effect.Classification])
		if product := activity.ProductContext; product != nil && product.Purpose == domain.ProductPurposeInterest &&
			effect.Target == domain.EffectTargetAccountCash && effect.Role == domain.EffectRoleAmount {
			class = l.Text(i18n.HistoryIncome)
		}
		if activity.ReversesActivityID != nil && class != "" {
			class = fmt.Sprintf(l.Text(i18n.HistoryReversedEffect), class)
		}
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
	from, to := transferCurrencies(activity.Effects)
	if paired {
		if rate := transferRateLabel(view, activity.TransactionFXRate, from, to); rate != "" {
			facts = append(facts, ink(rate))
		}
	}
	namedBalances := paired || (activity.ReversesActivityID != nil && from != "" && to != "")
	type balanceRow struct {
		account  domain.AccountID
		target   domain.EffectTarget
		currency domain.CurrencyCode
		amount   string
	}
	shown := make(map[balanceRow]bool)
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
		if result.AccountID != nil {
			// Fee and principal effects can carry the same stored endpoint balance.
			row := balanceRow{*result.AccountID, result.Target, result.Currency, result.Amount}
			if shown[row] {
				continue
			}
			shown[row] = true
		}
		balance := result.Amount + " " + result.Currency.String()
		if debtAccount != "" && result.AccountID != nil && *result.AccountID == debtAccount && result.Target == domain.EffectTargetAccountValue {
			facts = append(facts, ink(fmt.Sprintf(l.Text(i18n.AccountCardBalanceOwedAfter), balance)))
		} else if namedBalances || debt {
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
