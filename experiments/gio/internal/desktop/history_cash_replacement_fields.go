package desktop

import (
	"image"
	"slices"

	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func cashReplacementFields(view ui.View, form *historyAmendmentDraft, origin *image.Point, busy bool) []layout.Widget {
	draft := form.investment
	command := &draft.command
	accounts := draft.sources.Accounts
	selected := cashReplacementAccount(accounts, command.AccountID)
	otherAmount := &draft.values[0]

	picker := func(index int, label i18n.MessageID, choices []ui.Choice, value string, searchable bool, change func(string)) layout.Widget {
		return investmentReplacementPicker(view, form, origin, &draft.selectors[index],
			ui.PickerField{Label: label, Disabled: busy}, choices, value, searchable, change)
	}
	accountPicker := func(index int, label i18n.MessageID, value *string, allowed func(domain.Account) bool, changed func(domain.Account)) layout.Widget {
		var choices []ui.Choice
		for _, account := range accounts {
			if (account.ArchivedAt != nil && account.ID != draft.Account) || (allowed != nil && !allowed(account)) {
				continue
			}
			name := account.Name
			if account.TrackingMode != domain.TrackingHoldings {
				name += " · " + account.DefaultCurrency.String()
			}
			choices = append(choices, ui.Choice{ID: account.ID.String(), Label: name, SearchText: account.Name})
		}
		return picker(index, label, choices, *value, true, func(id string) {
			*value = id
			changed(cashReplacementAccount(accounts, id))
		})
	}
	money := func(field *ui.TextField, label i18n.MessageID, currency string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			if field == &form.amount {
				focusInvestmentReplacementInput(gtx, form, field, busy)
			}
			options := ui.FieldOptions{Label: label, Disabled: busy}
			if currency == "" {
				return field.Layout(gtx, view, options)
			}
			return field.LayoutCurrency(gtx, view, options, currency)
		}
	}
	varCurrencyMoney := func(index int, field *ui.TextField, label, currencyLabel i18n.MessageID, currency *string, exclude string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			if busy {
				gtx = gtx.Disabled()
			}
			if field == &form.amount {
				focusInvestmentReplacementInput(gtx, form, field, busy)
			}
			return layoutCashCurrencyAmount(gtx, view, *origin, field, label, &draft.selectors[index], currencyLabel,
				domain.CurrencyCode(*currency), domain.CurrencyCode(exclude), func(value domain.CurrencyCode) {
					if *currency != value.String() {
						*currency, form.problem = value.String(), i18n.NoMessage
					}
				})
		}
	}
	reasonPicker := func() layout.Widget {
		reasons, reason := cashReplacementReasons(command, selected)
		choices := make([]ui.Choice, 0, len(reasons))
		for _, reason := range reasons {
			choices = append(choices, ui.Choice{ID: string(reason), Label: view.Language.Text(checkpointCashReasonLabel(reason))})
		}
		return picker(5, i18n.HistoryRepeatReason, choices, reason, false, func(value string) { command.Reason = value })
	}
	cashAccount := func(account domain.Account) bool {
		return !account.IsLiability() && (account.TrackingMode == domain.TrackingBalance || account.TrackingMode == domain.TrackingHoldings)
	}
	var fields []layout.Widget
	switch command.Kind {
	case historyapi.ChangeMoneyAdded, historyapi.ChangeMoneyRemoved:
		amount := money(&form.amount, i18n.HistoryRepeatAmount, command.Currency)
		if selected.TrackingMode == domain.TrackingHoldings {
			amount = varCurrencyMoney(3, &form.amount, i18n.HistoryRepeatAmount, i18n.CashCurrency, &command.Currency, "")
		}
		fields = []layout.Widget{
			accountPicker(1, i18n.HistoryRepeatAccount, &command.AccountID, nil, func(account domain.Account) {
				command.Currency, command.Reason = account.DefaultCurrency.String(), ""
			}),
			amount,
			reasonPicker(),
		}
	case historyapi.ChangeCashTransfer:
		transferAccount := func(account domain.Account) bool { return !account.IsLiability() }
		fields = []layout.Widget{
			accountPicker(1, i18n.TransferFrom, &command.FromAccountID, transferAccount, func(account domain.Account) {
				command.SentCurrency = account.DefaultCurrency.String()
				if command.ToAccountID == account.ID.String() {
					command.ToAccountID, command.ReceivedCurrency = "", ""
				}
			}),
			accountPicker(2, i18n.TransferTo, &command.ToAccountID, func(account domain.Account) bool {
				return transferAccount(account) && account.ID.String() != command.FromAccountID
			}, func(account domain.Account) { command.ReceivedCurrency = account.DefaultCurrency.String() }),
			money(&form.amount, i18n.TransferSentAmount, command.SentCurrency),
		}
		if command.SentCurrency != command.ReceivedCurrency {
			fields = append(fields, money(otherAmount, i18n.TransferReceivedAmount, command.ReceivedCurrency))
		}
		fields = append(fields, money(&form.fee, i18n.TransferFeeInput, command.SentCurrency))
	case historyapi.ChangeFXConversion:
		fields = []layout.Widget{
			accountPicker(1, i18n.ConversionAccount, &command.AccountID, func(account domain.Account) bool {
				return account.TrackingMode == domain.TrackingHoldings
			}, func(account domain.Account) {
				command.SoldCurrency = account.DefaultCurrency.String()
				if command.BoughtCurrency == command.SoldCurrency {
					command.BoughtCurrency = ""
				}
			}),
			varCurrencyMoney(3, &form.amount, i18n.ConversionSoldAmount, i18n.ConversionSoldCurrency, &command.SoldCurrency, command.BoughtCurrency),
			varCurrencyMoney(4, otherAmount, i18n.ConversionBoughtAmount, i18n.ConversionBoughtCurrency, &command.BoughtCurrency, command.SoldCurrency),
			money(&form.fee, i18n.TransferFeeInput, command.SoldCurrency),
		}
	case historyapi.ChangeValueUpdate:
		label := i18n.AccountValueTarget
		if selected.IsLiability() {
			label = i18n.LiabilityBalanceCorrection
		}
		fields = []layout.Widget{
			accountPicker(1, i18n.HistoryRepeatAccount, &command.AccountID, func(account domain.Account) bool {
				return account.TrackingMode == domain.TrackingBalance || account.TrackingMode == domain.TrackingManualValue
			}, func(account domain.Account) {
				command.NewValueCurrency, command.Reason = account.DefaultCurrency.String(), ""
			}),
			money(&form.amount, label, command.NewValueCurrency),
			reasonPicker(),
		}
	case historyapi.ChangeDebtDraw, historyapi.ChangeDebtPayment:
		fields = []layout.Widget{
			accountPicker(1, i18n.LiabilityChooseDebt, &command.DebtAccountID, func(account domain.Account) bool {
				return account.IsLiability() && account.TrackingMode == domain.TrackingBalance
			}, func(domain.Account) {}),
			accountPicker(2, i18n.LiabilityChooseCash, &command.CashAccountID, cashAccount, func(account domain.Account) {
				command.PrincipalCurrency = account.DefaultCurrency.String()
			}),
			money(&form.amount, i18n.HistoryDebtPrincipal, command.PrincipalCurrency),
		}
		if command.Kind == historyapi.ChangeDebtPayment {
			fields = append(fields, money(&form.fee, i18n.LiabilityInterest, command.PrincipalCurrency))
		}
	}
	return fields
}

