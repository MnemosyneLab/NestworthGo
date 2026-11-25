package desktop

import (
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func wealthBridgeStatus(status string) i18n.MessageID {
	switch status {
	case "reconciled":
		return i18n.WealthBridgeReconciled
	case "partial":
		return i18n.WealthBridgePartial
	default:
		return i18n.WealthBridgeUnavailable
	}
}

func wealthBridgeEvidence(view ui.View, sheet *wealthDetail) []layout.Widget {
	l, result := view.Language, sheet.result
	r := result.Reconciliation
	rows := append([]layout.Widget{view.Message(l.Text(i18n.WealthBridgeIdentity), view.Colors.Ink)}, reconciliationTotals(view, r)...)
	rows = append(rows, view.Message(l.Text(wealthBridgeStatus(r.Status)), view.Colors.SecondaryInk),
		view.Message(l.Text(i18n.WealthBridgeBasis), view.Colors.SecondaryInk))
	rows = append(rows, wealthBridgeAmounts(view, r.Contributions, r.Reasons, r.Currency)...)
	if len(r.Details) == 0 {
		return rows
	}
	rows = append(rows, view.Body(ui.TextLabel, l.Text(i18n.WealthRowComponents), view.Colors.Ink))
	byKey := make(map[string]application.HistoricalOverviewRow, len(result.Rows))
	for _, row := range result.Rows {
		byKey[row.Key] = row
	}
	for _, detail := range r.Details {
		row := byKey[detail.Key]
		name := wealthRowName(l, row)
		if parent := byKey[row.ParentKey]; parent.Name != "" {
			name = wealthRowName(l, parent) + " · " + name
		}
		button := portfolioButton(sheet.links, detail.Key)
		label := name + " · " + l.Text(i18n.WealthBridgeResidual) + " · " + wealthBridgeExact(view, detail.Residual, r.Currency)
		selected := sheet.component == detail.Key
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			if button.Clicked(gtx) {
				sheet.component = detail.Key
				if selected {
					sheet.component = ""
				}
				gtx.Execute(op.InvalidateCmd{})
			}
			return analysisToggle(view, button, label, selected)(gtx)
		})
		if selected {
			rows = append(rows, wealthBridgeComponent(view, detail, r.Currency, result.Timezone)...)
		}
	}
	return rows
}

func wealthBridgeComponent(view ui.View, detail application.HistoricalReconciliationDetail, currency, timezone string) []layout.Widget {
	l := view.Language
	rows := []layout.Widget{view.Message(l.Text(i18n.WealthBridgeComponentIdentity), view.Colors.SecondaryInk)}
	amount := func(id i18n.MessageID, value *string, currency string) {
		rows = append(rows, view.Message(l.Text(id)+" · "+wealthBridgeExact(view, value, currency), view.Colors.Ink))
	}
	amount(i18n.WealthAccountDelta, detail.Total, currency)
	amount(i18n.WealthBridgeSupported, &detail.Supported, currency)
	amount(i18n.WealthBridgePrincipal, &detail.Principal, currency)
	amount(i18n.WealthBridgeResidual, detail.Residual, currency)
	if detail.OpeningQuantity != nil {
		amount(i18n.WealthBridgeOpenQty, detail.OpeningQuantity, "")
		amount(i18n.WealthBridgeActivityQty, detail.ActivityQuantity, "")
		amount(i18n.WealthBridgeCloseQty, detail.ClosingQuantity, "")
		amount(i18n.WealthBridgeResidualQty, detail.QuantityResidual, "")
	}
	rows = append(rows, wealthBridgeAmounts(view, detail.Contributions, detail.Reasons, currency)...)
	if len(detail.Records) == 0 {
		return append(rows, view.Message(l.Text(i18n.WealthBridgeNoRecords), view.Colors.SecondaryInk))
	}
	rows = append(rows, view.Body(ui.TextLabel, l.Text(i18n.WealthBridgeRecords), view.Colors.Ink))
	if loc, err := time.LoadLocation(timezone); err == nil {
		view.TimeLocation = loc
	}
	for _, record := range detail.Records {
		kind := historyKindLabel(domain.ActivityKind(record.Kind), domain.ActivityReason(record.Reason))
		text := l.Text(kind)
		if at, err := time.Parse(time.RFC3339Nano, record.EffectiveAt); err == nil {
			text += " · " + valuationReadTime(view, at)
		}
		rows = append(rows, view.Message(text, view.Colors.Ink))
		if record.Note != nil && *record.Note != "" {
			rows = append(rows, view.Message(*record.Note, view.Colors.SecondaryInk))
		}
		for _, effect := range record.Effects {
			id := wealthBridgeEffect(effect.Classification)
			if effect.Amount != nil {
				amount(id, effect.Amount, effect.Currency)
			}
			if effect.Quantity != nil {
				amount(id, effect.Quantity, "")
			}
		}
	}
	return rows
}

