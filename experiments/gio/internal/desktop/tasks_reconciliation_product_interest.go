package desktop

import (
	"fmt"
	"image"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type holdingProductInterest struct {
	manualMaturityInterest bool
	remaining              ui.TextField
	holdingProductTime
}

type holdingProductTime struct {
	day, minute string
	date        ui.DatePicker
	clock       ui.TimePicker
}

func (f *holdingProductInterest) request(link domain.ReconciliationLink, id domain.ProductContractID, amount string) liquidity.ProductCommandRequest {
	command := &application.ReceiveInterestCommand{
		ProductID: id.String(), Amount: amount, EffectiveLocalDate: f.day, EffectiveLocalTime: f.minute,
	}
	if remaining := strings.TrimSpace(f.remaining.Text()); f.manualMaturityInterest && remaining != "" {
		command.RemainingInterest = &remaining
	}
	return liquidity.ProductCommandRequest{Kind: string(domain.ProductOpReceiveInterest), ReconciliationLink: &link, ReceiveInterest: command}
}

func (c *holdingReconciliation) holdingProductInterestFields(view ui.View, origin *image.Point, d *holdingReconDraft, ready bool) []layout.Widget {
	f := d.product.interest
	rows := c.holdingProductTimeFields(view, origin, d, &f.holdingProductTime, &d.total.Editor, ready)
	// Locked products cannot use simple interest. Only manual maturity
	// interest has an applicable forecast field on this receipt.
	if f.manualMaturityInterest {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return f.remaining.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.HoldingReconRemainingInterest, Disabled: !ready}, d.product.currency)
		})
	}
	return rows
}

func (c *holdingReconciliation) holdingProductTimeFields(view ui.View, origin *image.Point, d *holdingReconDraft, f *holdingProductTime, previous *widget.Editor, ready bool) []layout.Widget {
	l := view.Language
	week := time.Monday
	if view.WeekStart == "sunday" {
		week = time.Sunday
	}
	zone, today := l.Text(i18n.OverviewNoValue), ""
	if c.zone != nil {
		zone, today = c.zone.String(), c.clock().In(c.zone).Format(time.DateOnly)
	}
	options := ui.DateOptions{Today: today, Max: today, Clearable: true, WeekStart: week, Format: view.DateFormat}
	return []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			field := ui.PickerField{Label: i18n.PreviewEffectiveDate, Origin: *origin, Disabled: !ready || c.zone == nil,
				Previous: previous, Next: &f.clock.Trigger}
			f.date.Layout(gtx, view, field, options, f.day, func(day string) { f.day, d.problem = day, i18n.NoMessage })
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
		},
		func(gtx layout.Context) layout.Dimensions {
			field := ui.PickerField{Label: i18n.PreviewEffectiveTime, Origin: *origin, Disabled: !ready || c.zone == nil,
				Previous: &f.date.Trigger}
			f.clock.Layout(gtx, view, field, f.minute, view.TimeFormat == settings.TimeFormat12, true,
				func(minute string) { f.minute, d.problem = minute, i18n.NoMessage })
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
		},
		view.Message(fmt.Sprintf(l.Text(i18n.ProductReceiptTimeHelp), zone), view.Colors.SecondaryInk),
	}
}

func holdingProductOperationReviewRows(view ui.View, f *holdingProductDraft, review *liquidity.ProductOperationPreviewDTO) []layout.Widget {
	l := view.Language
	fact := func(label i18n.MessageID, text string) layout.Widget {
		return view.Message(fmt.Sprintf(l.Text(label), text), view.Colors.Ink)
	}
	money := func(label i18n.MessageID, value *wire.MoneyView) layout.Widget {
		if value == nil {
			return fact(label, l.Text(i18n.OverviewNoValue))
		}
		return fact(label, holdingSnapshotValue(view, value.Amount, value.Currency))
	}
	rows := []layout.Widget{view.Message(f.name, view.Colors.Ink)}
	if f.add != nil {
		rows = append(rows, holdingProductAddFacts(view, []byte(review.NormalizedJSON))...)
	} else if f.renewal != nil {
		rows = append(rows, holdingRenewalFacts(view, []byte(review.NormalizedJSON), f.currency)...)
	} else if f.settlement != nil {
		rows = append(rows, holdingSettlementFacts(view, []byte(review.NormalizedJSON), f.currency)...)
	} else {
		for _, change := range review.Activities {
			for _, effect := range change.Effects {
				if effect.Target == string(domain.EffectTargetAccountCash) && effect.Role == string(domain.EffectRoleAmount) {
					rows = append(rows, money(i18n.ProductInterestFact, effect.Money))
				}
			}
		}
	}
	for _, value := range review.CashBefore {
		rows = append(rows, money(i18n.ProductCashBefore, &value))
	}
	if len(review.CashBefore) == 0 {
		rows = append(rows, money(i18n.ProductCashBefore, nil))
	}
	for _, value := range review.CashAfter {
		rows = append(rows, money(i18n.ProductCashAfter, &value))
	}
	if len(review.CashAfter) == 0 {
		rows = append(rows, money(i18n.ProductCashAfter, nil))
	}
	rows = append(rows, money(i18n.ProductValueBefore, review.ProductBefore), money(i18n.ProductValueAfter, review.ProductAfter))
	impact := l.Text(i18n.OverviewNoValue)
	if review.NetWorthKnown && review.NetWorthDelta != nil {
		impact = holdingSnapshotValue(view, review.NetWorthDelta.Amount, review.NetWorthDelta.Currency)
	}
	rows = append(rows, fact(i18n.HoldingReconValuationImpact, impact))
	zone, err := time.LoadLocation(review.Timezone)
	if err == nil {
		if at, err := time.Parse(time.RFC3339Nano, review.EffectiveAt); err == nil {
			rows = append(rows, view.Message(tradeTimestamp(view, at, zone), view.Colors.SecondaryInk))
		}
	}
	if f.interest != nil {
		var command application.ProductCommand
		if jsoncodec.Unmarshal([]byte(review.NormalizedJSON), &command) == nil && command.ReceiveInterest != nil && command.ReceiveInterest.RemainingInterest != nil {
			rows = append(rows, fact(i18n.HoldingReconRemainingInterestFact,
				holdingSnapshotValue(view, *command.ReceiveInterest.RemainingInterest, f.currency)))
		}
	}
	return rows
}

func holdingProductInterestEvidenceRows(view ui.View, evidence holdingLinkedEvidence) []layout.Widget {
	var command application.ProductCommand
	if jsoncodec.Unmarshal(evidence.Committed.Request, &command) != nil || command.ReceiveInterest == nil {
		return nil
	}
	fact := func(label i18n.MessageID, value, currency string) layout.Widget {
		text := fmt.Sprintf(view.Language.Text(label), holdingSnapshotValue(view, value, currency))
		return view.Message(text, view.Colors.Ink)
	}
	var rows []layout.Widget
	if evidence.Before != nil {
		rows = append(rows, fact(i18n.ProductInterestFact, strings.TrimSpace(command.ReceiveInterest.Amount), evidence.Before.Currency))
	}
	for _, contract := range evidence.Committed.Contracts {
		if contract.MaturityInterest != nil {
			rows = append(rows, fact(i18n.HoldingReconRemainingInterestFact, *contract.MaturityInterest, contract.Currency))
		}
	}
	return rows
}
