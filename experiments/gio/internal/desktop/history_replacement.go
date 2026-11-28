package desktop

import (
	"image"
	"slices"
	"strings"

	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

var replacementKinds = [...]struct {
	kind  historyapi.ChangeCommandKind
	label i18n.MessageID
}{
	{historyapi.ChangeMoneyAdded, i18n.HistoryCashIn},
	{historyapi.ChangeMoneyRemoved, i18n.HistoryCashOut},
	{historyapi.ChangeCashTransfer, i18n.HistoryCashTransfer},
	{historyapi.ChangeFXConversion, i18n.HistoryFXConversion},
	{historyapi.ChangeValueUpdate, i18n.HistoryValueUpdate},
	{historyapi.ChangeTrade, i18n.HistoryReplacementTrade},
	{historyapi.ChangeCashDividend, i18n.HistoryCashDividend},
	{historyapi.ChangePositionTransfer, i18n.HistoryPositionTransfer},
	{historyapi.ChangePositionAdjustment, i18n.AccountRecordPositionAdjustment},
	{historyapi.ChangeDebtDraw, i18n.HistoryDebtDraw},
	{historyapi.ChangeDebtPayment, i18n.HistoryDebtPayment},
}

func isPositionReplacement(kind historyapi.ChangeCommandKind) bool {
	switch kind {
	case historyapi.ChangeTrade, historyapi.ChangeCashDividend, historyapi.ChangePositionTransfer, historyapi.ChangePositionAdjustment:
		return true
	default:
		return false
	}
}

func investmentReplacementRequest(form *historyAmendmentDraft) (historyapi.ChangeCommandRequest, bool) {
	investment := form.investment
	request := investment.command
	if investment.loading != nil || investment.readFailed {
		return request, false
	}
	if isPositionReplacement(request.Kind) {
		investmentReplacementAmounts(form, &request)
	} else {
		cashReplacementAmounts(form, &request)
	}
	if note := strings.TrimSpace(investment.note.Text()); note != "" {
		request.Note = &note
	} else {
		request.Note = nil
	}
	hasFields := func(fields ...string) bool {
		return !slices.ContainsFunc(fields, func(field string) bool { return strings.TrimSpace(field) == "" })
	}
	hasHolding := func(id string) bool {
		return slices.ContainsFunc(investment.sources.Holdings, func(holding history.PositionHolding) bool {
			return holding.Holding.ID.String() == id
		})
	}
	var ready bool
	switch request.Kind {
	case historyapi.ChangeTrade:
		ready = hasHolding(request.HoldingID) && hasFields(request.SettlementAccountID, request.InstrumentID, request.Side, request.Quantity, request.Gross, request.GrossCurrency)
	case historyapi.ChangeCashDividend:
		ready = hasHolding(request.HoldingID) && hasFields(request.Amount, request.Currency)
	case historyapi.ChangePositionTransfer:
		ready = hasHolding(request.FromHoldingID) && hasHolding(request.ToHoldingID) && hasFields(request.Quantity)
	case historyapi.ChangePositionAdjustment:
		ready = hasHolding(request.HoldingID) && hasFields(request.Quantity)
	case historyapi.ChangeMoneyAdded, historyapi.ChangeMoneyRemoved:
		ready = hasFields(request.AccountID, request.Amount, request.Currency)
	case historyapi.ChangeCashTransfer:
		ready = hasFields(request.FromAccountID, request.ToAccountID, request.Sent, request.SentCurrency, request.Received, request.ReceivedCurrency)
	case historyapi.ChangeFXConversion:
		ready = hasFields(request.AccountID, request.Sold, request.SoldCurrency, request.Bought, request.BoughtCurrency)
	case historyapi.ChangeValueUpdate:
		ready = hasFields(request.AccountID, request.NewValue, request.NewValueCurrency)
	case historyapi.ChangeDebtDraw, historyapi.ChangeDebtPayment:
		ready = hasFields(request.DebtAccountID, request.CashAccountID, request.Principal, request.PrincipalCurrency)
	}
	return request, ready
}

func (form *historyAmendmentDraft) selectReplacement(kind historyapi.ChangeCommandKind) {
	investment := form.investment
	if investment.command.Kind == kind {
		return
	}
	investment.command = historyapi.ChangeCommandRequest{Kind: kind}
	if kind == historyapi.ChangeTrade {
		investment.command.Side = string(domain.TradeBuy)
	}
	if kind == investment.initial.Kind {
		investment.command = investment.initial
	}
	amount := investment.command.Amount
	if kind == historyapi.ChangeTrade {
		amount = investment.command.Gross
	}
	form.amount.Editor.SetText(amount)
	form.fee.Editor.SetText(investment.command.Fee)
	investment.quantity.Editor.SetText(investment.command.Quantity)
	investment.secondaryAmount.Editor.SetText("")
	form.problem = i18n.NoMessage
}

func (p *historyPage) replacementFields(view ui.View, form *historyAmendmentDraft, origin *image.Point, busy bool) []layout.Widget {
	investment := form.investment
	l := view.Language
	if investment.loading != nil {
		return []layout.Widget{view.Message(l.Text(i18n.HistoryLoading), view.Colors.SecondaryInk)}
	}
	if investment.readFailed {
		return []layout.Widget{
			view.Message(l.Text(i18n.ContributionDetailFailed), view.Colors.ErrorInk),
			taskLink(view, &investment.reload, l.Text(i18n.HistoryRetry), func() { p.reads.loadInvestmentReplacement(form) }),
		}
	}
	var rows []layout.Widget
	if investment.sources.OriginalTradeRestricted {
		rows = append(rows, view.Message(l.Text(i18n.HistoryReplacementArchivedTrade), view.Colors.SecondaryInk))
	} else {
		options := make([]ui.Choice, 0, len(replacementKinds))
		for _, kind := range replacementKinds {
			options = append(options, ui.Choice{ID: string(kind.kind), Label: l.Text(kind.label)})
		}
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			investment.kind.Layout(gtx, view, ui.PickerField{
				Label: i18n.AccountRecordType, Origin: *origin, Disabled: busy,
			}, ui.SelectOptions{Items: options}, string(investment.command.Kind), func(value string) {
				form.selectReplacement(historyapi.ChangeCommandKind(value))
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
		})
	}
	if isPositionReplacement(investment.command.Kind) {
		rows = append(rows, investmentReplacementFields(view, form, origin, busy)...)
	} else {
		rows = append(rows, cashReplacementFields(view, form, origin, busy)...)
	}
	return append(rows, func(gtx layout.Context) layout.Dimensions {
		return investment.note.Layout(gtx, view, ui.FieldOptions{
			Label: i18n.PreviewNotes, Hint: i18n.PreviewNotesHint, Help: i18n.PreviewNotesHelp, Disabled: busy,
		})
	})
}