func wealthBridgeAmounts(view ui.View, contributions []application.HistoricalContribution, reasons []string, currency string) []layout.Widget {
	l := view.Language
	rows := make([]layout.Widget, 0, len(contributions)+len(reasons))
	for _, item := range contributions {
		rows = append(rows, view.Message(l.Text(wealthBridgeContribution(item.Key))+" · "+wealthBridgeExact(view, &item.Amount, currency), view.Colors.Ink))
	}
	for _, reason := range reasons {
		rows = append(rows, view.Message(l.Text(wealthBridgeReason(reason)), view.Colors.SecondaryInk))
	}
	return rows
}

func wealthBridgeExact(view ui.View, value *string, currency string) string {
	if value == nil {
		return view.Language.Text(i18n.OverviewNoValue)
	}
	amount := portfolioDecimal(view, *value)
	if currency != "" {
		return amount + " " + currency
	}
	return amount
}

func wealthBridgeContribution(key string) i18n.MessageID {
	switch key {
	case "spending":
		return i18n.WealthBridgeSpending
	case "external_flow":
		return i18n.WealthBridgeExternalFlow
	case "income":
		return i18n.WealthBridgeIncome
	case "dividend_interest":
		return i18n.WealthBridgeDividend
	case "fee":
		return i18n.WealthBridgeFee
	case "adjustment":
		return i18n.WealthBridgeAdjustment
	case "liability_impact":
		return i18n.WealthBridgeDebt
	case "price_change":
		return i18n.WealthBridgePrice
	case "fx_impact":
		return i18n.WealthBridgeFX
	default:
		return i18n.OverviewEvidenceUnavailable
	}
}

func wealthBridgeReason(key string) i18n.MessageID {
	switch key {
	case "currency_mismatch":
		return i18n.WealthAllocationCurrencyChanged
	case "current_endpoint":
		return i18n.WealthBridgeCurrent
	case "incomplete_endpoints":
		return i18n.WealthBridgeIncomplete
	case "budget_exceeded":
		return i18n.WealthHistoryBudget
	case "scope_changed":
		return i18n.WealthHistoryScopeChanged
	case "basis_changed", "remeasurement_basis":
		return i18n.WealthHistoryBasisChanged
	case "quantity_mismatch", "quantity_activity":
		return i18n.WealthBridgeQuantityMismatch
	case "principal_pairing", "internal_imbalance":
		return i18n.WealthBridgeUnpaired
	case "valuation_mismatch", "missing_evidence":
		return i18n.WealthAllocationMissing
	case "foreign_internal_exposure":
		return i18n.WealthBridgeExposure
	case "unsupported_activity", "unexplained_movement":
		return i18n.WealthBridgeUnexplained
	default:
		return i18n.OverviewEvidenceUnavailable
	}
}

func wealthBridgeEffect(key string) i18n.MessageID {
	switch key {
	case "debt_principal", "trade_principal":
		return i18n.WealthBridgePrincipal
	case "fee":
		return i18n.WealthBridgeFee
	case "income":
		return i18n.WealthBridgeIncome
	case "internal_transfer":
		return i18n.WealthBridgeTransfer
	case "remeasurement":
		return i18n.WealthBridgeAdjustment
	case "external_inflow", "external_outflow":
		return i18n.WealthBridgeExternalFlow
	default:
		return i18n.OverviewEvidenceUnavailable
	}
}