func cashReplacementAmounts(form *historyAmendmentDraft, request *historyapi.ChangeCommandRequest) {
	amount, fee := form.amount.Text(), form.fee.Text()
	otherAmount := form.investment.values[0].Text()
	switch request.Kind {
	case historyapi.ChangeMoneyAdded, historyapi.ChangeMoneyRemoved:
		request.Amount = amount
	case historyapi.ChangeCashTransfer:
		request.Sent, request.Received = amount, otherAmount
		if request.SentCurrency == request.ReceivedCurrency {
			request.Received = amount
		}
		request.Fee, request.FeeCurrency = fee, request.SentCurrency
	case historyapi.ChangeFXConversion:
		request.Sold, request.Bought = amount, otherAmount
		request.Fee, request.FeeCurrency = fee, request.SoldCurrency
	case historyapi.ChangeValueUpdate:
		request.NewValue = amount
	case historyapi.ChangeDebtDraw, historyapi.ChangeDebtPayment:
		request.Principal = amount
		if request.Kind == historyapi.ChangeDebtPayment {
			request.InterestOrFee, request.InterestOrFeeCurrency = fee, request.PrincipalCurrency
		}
	}
	if request.Kind == historyapi.ChangeMoneyAdded || request.Kind == historyapi.ChangeMoneyRemoved || request.Kind == historyapi.ChangeValueUpdate {
		account := cashReplacementAccount(form.investment.sources.Accounts, request.AccountID)
		_, request.Reason = cashReplacementReasons(request, account)
	}
}

func cashReplacementAccount(accounts []domain.Account, id string) domain.Account {
	index := slices.IndexFunc(accounts, func(account domain.Account) bool { return account.ID.String() == id })
	if index < 0 {
		return domain.Account{}
	}
	return accounts[index]
}

func cashReplacementReasons(request *historyapi.ChangeCommandRequest, account domain.Account) ([]domain.ActivityReason, string) {
	reasons, fallback := domain.MoneyInReasons(), domain.ReasonOther
	switch request.Kind {
	case historyapi.ChangeMoneyAdded:
		if account.IsLiability() {
			reasons = domain.LiabilityIncreaseReasons()
		}
	case historyapi.ChangeMoneyRemoved:
		reasons = domain.MoneyOutReasons()
		if account.IsLiability() {
			reasons = domain.LiabilityDecreaseReasons()
		}
	case historyapi.ChangeValueUpdate:
		reasons, fallback = domain.ValueUpdateReasons(), domain.ReasonReconciliation
		if account.IsLiability() {
			reasons = []domain.ActivityReason{fallback}
		}
	}
	if reason := domain.ActivityReason(request.Reason); slices.Contains(reasons, reason) {
		return reasons, request.Reason
	}
	return reasons, string(fallback)
}
