package desktop

import (
	"image"
	"slices"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func investmentReplacementFields(view ui.View, form *historyAmendmentDraft, origin *image.Point, busy bool) []layout.Widget {
	switch form.investment.command.Kind {
	case historyapi.ChangeTrade:
		return investmentReplacementTradeFields(view, form, origin, busy)
	case historyapi.ChangeCashDividend:
		return investmentReplacementDividendFields(view, form, origin, busy)
	case historyapi.ChangePositionTransfer:
		return investmentReplacementTransferFields(view, form, origin, busy)
	case historyapi.ChangePositionAdjustment:
		return investmentReplacementAdjustmentFields(view, form, origin, busy)
	default:
		return nil
	}
}

func investmentReplacementAmounts(form *historyAmendmentDraft, request *historyapi.ChangeCommandRequest) {
	draft := form.investment
	switch request.Kind {
	case historyapi.ChangeTrade:
		request.Quantity = strings.TrimSpace(draft.quantity.Text())
		if draft.sources.OriginalTradeRestricted {
			request.Quantity = draft.Quantity
		}
		request.Gross, request.Fee = strings.TrimSpace(form.amount.Text()), strings.TrimSpace(form.fee.Text())
		request.FeeCurrency = request.GrossCurrency
	case historyapi.ChangeCashDividend:
		request.Amount = strings.TrimSpace(form.amount.Text())
	case historyapi.ChangePositionTransfer:
		request.Quantity = strings.TrimSpace(draft.quantity.Text())
	case historyapi.ChangePositionAdjustment:
		request.Quantity = strings.TrimSpace(draft.quantity.Text())
		request.UnitCost = ""
		if request.Added {
			request.UnitCost = strings.TrimSpace(draft.secondaryAmount.Text())
		}
	}
}

func investmentReplacementTradeFields(view ui.View, form *historyAmendmentDraft, origin *image.Point, busy bool) []layout.Widget {
	draft, l := form.investment, view.Language
	request := &draft.command
	restricted := draft.sources.OriginalTradeRestricted
	quantityLabel, grossLabel, sideLabel := i18n.BuyQuantity, i18n.BuyGross, i18n.HistoryBuy
	if request.Side == string(domain.TradeSell) {
		quantityLabel, grossLabel, sideLabel = i18n.SellQuantity, i18n.SellGross, i18n.HistorySell
	}
	var rows []layout.Widget
	if restricted {
		rows = append(rows, view.Message(l.Text(sideLabel), view.Colors.Ink))
	} else {
		rows = investmentReplacementTradeSelectors(view, form, origin, busy)
	}
	ready := restricted || request.HoldingID != "" && request.GrossCurrency != ""
	return append(rows,
		func(gtx layout.Context) layout.Dimensions {
			if !restricted && ready {
				focusInvestmentReplacementInput(gtx, form, &draft.quantity, busy)
			}
			return draft.quantity.Layout(gtx, view, ui.FieldOptions{Label: quantityLabel, Disabled: busy || !ready, ReadOnly: restricted})
		},
		func(gtx layout.Context) layout.Dimensions {
			if restricted {
				focusInvestmentReplacementInput(gtx, form, &form.amount, busy)
			}
			return form.amount.LayoutCurrency(gtx, view, ui.FieldOptions{Label: grossLabel, Disabled: busy || !ready}, request.GrossCurrency)
		},
		func(gtx layout.Context) layout.Dimensions {
			return form.fee.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.BuyFee, Disabled: busy || !ready}, request.GrossCurrency)
		},
	)
}

