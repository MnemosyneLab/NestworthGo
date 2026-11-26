package desktop

import (
	"slices"
	"strings"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type historyRowNames struct {
	accounts        map[domain.AccountID]domain.Account
	instruments     map[domain.InstrumentID]string
	holdingAccounts map[domain.HoldingID]domain.AccountID
}

func newHistoryRowNames(refs history.References) historyRowNames {
	names := historyRowNames{
		accounts:    make(map[domain.AccountID]domain.Account, len(refs.Accounts)),
		instruments: make(map[domain.InstrumentID]string, len(refs.Instruments)),
	}
	for _, record := range refs.Accounts {
		names.accounts[record.Account.ID] = record.Account
	}
	for _, instrument := range refs.Instruments {
		names.instruments[instrument.ID] = instrument.DisplayLabel()
	}
	return names
}

func (n historyRowNames) account(id *domain.AccountID) domain.Account {
	if id != nil {
		return n.accounts[*id]
	}
	return domain.Account{}
}

func (n historyRowNames) endpoint(e domain.ActivityEffect) string {
	if e.AccountID != nil {
		return n.account(e.AccountID).Name
	}
	if e.HoldingID != nil {
		return n.accounts[n.holdingAccounts[*e.HoldingID]].Name
	}
	return ""
}

func historyRowSummary(view ui.View, activity domain.Activity, names historyRowNames) string {
	l := view.Language
	if activity.ReversesActivityID != nil {
		return l.Text(i18n.HistoryReversal)
	}

	var first domain.ActivityEffect
	byRole := make(map[domain.EffectRole]domain.ActivityEffect, len(activity.Effects))
	var instrument domain.InstrumentID
	for i, effect := range activity.Effects {
		if i == 0 || first.Money == nil && effect.Money != nil {
			first = effect
		}
		if _, seen := byRole[effect.Role]; !seen {
			byRole[effect.Role] = effect
		}
		if instrument == "" && effect.InstrumentID != nil {
			instrument = *effect.InstrumentID
		}
	}

	preference := settings.Settings{DecimalSeparator: view.DecimalSeparator, GroupingSeparator: view.GroupingSeparator}
	money := func(amount *domain.Money) string {
		if amount == nil {
			return ""
		}
		return format.AuditMoney(amount.CanonicalAmount(), amount.Currency().String(), preference)
	}
	label := historyKindLabel(activity.Kind, activity.Reason)
	account := names.account(first.AccountID)
	if account.IsLiability() {
		label = liabilityKindLabel(activity.Kind, activity.Reason)
	}
	amount, accountName := money(first.Money), names.endpoint(first)
	if detail := activity.DividendDetail; detail != nil {
		instrument = detail.InstrumentID
	}
	var quantity, fee string

	switch activity.Kind {
	case domain.ActivityCashTransfer, domain.ActivityFXConversion:
		from, to := byRole[domain.EffectRoleTransferFrom], byRole[domain.EffectRoleTransferTo]
		if from.Money != nil && to.Money != nil {
			fromAmount, toAmount := money(from.Money), money(to.Money)
			amount = fromAmount
			if from.Money.Currency() != to.Money.Currency() || !from.Money.Amount().Equal(to.Money.Amount()) {
				amount = historyRowRoute(fromAmount, toAmount)
			}
			accountName = historyRowRoute(names.account(from.AccountID).Name, names.account(to.AccountID).Name)
		}
		fee = money(byRole[domain.EffectRoleFee].Money)
	case domain.ActivityBuy, domain.ActivitySell:
		if principal := byRole[domain.EffectRolePrincipal]; principal.Money != nil {
			amount, accountName = money(principal.Money), names.account(principal.AccountID).Name
		}
		if detail := activity.TradeDetail; detail != nil {
			instrument, amount = detail.InstrumentID, money(&detail.Gross)
			quantity, fee = detail.Quantity.Canonical(), money(detail.Fee)
		}
	case domain.ActivityCashDividend:
		if detail := activity.DividendDetail; detail != nil {
			instrument, amount = detail.InstrumentID, money(&detail.Amount)
		}
	case domain.ActivityDebtDraw, domain.ActivityDebtPayment:
		if debt := byRole[domain.EffectRoleDebt]; debt.Money != nil {
			amount, accountName = money(debt.Money), names.account(debt.AccountID).Name
		}
		fee = money(byRole[domain.EffectRoleFee].Money)
	case domain.ActivityValueUpdate:
		if len(activity.Resulting) != 0 {
			result := activity.Resulting[0]
			if result.Amount != "" && result.Currency != "" {
				resultAccount := names.account(result.AccountID)
				ending := result.Amount
				if resultAccount.IsLiability() && ending != "0" && !strings.HasPrefix(ending, "-") {
					ending = "-" + ending
				}
				amount = format.AuditMoney(ending, result.Currency.String(), preference)
				if resultAccount.Name != "" {
					accountName = resultAccount.Name
				}
			}
		}
	case domain.ActivityCostAdjustment:
		if first.CostUnitPrice != nil {
			amount = portfolioDecimal(view, first.CostUnitPrice.Canonical())
		}
	case domain.ActivityPositionTransfer:
		from, to := byRole[domain.EffectRoleTransferFrom], byRole[domain.EffectRoleTransferTo]
		if from.HoldingID != nil && to.HoldingID != nil {
			accountName = historyRowRoute(names.endpoint(from), names.endpoint(to))
		}
		if activity.Reason == domain.ReasonReconciliation && activity.ProductContext == nil {
			label = i18n.HistoryPositionAdjustment
		}
		for _, role := range []domain.EffectRole{domain.EffectRoleTransferFrom, domain.EffectRoleTransferTo, domain.EffectRoleQuantity} {
			if movement := byRole[role]; movement.Quantity != nil {
				quantity = movement.Quantity.Canonical()
				break
			}
		}
	}
	if product := activity.ProductContext; product != nil {
		instrument = product.InstrumentID
		switch product.Purpose {
		case domain.ProductPurposeAcquisition:
			label = i18n.ProductHistoryOpened
		case domain.ProductPurposeExistingPosition:
			label = i18n.ProductHistoryRecorded
			// An existing-position record has no cash leg to summarize.
			amount, quantity = "", ""
		case domain.ProductPurposeRedemption:
			label = i18n.ProductHistorySettled
		case domain.ProductPurposeInterest:
			label = i18n.ProductHistoryInterest
		case domain.ProductPurposeReversal:
			label = i18n.HistoryReversal
		}
	}
	instrumentName := names.instruments[instrument]
	summary := []string{l.Text(label), amount, instrumentName, accountName, portfolioDecimal(view, quantity)}
	if activity.Kind == domain.ActivityCashIn || activity.Kind == domain.ActivityCashOut {
		reason := repeatReasonLabels[activity.Reason]
		if activity.Reason != domain.ReasonOther && reason != i18n.NoMessage && reason != label {
			summary = append(summary, l.Text(reason))
		}
	}
	if fee != "" {
		summary = append(summary, l.Text(i18n.HistoryFee)+" "+fee)
	}
	if activity.CorrectionGroupID != nil {
		summary = append(summary, l.Text(i18n.HistoryCorrectionRecord))
	}
	return strings.Join(slices.DeleteFunc(summary, func(value string) bool { return value == "" }), " · ")
}

func historyRowRoute(from, to string) string {
	if from == "" || from == to {
		return to
	}
	if to == "" {
		return from
	}
	return from + " → " + to
}