func investmentReplacementTradeSelectors(view ui.View, form *historyAmendmentDraft, origin *image.Point, busy bool) []layout.Widget {
	draft, l := form.investment, view.Language
	request := &draft.command
	accountPicker, sidePicker, holdingPicker := &draft.selectors[0], &draft.selectors[1], &draft.selectors[2]
	sides := []ui.Choice{
		{ID: string(domain.TradeBuy), Label: l.Text(i18n.HistoryBuy)},
		{ID: string(domain.TradeSell), Label: l.Text(i18n.HistorySell)},
	}
	holdingAccounts := make(map[domain.AccountID]bool)
	var holdings []ui.Choice
	for _, item := range draft.sources.Holdings {
		holdingAccounts[item.Account.ID] = true
		if item.Account.ID.String() != request.SettlementAccountID {
			continue
		}
		label := item.Instrument.SelectionLabel()
		if request.Side == string(domain.TradeSell) {
			label = custodyHoldingLabel(view, item)
		}
		holdings = append(holdings, ui.Choice{ID: item.Holding.ID.String(), Label: label, SearchText: item.Instrument.Name})
	}
	accounts := make([]ui.Choice, 0, len(holdingAccounts))
	for _, account := range draft.sources.Accounts {
		if holdingAccounts[account.ID] {
			accounts = append(accounts, ui.Choice{ID: account.ID.String(), Label: account.Name + " · " + account.DefaultCurrency.String()})
		}
	}
	holdingLabel := i18n.BuyInstrument
	if request.Side == string(domain.TradeSell) {
		holdingLabel = i18n.QuantityAdjustHolding
	}
	selectHolding := func(id string) {
		item := investmentReplacementHolding(draft.sources.Holdings, id)
		request.HoldingID, request.InstrumentID = item.Holding.ID.String(), item.Instrument.ID.String()
		request.GrossCurrency = item.Instrument.QuoteCurrency.String()
		request.FeeCurrency = request.GrossCurrency
	}
	clearAmounts := func() {
		draft.quantity.Editor.SetText("")
		form.amount.Editor.SetText("")
		form.fee.Editor.SetText("")
	}
	return []layout.Widget{
		investmentReplacementPicker(view, form, origin, accountPicker, ui.PickerField{
			Label: i18n.PositionImportAccount, Next: &sidePicker.Trigger, Disabled: busy,
		}, accounts, request.SettlementAccountID, true, func(id string) {
			instrument, currency := request.InstrumentID, request.GrossCurrency
			request.SettlementAccountID = id
			if request.Side == string(domain.TradeBuy) {
				for _, item := range draft.sources.Holdings {
					if item.Account.ID.String() == id && item.Instrument.ID.String() == instrument {
						selectHolding(item.Holding.ID.String())
						if request.GrossCurrency != currency {
							clearAmounts()
						}
						return
					}
				}
			}
			selectHolding("")
			clearAmounts()
		}),
		investmentReplacementPicker(view, form, origin, sidePicker, ui.PickerField{
			Label: i18n.HistoryReplacementTradeSide, Previous: &accountPicker.Trigger, Next: &holdingPicker.Trigger, Disabled: busy,
		}, sides, request.Side, false, func(id string) {
			request.Side = id
			selectHolding("")
			clearAmounts()
		}),
		investmentReplacementPicker(view, form, origin, holdingPicker, ui.PickerField{
			Label: holdingLabel, Previous: &sidePicker.Trigger, Next: &draft.quantity.Editor, Disabled: busy,
		}, holdings, request.HoldingID, true, func(id string) {
			selectHolding(id)
			clearAmounts()
		}),
	}
}

func investmentReplacementDividendFields(view ui.View, form *historyAmendmentDraft, origin *image.Point, busy bool) []layout.Widget {
	draft := form.investment
	request := &draft.command
	holdings := investmentReplacementHoldingChoices(view, draft.sources.Holdings)
	chooseCurrency := func(currency domain.CurrencyCode) {
		if currency.String() != request.Currency {
			request.Currency, form.problem = currency.String(), i18n.NoMessage
			form.amount.Editor.SetText("")
		}
	}
	return []layout.Widget{
		investmentReplacementPicker(view, form, origin, &draft.selectors[2], ui.PickerField{
			Label: i18n.QuantityAdjustHolding, Next: &form.amount.Editor, Disabled: busy,
		}, holdings, request.HoldingID, true, func(id string) {
			request.HoldingID = id
			chooseCurrency(investmentReplacementHolding(draft.sources.Holdings, id).Instrument.QuoteCurrency)
		}),
		func(gtx layout.Context) layout.Dimensions {
			if busy || request.HoldingID == "" {
				gtx = gtx.Disabled()
			}
			focusInvestmentReplacementInput(gtx, form, &form.amount, busy)
			return layoutCashCurrencyAmount(gtx, view, *origin, &form.amount, i18n.DividendAmount, &draft.selectors[3],
				i18n.CashCurrency, domain.CurrencyCode(request.Currency), "", chooseCurrency)
		},
	}
}

func investmentReplacementTransferFields(view ui.View, form *historyAmendmentDraft, origin *image.Point, busy bool) []layout.Widget {
	draft := form.investment
	request := &draft.command
	sourcePicker, targetPicker := &draft.selectors[4], &draft.selectors[5]
	holdings := draft.sources.Holdings
	source := investmentReplacementHolding(holdings, request.FromHoldingID)
	var targets []ui.Choice
	if source.Holding.ID != "" {
		for _, item := range holdings {
			if item.Instrument.ID == source.Instrument.ID && item.Account.ID != source.Account.ID {
				targets = append(targets, ui.Choice{ID: item.Holding.ID.String(), Label: item.Account.Name})
			}
		}
	}
	return []layout.Widget{
		investmentReplacementPicker(view, form, origin, sourcePicker, ui.PickerField{
			Label: i18n.CustodyFromHolding, Next: &targetPicker.Trigger, Disabled: busy,
		}, investmentReplacementHoldingChoices(view, holdings), request.FromHoldingID, true, func(id string) {
			request.FromHoldingID, request.ToHoldingID, request.ToAccountID = id, "", ""
			draft.quantity.Editor.SetText("")
		}),
		investmentReplacementPicker(view, form, origin, targetPicker, ui.PickerField{
			Label: i18n.CustodyToAccount, Previous: &sourcePicker.Trigger, Next: &draft.quantity.Editor, Disabled: busy,
		}, targets, request.ToHoldingID, true, func(id string) {
			request.ToHoldingID, request.ToAccountID = id, ""
		}),
		func(gtx layout.Context) layout.Dimensions {
			return draft.quantity.Layout(gtx, view, ui.FieldOptions{
				Label: i18n.CustodyQuantity, Disabled: busy || request.FromHoldingID == "" || request.ToHoldingID == "",
			})
		},
	}
}

func investmentReplacementAdjustmentFields(view ui.View, form *historyAmendmentDraft, origin *image.Point, busy bool) []layout.Widget {
	draft, l := form.investment, view.Language
	request := &draft.command
	holdingPicker, add, remove := &draft.selectors[2], &draft.adjustmentDirection[0], &draft.adjustmentDirection[1]
	item := investmentReplacementHolding(draft.sources.Holdings, request.HoldingID)
	ready := item.Holding.ID != ""
	rows := []layout.Widget{
		investmentReplacementPicker(view, form, origin, holdingPicker, ui.PickerField{
			Label: i18n.QuantityAdjustHolding, Next: add, Disabled: busy,
		}, investmentReplacementHoldingChoices(view, draft.sources.Holdings), request.HoldingID, true, func(id string) {
			request.HoldingID = id
			draft.quantity.Editor.SetText("")
			draft.secondaryAmount.Editor.SetText("")
		}),
		func(gtx layout.Context) layout.Dimensions {
			if busy {
				gtx = gtx.Disabled()
			}
			if add.Clicked(gtx) && !request.Added {
				request.Added, form.problem = true, i18n.NoMessage
				gtx.Execute(op.InvalidateCmd{})
			}
			if remove.Clicked(gtx) && request.Added {
				request.Added, form.problem = false, i18n.NoMessage
				draft.secondaryAmount.Editor.SetText("")
				gtx.Execute(op.InvalidateCmd{})
			}
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(view.TabButton(add, l.Text(i18n.QuantityAdjustAdd), request.Added)),
				layout.Rigid(view.TabButton(remove, l.Text(i18n.QuantityAdjustRemove), !request.Added)),
			)
		},
		func(gtx layout.Context) layout.Dimensions {
			return draft.quantity.Layout(gtx, view, ui.FieldOptions{Label: i18n.QuantityAdjustQuantity, Disabled: busy || !ready})
		},
	}
	if request.Added && ready {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return draft.secondaryAmount.LayoutCurrency(gtx, view, ui.FieldOptions{
				Label: i18n.HistoryReplacementOptionalUnitCost, Disabled: busy,
			}, item.Instrument.QuoteCurrency.String())
		})
	}
	return rows
}

func investmentReplacementPicker(view ui.View, form *historyAmendmentDraft, origin *image.Point, picker *ui.Select, field ui.PickerField, choices []ui.Choice, selected string, searchable bool, choose func(string)) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		field.Origin = *origin
		field.Disabled = field.Disabled || len(choices) == 0
		picker.Layout(gtx, view, field, ui.SelectOptions{Items: choices, Searchable: searchable}, selected, func(id string) {
			if id != selected {
				choose(id)
				form.problem = i18n.NoMessage
			}
		})
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
	}
}

func investmentReplacementHolding(holdings []history.PositionHolding, id string) history.PositionHolding {
	index := slices.IndexFunc(holdings, func(item history.PositionHolding) bool { return item.Holding.ID.String() == id })
	if index < 0 {
		return history.PositionHolding{}
	}
	return holdings[index]
}

func investmentReplacementHoldingChoices(view ui.View, holdings []history.PositionHolding) []ui.Choice {
	choices := make([]ui.Choice, 0, len(holdings))
	for _, item := range holdings {
		choices = append(choices, ui.Choice{ID: item.Holding.ID.String(), Label: custodyHoldingLabel(view, item), SearchText: item.Instrument.Name})
	}
	return choices
}

func focusInvestmentReplacementInput(gtx layout.Context, form *historyAmendmentDraft, field *ui.TextField, busy bool) {
	if form.focus && !busy && gtx.Enabled() {
		field.Focus(gtx)
		form.focus = false
	}
}
